//go:build windows || linux

package main

import (
	"net/netip"
	"testing"
)

func pfx(s string) netip.Prefix { return netip.MustParsePrefix(s) }

func TestTunAddrCandidates(t *testing.T) {
	if tunCandidates[0].String() != tunAddr {
		t.Fatalf("the default must stay first: %v", tunCandidates[0])
	}
	if tunMask != "255.255.255.252" || tunBits != 30 {
		t.Fatal("mask and prefix length disagree")
	}
	for i, a := range tunCandidates {
		p := netip.PrefixFrom(a, tunBits).Masked()
		if !a.Is4() || a == p.Addr() || p.Overlaps(fakePrefix) || !a.IsPrivate() {
			t.Errorf("bad candidate %v", a)
		}
		for _, b := range tunCandidates[i+1:] {
			if p.Contains(b) {
				t.Errorf("candidates %v and %v share a /30", a, b)
			}
		}
	}
}

func TestTunAddrPick(t *testing.T) {
	cases := []struct {
		used []netip.Prefix
		want string
		ok   bool
	}{
		{nil, tunAddr, true},
		{[]netip.Prefix{pfx("192.168.1.0/24"), pfx("172.20.10.0/28")}, tunAddr, true},
		{[]netip.Prefix{pfx("172.19.0.0/16")}, "10.233.233.1", true},   // a Docker network
		{[]netip.Prefix{pfx("172.19.233.2/32")}, "10.233.233.1", true}, // a single host
		{[]netip.Prefix{pfx("172.16.0.0/12"), pfx("10.0.0.0/8")}, "192.168.233.1", true},
		{[]netip.Prefix{pfx("172.16.0.0/12"), pfx("10.0.0.0/8"), pfx("192.168.0.0/16")}, tunAddr, false},
	}
	for _, c := range cases {
		got, ok := pickTunAddr(c.used)
		if got.String() != c.want || ok != c.ok {
			t.Errorf("%v: got %v %v, want %v %v", c.used, got, ok, c.want, c.ok)
		}
	}
}

// thisPC mirrors the developer machine: internet over iPhone USB tethering,
// Hyper-V / VMware adapters, disconnected VPN adapters and v2tun running.
func thisPC() ([]netIface, []netRoute) {
	ifs := []netIface{
		{Name: "Ethernet 8", Desc: "Apple Mobile Device Ethernet #2", ID: 7, Up: true, Prefixes: []netip.Prefix{pfx("172.20.10.6/28")}},
		{Name: "vEthernet (WSL (Hyper-V firewall))", Desc: "Hyper-V Virtual Ethernet Adapter #2", ID: 98, Up: true, Prefixes: []netip.Prefix{pfx("172.23.192.1/20")}},
		{Name: "VMware Network Adapter VMnet1", Desc: "VMware Virtual Ethernet Adapter for VMnet1", ID: 26, Up: true, Prefixes: []netip.Prefix{pfx("10.200.200.1/24")}},
		{Name: "Local Area Connection 2", Desc: "TAP-Windows Adapter V9", ID: 8, Tunnel: true},
		{Name: "Ethernet 6", Desc: "Fortinet SSL VPN Virtual Ethernet Adapter", ID: 28, Tunnel: true},
		{Name: tunName, Desc: "WireGuard Tunnel", ID: 89, Up: true, Tunnel: true, Prefixes: []netip.Prefix{pfx("172.19.233.1/30")}},
	}
	routes := []netRoute{
		{Dst: pfx("0.0.0.0/0"), ID: 7, Metric: 25, Main: true},
		{Dst: pfx("172.20.10.0/28"), ID: 7, Metric: 281, Main: true},
		{Dst: pfx("172.23.192.0/20"), ID: 98, Metric: 5256, Main: true},
		{Dst: pfx("10.200.200.0/24"), ID: 26, Metric: 291, Main: true},
		{Dst: pfx("0.0.0.0/1"), ID: 89, Metric: 6, Main: true},
		{Dst: pfx("128.0.0.0/1"), ID: 89, Metric: 6, Main: true},
		{Dst: pfx("172.19.233.0/30"), ID: 89, Metric: 261, Main: true},
		{Dst: pfx("127.0.0.0/8"), ID: 1, Metric: 331, Main: true},
	}
	return ifs, routes
}

func TestTunAddrUsed(t *testing.T) {
	ifs, routes := thisPC()
	used := usedPrefixes(ifs, routes, 89)
	for _, u := range used {
		if u.Overlaps(pfx("172.19.233.0/30")) || u.Bits() < 8 || u.Addr().IsLoopback() {
			t.Errorf("unexpected used prefix %v", u)
		}
	}
	if a, ok := pickTunAddr(used); !ok || a.String() != tunAddr {
		t.Errorf("this PC must keep the default, got %v", a)
	}
	// Our adapter is also recognised by name (e.g. a different LUID).
	if a, _ := pickTunAddr(usedPrefixes(ifs, routes, 12345)); a.String() != tunAddr {
		t.Errorf("own adapter counted as used: %v", a)
	}
	// A Docker / WSL network over the default moves the tunnel.
	ifs = append(ifs, netIface{Name: "br-1", ID: 50, Prefixes: []netip.Prefix{pfx("172.19.0.1/16")}})
	if a, _ := pickTunAddr(usedPrefixes(ifs, routes, 89)); a.String() != "10.233.233.1" {
		t.Errorf("overlap not detected: %v", a)
	}
}

