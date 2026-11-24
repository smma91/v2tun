//go:build windows

package main

import (
	"net"
	"net/netip"
	"sync"
	"time"

	"github.com/insomniacslk/dhcp/dhcpv4"
)

// dhcpServer is a minimal DHCP server for the hotspot, used only when
// Windows' connection sharing service does not hand out addresses itself.
// Devices get .10-.250 of the hotspot range, this computer as the router and
// public resolvers as DNS (their queries are routed into the tunnel too).
type dhcpServer struct {
	conn   *net.UDPConn
	self   netip.Addr
	subnet netip.Prefix
	dns    []netip.Addr

	mu     sync.Mutex
	leases map[string]netip.Addr // MAC -> address
}

const dhcpLease = 12 * time.Hour

func startDHCP(self netip.Addr, subnet netip.Prefix, dns []netip.Addr) (*dhcpServer, error) {
	c, err := net.ListenUDP("udp4", &net.UDPAddr{IP: self.AsSlice(), Port: 67})
	if err != nil {
		return nil, err
	}
	d := &dhcpServer{conn: c, self: self, subnet: subnet, dns: dns, leases: map[string]netip.Addr{}}
	go d.serve()
	return d, nil
}

func (d *dhcpServer) Close() { d.conn.Close() }

func (d *dhcpServer) serve() {
	defer recoverPanic("dhcp")
	buf := make([]byte, 1500)
	for {
		n, _, err := d.conn.ReadFromUDP(buf)
		if err != nil {
			return
		}
		req, err := dhcpv4.FromBytes(buf[:n])
		if err != nil || req.OpCode != dhcpv4.OpcodeBootRequest {
			continue
		}
		var typ dhcpv4.MessageType
		switch req.MessageType() {
		case dhcpv4.MessageTypeDiscover:
			typ = dhcpv4.MessageTypeOffer
		case dhcpv4.MessageTypeRequest:
			typ = dhcpv4.MessageTypeAck
		default:
			continue
		}
		ip := d.lease(req.ClientHWAddr.String())
		if !ip.IsValid() {
			continue
		}
		if typ == dhcpv4.MessageTypeAck {
			if want := req.RequestedIPAddress(); want != nil && !want.IsUnspecified() && !want.Equal(ip.AsSlice()) {
				typ = dhcpv4.MessageTypeNak
			}
		}
		mask := net.CIDRMask(d.subnet.Bits(), 32)
		var dns []net.IP
		for _, a := range d.dns {
			dns = append(dns, a.AsSlice())
		}
		resp, err := dhcpv4.NewReplyFromRequest(req,
			dhcpv4.WithMessageType(typ),
			dhcpv4.WithServerIP(d.self.AsSlice()),
			dhcpv4.WithOption(dhcpv4.OptServerIdentifier(d.self.AsSlice())),
			dhcpv4.WithYourIP(ip.AsSlice()),
			dhcpv4.WithNetmask(mask),
			dhcpv4.WithRouter(d.self.AsSlice()),
			dhcpv4.WithDNS(dns...),
			dhcpv4.WithLeaseTime(uint32(dhcpLease.Seconds())),
		)
		if err != nil {
			continue
		}
		if typ == dhcpv4.MessageTypeNak {
			resp.YourIPAddr = net.IPv4zero
		}
		// The device has no address yet: answer by broadcast.
		d.conn.WriteToUDP(resp.ToBytes(), &net.UDPAddr{IP: net.IPv4bcast, Port: 68})
	}
}

// lease returns the device's address (the same one again if it asks again).
func (d *dhcpServer) lease(mac string) netip.Addr {
	d.mu.Lock()
	defer d.mu.Unlock()
	if ip, ok := d.leases[mac]; ok {
		return ip
	}
	used := map[netip.Addr]bool{}
	for _, ip := range d.leases {
		used[ip] = true
	}
	b := d.subnet.Addr().As4()
	for i := 10; i <= 250; i++ {
		b[3] = byte(i)
		if ip := netip.AddrFrom4(b); !used[ip] && ip != d.self {
			d.leases[mac] = ip
			return ip
		}
	}
	return netip.Addr{}
}
