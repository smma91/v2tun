//go:build linux

package main

import (
	"net"
	"net/netip"
	"os"

	"github.com/vishvananda/netlink"
)

// prepareTun: the TUN device and the routes need CAP_NET_ADMIN.
func prepareTun() error {
	if !hasPrivileges() {
		return trErr("v2tun needs network permission. Press \"Grant permission\" (or run: sudo setcap cap_net_admin,cap_net_raw+ep %s)", exePath())
	}
	if _, err := os.Stat("/dev/net/tun"); err != nil {
		return trErr("The TUN device (/dev/net/tun) is not available on this system")
	}
	return nil
}

func exePath() string {
	if p := os.Getenv("APPIMAGE"); p != "" {
		return p
	}
	p, _ := os.Executable()
	return p
}

// configureTun gives the tunnel its address and brings it up. DNS needs no
// per-interface setting: every DNS query that would leave through the real
// network is redirected into the tunnel by the firewall (see firewall_linux.go),
// whatever resolver the system uses (systemd-resolved, NetworkManager, …).
func (e *Engine) configureTun(dev *tunDevice, dns []string) (uint32, error) {
	link, err := netlink.LinkByIndex(int(dev.LUID()))
	if err != nil {
		return 0, err
	}
	addr, err := netlink.ParseAddr(e.chooseTunAddr(uint64(link.Attrs().Index)) + "/30")
	if err != nil {
		return 0, err
	}
	if err := netlink.AddrReplace(link, addr); err != nil {
		return 0, err
	}
	if err := netlink.LinkSetUp(link); err != nil {
		return 0, err
	}
	target := netip.MustParseAddr("1.1.1.1")
	for _, d := range dns {
		if a, err := netip.ParseAddr(d); err == nil && a.Is4() {
			target = a
			break
		}
	}
	if err := captureDNS(target); err != nil {
		return 0, err
	}
	return uint32(dev.LUID()), nil
}

func (e *Engine) unconfigureTun() { releaseDNS() }

// addTunRoutes: two /1 routes cover the whole IPv4 internet and win over the
// default route without replacing it.
func addTunRoutes(idx uint32) error {
	for _, pfx := range []string{"0.0.0.0/1", "128.0.0.0/1"} {
		_, dst, _ := net.ParseCIDR(pfx)
		r := &netlink.Route{LinkIndex: int(idx), Dst: dst, Scope: netlink.SCOPE_LINK, Priority: 1}
		if err := netlink.RouteReplace(r); err != nil {
			return err
		}
	}
	return nil
}

// flushDNSCache: drop answers cached by systemd-resolved (FakeIP addresses
// are only valid while the tunnel is up). Best effort.
func flushDNSCache() { run("resolvectl", "flush-caches") }