func TestRouteConflict(t *testing.T) {
	ifs, routes := thisPC()
	if c := routeConflict(ifs, routes); c != "" {
		t.Fatalf("this PC: false conflict %q", c)
	}
	// Not connected yet: still no conflict.
	if c := routeConflict(ifs[:len(ifs)-1], routes); c != "" {
		t.Fatalf("this PC (v2tun off): false conflict %q", c)
	}

	wg := netIface{Name: "wg0", Desc: "WireGuard Tunnel", ID: 60, Up: true, Tunnel: true, Prefixes: []netip.Prefix{pfx("10.8.0.2/32")}}
	with := func(i netIface, rs ...netRoute) ([]netIface, []netRoute) {
		a, b := thisPC()
		return append(a, i), append(b, rs...)
	}
	cases := []struct {
		name string
		ifs  []netIface
		rts  []netRoute
		want string
	}{}
	add := func(name, want string, i netIface, rs ...netRoute) {
		a, b := with(i, rs...)
		cases = append(cases, struct {
			name string
			ifs  []netIface
			rts  []netRoute
			want string
		}{name, a, b, want})
	}
	add("wireguard /1 routes", "wg0", wg, netRoute{Dst: pfx("0.0.0.0/1"), ID: 60, Metric: 5, Main: true}, netRoute{Dst: pfx("128.0.0.0/1"), ID: 60, Metric: 5, Main: true})
	add("wireguard split tunnel", "", wg, netRoute{Dst: pfx("10.8.0.0/24"), ID: 60, Metric: 5, Main: true})
	add("full tunnel default, lower metric", "wg0", wg, netRoute{Dst: pfx("0.0.0.0/0"), ID: 60, Metric: 1, Main: true})
	add("full tunnel default, higher metric", "", wg, netRoute{Dst: pfx("0.0.0.0/0"), ID: 60, Metric: 9000, Main: true})
	add("full tunnel default, same metric", "", wg, netRoute{Dst: pfx("0.0.0.0/0"), ID: 60, Metric: 25, Main: true})
	down := wg
	down.Up = false
	add("vpn adapter down", "", down, netRoute{Dst: pfx("0.0.0.0/1"), ID: 60, Metric: 5, Main: true})
	add("policy routing (wg-quick)", "wg0", wg, netRoute{Dst: pfx("0.0.0.0/0"), ID: 60, Metric: 0, Main: false})
	eth := netIface{Name: "eth1", ID: 61, Up: true, Prefixes: []netip.Prefix{pfx("192.168.5.2/24")}}
	add("second uplink in another table", "", eth, netRoute{Dst: pfx("0.0.0.0/0"), ID: 61, Main: false})
	add("second uplink preferred", "", eth, netRoute{Dst: pfx("0.0.0.0/0"), ID: 61, Metric: 1, Main: true})
	ppp := netIface{Name: "ppp0", Desc: "WAN Miniport (PPPOE)", ID: 62, Up: true, Tunnel: vpnDescription("WAN Miniport (PPPOE)")}
	add("pppoe", "", ppp, netRoute{Dst: pfx("0.0.0.0/0"), ID: 62, Metric: 1, Main: true})
	for _, c := range cases {
		if got := routeConflict(c.ifs, c.rts); got != c.want {
			t.Errorf("%s: got %q, want %q", c.name, got, c.want)
		}
	}
}

func TestTunAddrVPNDescription(t *testing.T) {
	for d, want := range map[string]bool{
		"Apple Mobile Device Ethernet #2":           false,
		"Intel(R) Wi-Fi 6 AX201 160MHz":             false,
		"Intel(R) Ethernet Connection (16) I219-LM": false,
		"Remote NDIS based Internet Sharing Device": false,
		"Hyper-V Virtual Ethernet Adapter":          false,
		"Realtek USB GbE Family Controller":         false,
		"WAN Miniport (PPPOE)":                      false,
		"TAP-Windows Adapter V9":                    true,
		"Fortinet SSL VPN Virtual Ethernet Adapter": true,
		"WireGuard Tunnel":                          true,
		"Wintun Userspace Tunnel":                   true,
	} {
		if vpnDescription(d) != want {
			t.Errorf("%s: want %v", d, want)
		}
	}
}

// TestTunAddrLive only reads this machine's adapters and routes.
func TestTunAddrLive(t *testing.T) {
	ifs, routes, err := netSnapshot()
	if err != nil {
		t.Skip(err)
	}
	for _, i := range ifs {
		if i.Up {
			t.Logf("iface %q (%s) tunnel=%v %v", i.Name, i.Desc, i.Tunnel, i.Prefixes)
		}
	}
	for _, r := range routes {
		if r.Dst.Contains(netip.MustParseAddr("1.1.1.1")) {
			t.Logf("route %v if=%d metric=%d main=%v", r.Dst, r.ID, r.Metric, r.Main)
		}
	}
	a, ok := pickTunAddr(usedPrefixes(ifs, routes, 0))
	t.Logf("tunnel address: %v (free: %v)", a, ok)
	c, other := findConflict()
	t.Logf("conflict: %q (other VPN: %v)", c, other)
}
