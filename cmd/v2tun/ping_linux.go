//go:build linux

package main

import (
	"context"
	"errors"
	"net"
	"net/netip"
	"os"
	"time"

	"golang.org/x/net/icmp"
	"golang.org/x/net/ipv4"
)

// icmpPing sends one echo request out of the real interface (raw socket,
// bound to it and marked, so it bypasses the tunnel and the kill switch).
func icmpPing(src, dst netip.Addr, timeout time.Duration) (int, error) {
	if !dst.Is4() {
		return 0, errors.New("icmp: IPv4 only")
	}
	up, err := findUplink(nil)
	if err != nil {
		return 0, err
	}
	lc := net.ListenConfig{Control: bindToInterface(up.Name)}
	pc, err := lc.ListenPacket(context.Background(), "ip4:icmp", "0.0.0.0")
	if err != nil {
		return 0, err
	}
	defer pc.Close()
	id := os.Getpid() & 0xffff
	seq := int(time.Now().UnixNano() & 0xffff)
	msg := icmp.Message{Type: ipv4.ICMPTypeEcho, Body: &icmp.Echo{ID: id, Seq: seq, Data: []byte("v2tun-ping")}}
	b, err := msg.Marshal(nil)
	if err != nil {
		return 0, err
	}
	start := time.Now()
	if _, err := pc.WriteTo(b, &net.IPAddr{IP: dst.AsSlice()}); err != nil {
		return 0, err
	}
	pc.SetReadDeadline(start.Add(timeout))
	buf := make([]byte, 1500)
	for {
		n, from, err := pc.ReadFrom(buf)
		if err != nil {
			return 0, errors.New("no reply")
		}
		if a, ok := from.(*net.IPAddr); !ok || !a.IP.Equal(dst.AsSlice()) {
			continue
		}
		m, err := icmp.ParseMessage(1, buf[:n])
		if err != nil || m.Type != ipv4.ICMPTypeEchoReply {
			continue
		}
		if e, ok := m.Body.(*icmp.Echo); ok && e.Seq == seq {
			return max(int(time.Since(start).Milliseconds()), 1), nil
		}
	}
}
