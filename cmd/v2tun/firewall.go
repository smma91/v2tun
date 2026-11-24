//go:build windows

package main

import (
	"fmt"
	"net/netip"

	"github.com/tailscale/wf"
	"golang.org/x/sys/windows"
)

// Weights inside our WFP sublayer; higher is evaluated first.
const (
	wLoopback = 15
	wTunnel   = 14
	wApp      = 13
	wDHCP     = 12
	wBlockDNS = 11
	wLAN      = 10
	wBlockAll = 0
)

var (
	connectLayers = []wf.LayerID{wf.LayerALEAuthConnectV4, wf.LayerALEAuthConnectV6}
	allLayers     = []wf.LayerID{wf.LayerALEAuthConnectV4, wf.LayerALEAuthConnectV6, wf.LayerALEAuthRecvAcceptV4, wf.LayerALEAuthRecvAcceptV6}
)

type killSwitch struct {
	s        *wf.Session
	provider wf.ProviderID
	sublayer wf.SublayerID
	hotspot  []wf.RuleID // rules of an active hotspot
}

func guid() windows.GUID {
	g, _ := windows.GenerateGUID()
	return g
}

// enableKillSwitch blocks every connection that does not go through the
// tunnel. Exceptions: loopback (v2rayN's SOCKS port), the proxy cores and this
// app (they carry the tunnel to the server), DHCP and optionally the LAN.
// The WFP session is dynamic: Windows drops all rules when the app exits.
//
// With full=false only the leak guard is installed (no IPv6, no DNS outside
// the tunnel) and regular IPv4 traffic is not blocked.
func enableKillSwitch(tunLUID uint64, apps []string, allowLAN, full bool) (*killSwitch, error) {
	s, err := wf.New(&wf.Options{Name: "v2tun", Description: "v2tun kill switch", Dynamic: true})
	if err != nil {
		return nil, fmt.Errorf("WFP: %w", err)
	}
	k := &killSwitch{s: s, provider: wf.ProviderID(guid()), sublayer: wf.SublayerID(guid())}
	ok := false
	defer func() {
		if !ok {
			s.Close()
		}
	}()
	if err := s.AddProvider(&wf.Provider{ID: k.provider, Name: "v2tun"}); err != nil {
		return nil, err
	}
	if err := s.AddSublayer(&wf.Sublayer{ID: k.sublayer, Name: "v2tun", Provider: k.provider, Weight: 0xFFFF}); err != nil {
		return nil, err
	}

	for _, l := range allLayers {
		if err := k.add("loopback", l, wLoopback, wf.ActionPermit,
			&wf.Match{Field: wf.FieldFlags, Op: wf.MatchTypeFlagsAllSet, Value: wf.ConditionFlagIsLoopback}); err != nil {
			return nil, err
		}
		if err := k.add("tunnel", l, wTunnel, wf.ActionPermit,
			&wf.Match{Field: wf.FieldIPLocalInterface, Op: wf.MatchTypeEqual, Value: tunLUID}); err != nil {
			return nil, err
		}
	}
	for _, app := range apps {
		id, err := wf.AppID(app)
		if err != nil {
			continue
		}
		for _, l := range allLayers {
			if err := k.add("app "+app, l, wApp, wf.ActionPermit,
				&wf.Match{Field: wf.FieldALEAppID, Op: wf.MatchTypeEqual, Value: id}); err != nil {
				return nil, err
			}
		}
	}
	udp := &wf.Match{Field: wf.FieldIPProtocol, Op: wf.MatchTypeEqual, Value: wf.IPProtoUDP}
	if err := k.add("dhcp", wf.LayerALEAuthConnectV4, wDHCP, wf.ActionPermit, udp,
		&wf.Match{Field: wf.FieldIPRemotePort, Op: wf.MatchTypeEqual, Value: uint16(67)}); err != nil {
		return nil, err
	}
	if err := k.add("dhcp6", wf.LayerALEAuthConnectV6, wDHCP, wf.ActionPermit, udp,
		&wf.Match{Field: wf.FieldIPRemotePort, Op: wf.MatchTypeEqual, Value: uint16(547)}); err != nil {
		return nil, err
	}
	if !full {
		// Kill switch off: IPv4 may go outside the tunnel when it is down, but
		// the two silent leaks are still closed. IPv6 is not carried by the
		// tunnel, so all internet IPv6 is blocked (local link and multicast
		// stay, the LAN keeps working); and DNS may only leave through the
		// tunnel, so Windows cannot also ask the router or ISP resolvers.
		if err := k.blockDNSOutside(); err != nil {
			return nil, err
		}
		for _, l := range []wf.LayerID{wf.LayerALEAuthConnectV6, wf.LayerALEAuthRecvAcceptV6} {
			for _, n := range []string{"fe80::/10", "ff00::/8"} {
				if err := k.add("ipv6 local "+n, l, wLAN, wf.ActionPermit,
					&wf.Match{Field: wf.FieldIPRemoteAddress, Op: wf.MatchTypeEqual, Value: netip.MustParsePrefix(n)}); err != nil {
					return nil, err
				}
			}
			if err := k.add("block ipv6 (not tunnelled)", l, wBlockAll, wf.ActionBlock); err != nil {
				return nil, err
			}
		}
		ok = true
		return k, nil
	}
	if allowLAN {
		for _, l := range connectLayers {
			for _, p := range []wf.IPProto{wf.IPProtoUDP, wf.IPProtoTCP} {
				if err := k.add("block dns outside tunnel", l, wBlockDNS, wf.ActionBlock,
					&wf.Match{Field: wf.FieldIPProtocol, Op: wf.MatchTypeEqual, Value: p},
					&wf.Match{Field: wf.FieldIPRemotePort, Op: wf.MatchTypeEqual, Value: uint16(53)}); err != nil {
					return nil, err
				}
			}
		}
		for _, l := range allLayers {
			nets := []string{"fe80::/10", "fc00::/7", "ff00::/8"}
			if l == wf.LayerALEAuthConnectV4 || l == wf.LayerALEAuthRecvAcceptV4 {
				nets = []string{"10.0.0.0/8", "172.16.0.0/12", "192.168.0.0/16", "169.254.0.0/16", "224.0.0.0/4", "255.255.255.255/32"}
			}
			for _, n := range nets {
				if err := k.add("lan "+n, l, wLAN, wf.ActionPermit,
					&wf.Match{Field: wf.FieldIPRemoteAddress, Op: wf.MatchTypeEqual, Value: netip.MustParsePrefix(n)}); err != nil {
					return nil, err
				}
			}
		}
	}
	for _, l := range allLayers {
		if err := k.add("block all", l, wBlockAll, wf.ActionBlock); err != nil {
			return nil, err
		}
	}
	ok = true
	return k, nil
}

