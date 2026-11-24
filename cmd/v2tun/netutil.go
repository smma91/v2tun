//go:build windows || linux

package main

import (
	"fmt"
	"net"
	"net/netip"
	"strings"
	"time"

	"golang.org/x/net/dns/dnsmessage"
)

// resolveDirect resolves A records outside the tunnel (used to pin the proxy
// server name, which the core must be able to resolve without the tunnel).
func resolveDirect(u uplink, host string) ([]netip.Addr, error) {
	if a, err := netip.ParseAddr(host); err == nil {
		return []netip.Addr{a}, nil
	}
	name, err := dnsmessage.NewName(strings.TrimSuffix(host, ".") + ".")
	if err != nil {
		return nil, err
	}
	servers := append(append([]netip.Addr{}, u.DNS...), netip.MustParseAddr("1.1.1.1"), netip.MustParseAddr("8.8.8.8"))
	var lastErr error
	for _, srv := range servers {
		ips, err := queryA(u, srv, name)
		if err == nil && len(ips) > 0 {
			return ips, nil
		}
		lastErr = err
	}
	if lastErr == nil {
		lastErr = fmt.Errorf("no A record for %s", host)
	}
	return nil, lastErr
}

func queryA(u uplink, server netip.Addr, name dnsmessage.Name) ([]netip.Addr, error) {
	msg := dnsmessage.Message{
		Header:    dnsmessage.Header{ID: uint16(time.Now().UnixNano()), RecursionDesired: true},
		Questions: []dnsmessage.Question{{Name: name, Type: dnsmessage.TypeA, Class: dnsmessage.ClassINET}},
	}
	q, err := msg.Pack()
	if err != nil {
		return nil, err
	}
	pc, err := directListenUDP(u)
	if err != nil {
		return nil, err
	}
	defer pc.Close()
	pc.SetDeadline(time.Now().Add(3 * time.Second))
	if _, err := pc.WriteTo(q, net.UDPAddrFromAddrPort(netip.AddrPortFrom(server, 53))); err != nil {
		return nil, err
	}
	buf := make([]byte, 1500)
	n, _, err := pc.ReadFrom(buf)
	if err != nil {
		return nil, err
	}
	var resp dnsmessage.Message
	if err := resp.Unpack(buf[:n]); err != nil {
		return nil, err
	}
	var out []netip.Addr
	for _, a := range resp.Answers {
		if r, ok := a.Body.(*dnsmessage.AResource); ok {
			out = append(out, netip.AddrFrom4(r.A))
		}
	}
	return out, nil
}
