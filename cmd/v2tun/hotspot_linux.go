//go:build linux

package main

import (
	"errors"
	"net"
	"net/netip"
	"os/exec"
	"strings"

	"github.com/google/nftables"
	"github.com/google/nftables/expr"
	"github.com/vishvananda/netlink"
)

// On Linux NetworkManager runs the access point ("shared" mode: DHCP and DNS
// by its dnsmasq, IP forwarding and NAT). Its DNS questions leave through the
// real network and are captured into the tunnel (captureDNS); the devices'
// traffic follows the routes into the tunnel, and the firewall table below
// stops it from going anywhere else.

const (
	hotspotConn  = "v2tun-hotspot"
	hotspotTable = "v2tun_hotspot"
)

type accessPoint struct{ ifname string }

func nmcli(args ...string) (string, error) {
	out, err := exec.Command("nmcli", args...).CombinedOutput()
	s := strings.TrimSpace(string(out))
	if err != nil {
		if s == "" {
			s = err.Error()
		}
		return s, errors.New(s)
	}
	return s, nil
}

func startAccessPoint(ssid, pass, dns string) (*accessPoint, error) {
	if _, err := exec.LookPath("nmcli"); err != nil {
		return nil, trErr("The hotspot needs NetworkManager (nmcli was not found)")
	}
	if len(pass) < 8 || len(pass) > 63 {
		return nil, trErr("The hotspot password must have 8 to 63 characters")
	}
	if len(ssid) == 0 || len(ssid) > 32 {
		return nil, trErr("The hotspot name must have 1 to 32 characters")
	}
	out, err := nmcli("-t", "-f", "DEVICE,TYPE", "device")
	if err != nil {
		return nil, err
	}
	dev := ""
	for _, l := range strings.Split(out, "\n") {
		if d, t, ok := strings.Cut(l, ":"); ok && t == "wifi" {
			dev = d
			break
		}
	}
	if dev == "" {
		return nil, trErr("No Wi-Fi card was found")
	}
	nmcli("connection", "delete", hotspotConn) // an old one from a crash
	if _, err := nmcli("device", "wifi", "hotspot", "ifname", dev, "con-name", hotspotConn, "ssid", ssid, "password", pass); err != nil {
		return nil, err
	}
	return &accessPoint{ifname: dev}, nil
}

func (ap *accessPoint) Alive() bool {
	out, _ := nmcli("-t", "-f", "NAME", "connection", "show", "--active")
	for _, l := range strings.Split(out, "\n") {
		if l == hotspotConn {
			return true
		}
	}
	return false
}

// Clients counts the devices seen on the hotspot recently.
func (ap *accessPoint) Clients() int {
	link, err := netlink.LinkByName(ap.ifname)
	if err != nil {
		return 0
	}
	ns, _ := netlink.NeighList(link.Attrs().Index, netlink.FAMILY_V4)
	n := 0
	for _, x := range ns {
		if x.State&(netlink.NUD_REACHABLE|netlink.NUD_STALE|netlink.NUD_DELAY|netlink.NUD_PROBE) != 0 && x.IP.To4() != nil && !x.IP.Equal(net.IPv4bcast) {
			n++
		}
	}
	return n
}

func (ap *accessPoint) Close() {
	nmcli("connection", "down", hotspotConn)
	nmcli("connection", "delete", hotspotConn)
}

func (e *Engine) routeHotspot(ap *accessPoint) error {
	e.mu.Lock()
	defer e.mu.Unlock()
	if !e.running {
		return trErr("Connect first: the hotspot shares the tunnel")
	}
	c, err := nftables.New()
	if err != nil {
		return err
	}
	deleteTable(c, nftables.TableFamilyINet, hotspotTable)
	t := c.AddTable(&nftables.Table{Family: nftables.TableFamilyINet, Name: hotspotTable})
	policy := nftables.ChainPolicyAccept
	ch := c.AddChain(&nftables.Chain{Name: "forward", Table: t, Type: nftables.ChainTypeFilter,
		Hooknum: nftables.ChainHookForward, Priority: nftables.ChainPriorityFilter, Policy: &policy})
	iif := func(name string) []expr.Any {
		return []expr.Any{
			&expr.Meta{Key: expr.MetaKeyIIFNAME, Register: 1},
			&expr.Cmp{Op: expr.CmpOpEq, Register: 1, Data: ifname(name)},
		}
	}
	add := func(e []expr.Any) { c.AddRule(&nftables.Rule{Table: t, Chain: ch, Exprs: e}) }
	add(join(iif(ap.ifname), oif(expr.CmpOpEq, tunName), accept)) // devices -> tunnel
	add(join(iif(tunName), oif(expr.CmpOpEq, ap.ifname), accept)) // replies
	add(join(iif(ap.ifname), reject))                             // anything else from the hotspot
	if err := c.Flush(); err != nil {
		return err
	}
	if a, err := netlink.LinkByName(ap.ifname); err == nil {
		if as, _ := netlink.AddrList(a, netlink.FAMILY_V4); len(as) > 0 {
			ones, _ := as[0].Mask.Size()
			if ip, ok := netip.AddrFromSlice(as[0].IP.To4()); ok {
				e.handler.watchHotspot(netip.PrefixFrom(ip, ones).Masked())
			}
		}
	}
	return nil
}

func (e *Engine) unrouteHotspot(ap *accessPoint) {
	if c, err := nftables.New(); err == nil {
		deleteTable(c, nftables.TableFamilyINet, hotspotTable)
	}
}

// cleanupHotspot removes a hotspot left by a v2tun that was killed.
func cleanupHotspot() {
	exec.Command("nmcli", "connection", "delete", hotspotConn).Run()
	if c, err := nftables.New(); err == nil {
		deleteTable(c, nftables.TableFamilyINet, hotspotTable)
	}
}