func (k *killSwitch) blockDNSOutside() error {
	for _, l := range connectLayers {
		for _, p := range []wf.IPProto{wf.IPProtoUDP, wf.IPProtoTCP} {
			if err := k.add("block dns outside tunnel", l, wBlockDNS, wf.ActionBlock,
				&wf.Match{Field: wf.FieldIPProtocol, Op: wf.MatchTypeEqual, Value: p},
				&wf.Match{Field: wf.FieldIPRemotePort, Op: wf.MatchTypeEqual, Value: uint16(53)}); err != nil {
				return err
			}
		}
	}
	return nil
}

func (k *killSwitch) add(name string, layer wf.LayerID, weight uint64, action wf.Action, conds ...*wf.Match) error {
	err := k.s.AddRule(&wf.Rule{
		ID: wf.RuleID(guid()), Name: "v2tun: " + name, Layer: layer, Sublayer: k.sublayer,
		Provider: k.provider, Weight: weight, Conditions: conds, Action: action,
	})
	if err != nil {
		return fmt.Errorf("rule %q: %w", name, err)
	}
	return nil
}

// allowHotspot lets the hotspot adapter work under the kill switch: DHCP and
// DNS from its devices reach Windows' sharing service and v2tun. (Their
// forwarded traffic follows the routes into the tunnel; when the tunnel stops
// v2tun turns the hotspot off, and it also ends with v2tun if v2tun is killed.)
func (k *killSwitch) allowHotspot(apLUID uint64) error {
	k.removeHotspot()
	add := func(name string, layer wf.LayerID, weight uint64, action wf.Action, conds ...*wf.Match) error {
		id := wf.RuleID(guid())
		if err := k.s.AddRule(&wf.Rule{ID: id, Name: "v2tun: hotspot " + name, Layer: layer, Sublayer: k.sublayer,
			Provider: k.provider, Weight: weight, Conditions: conds, Action: action}); err != nil {
			return fmt.Errorf("rule %q: %w", name, err)
		}
		k.hotspot = append(k.hotspot, id)
		return nil
	}
	for _, l := range allLayers {
		if err := add("adapter", l, wTunnel, wf.ActionPermit,
			&wf.Match{Field: wf.FieldIPLocalInterface, Op: wf.MatchTypeEqual, Value: apLUID}); err != nil {
			return err
		}
	}
	return nil
}

func (k *killSwitch) removeHotspot() {
	for _, id := range k.hotspot {
		k.s.DeleteRule(id)
	}
	k.hotspot = nil
}

func (k *killSwitch) Close() {
	if k != nil && k.s != nil {
		k.s.Close()
	}
}
