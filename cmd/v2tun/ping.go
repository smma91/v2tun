//go:build windows || linux

package main

import (
	"context"
	"net"
	"net/netip"
	"strconv"
	"strings"
	"sync"
	"time"
)

// udpProtocols have no TCP handshake to time; they are pinged with ICMP.
var udpProtocols = map[string]bool{"hysteria": true, "hysteria2": true, "tuic": true, "wireguard": true}

// tcpPing times the TCP handshake to the server over the real interface.
func tcpPing(up uplink, ip netip.Addr, port int, timeout time.Duration) (int, error) {
	d := directDialer(up)
	d.Timeout = timeout
	start := time.Now()
	c, err := d.DialContext(context.Background(), "tcp4", net.JoinHostPort(ip.String(), strconv.Itoa(port)))
	if err != nil {
		return 0, err
	}
	ms := int(time.Since(start).Milliseconds())
	c.Close()
	return max(ms, 1), nil
}

func splitAddr(addr string) (string, int) {
	addr = strings.TrimPrefix(addr, "https://")
	if i := strings.IndexByte(addr, '/'); i >= 0 {
		addr = addr[:i]
	}
	if h, p, err := net.SplitHostPort(addr); err == nil {
		return strings.Trim(h, "[]"), atoiDef(p, 443)
	}
	return addr, 443
}

// pingServers measures every server's ping (ms; -1 = no answer).
func pingServers(up uplink, servers []*Server) map[string]int {
	out := map[string]int{}
	var mu sync.Mutex
	var wg sync.WaitGroup
	sem := make(chan struct{}, 16)
	dns := map[string][]netip.Addr{}
	var dnsMu sync.Mutex
	resolve := func(h string) []netip.Addr {
		dnsMu.Lock()
		defer dnsMu.Unlock()
		if ips, ok := dns[h]; ok {
			return ips
		}
		ips, _ := resolveDirect(up, h)
		dns[h] = ips
		return ips
	}
	for _, s := range servers {
		if s.Addr == "" {
			continue
		}
		wg.Add(1)
		go func(s *Server) {
			defer wg.Done()
			sem <- struct{}{}
			defer func() { <-sem }()
			host, port := splitAddr(s.Addr)
			ms := -1
			if ips := resolve(host); len(ips) > 0 {
				var err error
				if udpProtocols[s.Type] {
					ms, err = icmpPing(up.Addr, ips[0], 3*time.Second)
				} else {
					ms, err = tcpPing(up, ips[0], port, 3*time.Second)
				}
				if err != nil {
					ms = -1
				}
			}
			mu.Lock()
			out[s.ID] = ms
			mu.Unlock()
		}(s)
	}
	wg.Wait()
	return out
}
