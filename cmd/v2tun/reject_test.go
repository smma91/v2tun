//go:build windows || linux

package main

import (
	"net/netip"
	"testing"

	"gvisor.dev/gvisor/pkg/tcpip/checksum"
	"gvisor.dev/gvisor/pkg/tcpip/header"
)

func TestPortUnreachable(t *testing.T) {
	app, srv := netip.MustParseAddr("172.19.233.1"), netip.MustParseAddr("142.250.1.1")
	p := portUnreachable(app, 51000, srv, 443)
	ip := header.IPv4(p)
	if !ip.IsValid(len(p)) || ip.CalculateChecksum() != 0xffff {
		t.Fatal("outer IP header invalid")
	}
	if ip.SourceAddress().String() != "142.250.1.1" || ip.DestinationAddress().String() != "172.19.233.1" || ip.Protocol() != uint8(header.ICMPv4ProtocolNumber) {
		t.Fatalf("outer: %s -> %s proto %d", ip.SourceAddress(), ip.DestinationAddress(), ip.Protocol())
	}
	icmp := header.ICMPv4(ip.Payload())
	if icmp.Type() != header.ICMPv4DstUnreachable || icmp.Code() != header.ICMPv4PortUnreachable {
		t.Fatalf("type %d code %d", icmp.Type(), icmp.Code())
	}
	if checksum.Checksum(icmp, 0) != 0xffff {
		t.Fatal("ICMP checksum")
	}
	inner := header.IPv4(icmp[8:])
	if inner.CalculateChecksum() != 0xffff || inner.SourceAddress().String() != "172.19.233.1" || inner.Protocol() != uint8(header.UDPProtocolNumber) {
		t.Fatal("quoted IP header")
	}
	udp := header.UDP(icmp[8+20:])
	if udp.SourcePort() != 51000 || udp.DestinationPort() != 443 {
		t.Fatalf("quoted ports %d -> %d", udp.SourcePort(), udp.DestinationPort())
	}
}
