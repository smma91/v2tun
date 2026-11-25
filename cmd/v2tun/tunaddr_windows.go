//go:build windows

package main

import (
	"net/netip"
	"unsafe"

	"golang.org/x/sys/windows"
)

const ifTypePropVirtual = 53 // Wintun, TAP and most VPN adapters

// netSnapshot reads the IPv4 adapters and routes (read-only).
func netSnapshot() ([]netIface, []netRoute, error) {
	size := uint32(32 * 1024)
	var buf []byte
	for i := 0; ; i++ {
		buf = make([]byte, size)
		err := windows.GetAdaptersAddresses(windows.AF_INET, 0, 0, (*windows.IpAdapterAddresses)(unsafe.Pointer(&buf[0])), &size)
		if err == nil {
			break
		}
		if err != windows.ERROR_BUFFER_OVERFLOW || i > 4 {
			return nil, nil, err
		}
	}
	var ifs []netIface
	for p := (*windows.IpAdapterAddresses)(unsafe.Pointer(&buf[0])); p != nil; p = p.Next {
		desc := windows.UTF16PtrToString(p.Description)
		i := netIface{
			Name:   windows.UTF16PtrToString(p.FriendlyName),
			Desc:   desc,
			ID:     p.Luid,
			Up:     p.OperStatus == windows.IfOperStatusUp,
			Tunnel: p.IfType == ifTypePropVirtual || p.IfType == windows.IF_TYPE_TUNNEL || vpnDescription(desc),
		}
		for u := p.FirstUnicastAddress; u != nil; u = u.Next {
			if ip, ok := netip.AddrFromSlice(u.Address.IP()); ok && ip.Unmap().Is4() {
				if pfx, err := ip.Unmap().Prefix(int(u.OnLinkPrefixLength)); err == nil {
					i.Prefixes = append(i.Prefixes, pfx)
				}
			}
		}
		ifs = append(ifs, i)
	}

	var tbl *windows.MibIpForwardTable2
	if err := windows.GetIpForwardTable2(windows.AF_INET, &tbl); err != nil {
		return ifs, nil, err
	}
	defer windows.FreeMibTable(unsafe.Pointer(tbl))
	ifMetric := map[uint64]uint32{}
	var routes []netRoute
	for _, r := range tbl.Rows() {
		m, ok := ifMetric[r.InterfaceLuid]
		if !ok {
			row := windows.MibIpInterfaceRow{Family: windows.AF_INET, InterfaceLuid: r.InterfaceLuid}
			if windows.GetIpInterfaceEntry(&row) == nil {
				m = row.Metric
			}
			ifMetric[r.InterfaceLuid] = m
		}
		routes = append(routes, netRoute{
			Dst:    netip.PrefixFrom(sockaddrV4(&r.DestinationPrefix.Prefix), int(r.DestinationPrefix.PrefixLength)),
			ID:     r.InterfaceLuid,
			Metric: r.Metric + m,
			Main:   true,
		})
	}
	return ifs, routes, nil
}
