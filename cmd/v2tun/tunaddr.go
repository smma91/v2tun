//go:build windows || linux

package main

import (
	"net/netip"
	"strings"
)

// tunCandidates are the tunnel addresses tried in order (each a /30). The
// first is the historical default; the others are used only when it overlaps
// a network already on this machine. None is in the FakeIP range.
var tunCandidates = []netip.Addr{
	netip.MustParseAddr(tunAddr),
	netip.MustParseAddr("10.233.233.1"),
	netip.MustParseAddr("172.23.233.1"),
	netip.MustParseAddr("192.168.233.1"),
}

const tunBits = 30

// netIface / netRoute are a platform-neutral snapshot of the IPv4 setup.
type netIface struct {
	Name, Desc string
	ID         uint64 // LUID on Windows, interface index on Linux
	Up         bool
	Tunnel     bool           // a VPN / TUN style adapter
	Prefixes   []netip.Prefix // its IPv4 addresses with their on-link prefix
}

type netRoute struct {
	Dst    netip.Prefix
	ID     uint64
	Metric uint32 // effective metric (lower wins)
	Main   bool   // in the main routing table (always true on Windows)
}

// usedPrefixes lists the IPv4 networks present on the machine, except our own
// tunnel and routes too broad to mean a real network (default, /1, ...).
func usedPrefixes(ifs []netIface, routes []netRoute, self uint64) []netip.Prefix {
	ours := func(id uint64, name string) bool { return id == self || name == tunName }
	names := map[uint64]string{}
	var out []netip.Prefix
	for _, i := range ifs {
		names[i.ID] = i.Name
		if ours(i.ID, i.Name) {
			continue
		}
		for _, p := range i.Prefixes {
			if p.IsValid() && p.Addr().Is4() && !p.Addr().IsLoopback() {
				out = append(out, p)
			}
		}
	}
	for _, r := range routes {
		if ours(r.ID, names[r.ID]) || !r.Dst.IsValid() || !r.Dst.Addr().Is4() || r.Dst.Bits() < 8 || r.Dst.Addr().IsLoopback() {
			continue
		}
		out = append(out, r.Dst)
	}
	return out
}

// pickTunAddr returns the first candidate whose /30 overlaps nothing in use;
// false (and the default) when every candidate overlaps.
func pickTunAddr(used []netip.Prefix) (netip.Addr, bool) {
next:
	for _, c := range tunCandidates {
		p := netip.PrefixFrom(c, tunBits).Masked()
		for _, u := range used {
			if p.Overlaps(u) {
				continue next
			}
		}
		return c, true
	}
	return tunCandidates[0], false
}

// chooseTunAddr picks the tunnel address for this connection; self is our
// own adapter (already created, possibly still holding an old address).
func (e *Engine) chooseTunAddr(self uint64) string {
	ifs, routes, err := netSnapshot()
	if err != nil {
		return tunAddr
	}
	a, ok := pickTunAddr(usedPrefixes(ifs, routes, self))
	if !ok {
		return tunAddr
	}
	if a.String() != tunAddr {
		e.logf("info", "Tunnel address %s (%s is used by another network)", a, tunAddr)
	}
	return a.String()
}

var vpnKeywords = []string{"tap-windows", "wintun", "wireguard", "openvpn", "anyconnect", "vpn", "pangp",
	"zerotier", "tailscale", "sing-tun", "singbox"}

func vpnDescription(s string) bool {
	s = strings.ToLower(s)
	for _, k := range vpnKeywords {
		if strings.Contains(s, k) {
			return true
		}
	}
	return false
}

// routeConflict returns the name of another VPN / TUN adapter (not ours) that
// carries the internet traffic, or "". Ordinary adapters (Wi-Fi, Ethernet,
// USB tethering, PPPoE) never count.
func routeConflict(ifs []netIface, routes []netRoute) string {
	target := netip.MustParseAddr("1.1.1.1")
	byID := map[uint64]netIface{}
	for _, i := range ifs {
		byID[i.ID] = i
	}
	found := false
	var best netRoute
	var bestIf netIface
	for _, r := range routes {
		i, ok := byID[r.ID]
		if !ok || !i.Up || i.Name == tunName || !r.Dst.IsValid() || !r.Dst.Contains(target) {
			continue
		}
		if !r.Main {
			// A policy-routed full tunnel (wg-quick, Tailscale exit node...).
			if i.Tunnel && r.Dst.Bits() == 0 {
				return i.Name
			}
			continue
		}
		b, bb := r.Dst.Bits(), best.Dst.Bits()
		if !found || b > bb || b == bb && (r.Metric < best.Metric || r.Metric == best.Metric && bestIf.Tunnel && !i.Tunnel) {
			found, best, bestIf = true, r, i
		}
	}
	if found && bestIf.Tunnel {
		return bestIf.Name
	}
	return ""
}
