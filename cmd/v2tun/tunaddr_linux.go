//go:build linux

package main

import (
	"net/netip"
	"strings"

	"github.com/vishvananda/netlink"
	"golang.org/x/sys/unix"
)

// netSnapshot reads the IPv4 interfaces and routes of every table (read-only).
func netSnapshot() ([]netIface, []netRoute, error) {
	links, err := netlink.LinkList()
	if err != nil {
		return nil, nil, err
	}
	byIndex := map[int]int{}
	var ifs []netIface
	for _, l := range links {
		a := l.Attrs()
		t := l.Type()
		byIndex[a.Index] = len(ifs)
		ifs = append(ifs, netIface{
			Name: a.Name,
			Desc: t,
			ID:   uint64(a.Index),
			Up:   a.Flags&unix.IFF_UP != 0 && a.OperState != netlink.OperDown,
			Tunnel: t == "tuntap" || t == "tun" || t == "wireguard" || t == "ipip" || t == "gre" || t == "vti" || t == "xfrm" ||
				strings.HasPrefix(a.Name, "tun") || strings.HasPrefix(a.Name, "wg"),
		})
	}
	if addrs, err := netlink.AddrList(nil, netlink.FAMILY_V4); err == nil {
		for _, x := range addrs {
			n, ok := byIndex[x.LinkIndex]
			ip, ok2 := netip.AddrFromSlice(x.IP.To4())
			if !ok || !ok2 || x.IPNet == nil {
				continue
			}
			ones, _ := x.Mask.Size()
			ifs[n].Prefixes = append(ifs[n].Prefixes, netip.PrefixFrom(ip, ones))
		}
	}
	rs, err := netlink.RouteListFiltered(netlink.FAMILY_V4, &netlink.Route{Table: unix.RT_TABLE_UNSPEC}, netlink.RT_FILTER_TABLE)
	if err != nil {
		return ifs, nil, err
	}
	var routes []netRoute
	for _, r := range rs {
		if r.LinkIndex <= 0 || r.Table == unix.RT_TABLE_LOCAL || r.Type != unix.RTN_UNICAST {
			continue
		}
		pfx := netip.MustParsePrefix("0.0.0.0/0")
		if r.Dst != nil {
			a, ok := netip.AddrFromSlice(r.Dst.IP.To4())
			if !ok {
				continue
			}
			ones, _ := r.Dst.Mask.Size()
			pfx = netip.PrefixFrom(a, ones)
		}
		routes = append(routes, netRoute{Dst: pfx, ID: uint64(r.LinkIndex), Metric: uint32(r.Priority), Main: r.Table == unix.RT_TABLE_MAIN})
	}
	return ifs, routes, nil
}
