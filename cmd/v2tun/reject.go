//go:build windows || linux

package main

import (
	"encoding/binary"
	"net/netip"

	"gvisor.dev/gvisor/pkg/buffer"
	"gvisor.dev/gvisor/pkg/tcpip/checksum"
	"gvisor.dev/gvisor/pkg/tcpip/header"
	"gvisor.dev/gvisor/pkg/tcpip/stack"
)

// portUnreachable builds the ICMP "port unreachable" a host sends for a UDP
// packet nobody listens to (RFC 792): it quotes the IP and UDP headers of the
// original packet, which is how the sending app's socket is found.
func portUnreachable(src netip.Addr, srcPort uint16, dst netip.Addr, dstPort uint16) []byte {
	const ipLen, quoted = 20, 28 // quoted: original IP header + 8 bytes of UDP
	p := make([]byte, ipLen+8+quoted)

	// outer IP header: from the destination the app wanted, back to the app
	writeIPv4(p[:ipLen], len(p), uint8(header.ICMPv4ProtocolNumber), dst, src)
	icmp := p[ipLen:]
	icmp[0], icmp[1] = 3, 3 // destination unreachable / port unreachable
	orig := icmp[8:]
	writeIPv4(orig[:ipLen], quoted, uint8(header.UDPProtocolNumber), src, dst)
	binary.BigEndian.PutUint16(orig[20:], srcPort)
	binary.BigEndian.PutUint16(orig[22:], dstPort)
	binary.BigEndian.PutUint16(orig[24:], 8)
	binary.BigEndian.PutUint16(icmp[2:], ^checksum.Checksum(icmp, 0))
	return p
}

func writeIPv4(b []byte, total int, proto uint8, src, dst netip.Addr) {
	b[0] = 0x45 // version 4, 20-byte header
	binary.BigEndian.PutUint16(b[2:], uint16(total))
	b[8], b[9] = 64, proto // TTL, protocol
	s, d := src.As4(), dst.As4()
	copy(b[12:], s[:])
	copy(b[16:], d[:])
	binary.BigEndian.PutUint16(b[10:], ^checksum.Checksum(b[:20], 0))
}

// rejecter answers through the tunnel's network stack, so the packet reaches
// the app (or, through connection sharing, the hotspot device) like any
// reply from the internet.
func rejecter(stk *stack.Stack) func(src netip.Addr, srcPort uint16, dst netip.Addr, dstPort uint16) {
	return func(src netip.Addr, srcPort uint16, dst netip.Addr, dstPort uint16) {
		if !src.Is4() || !dst.Is4() {
			return
		}
		pkt := portUnreachable(src, srcPort, dst, dstPort)
		for id := range stk.NICInfo() {
			stk.WriteRawPacket(id, header.IPv4ProtocolNumber, buffer.MakeWithData(pkt))
		}
	}
}
