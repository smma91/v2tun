package mobile

import (
	"context"
	"net"
	"net/netip"
	"strconv"
	"strings"
	"sync"
	"syscall"
	"time"
)

// Protector keeps a socket out of the tunnel while the VPN is on
// (VpnService.protect); without a running VPN it does nothing.
type Protector interface {
	Protect(fd int32) bool
}

// udpOnly protocols have no TCP handshake to time.
var udpOnly = map[string]bool{"hysteria": true, "hysteria2": true, "tuic": true, "wireguard": true}

// PingServers times the TCP handshake to every server over the real network
// (dns: the real network's DNS servers, comma separated). Results are stored
// (ms; -1 = no answer) and shown in the list.
func PingServers(p Protector, dns string) {
	control := func(_, _ string, c syscall.RawConn) error {
		if p != nil {
			c.Control(func(fd uintptr) { p.Protect(int32(fd)) })
		}
		return nil
	}
	resolvers := []string{}
	for _, s := range strings.Split(dns, ",") {
		if a, err := netip.ParseAddr(strings.TrimSpace(s)); err == nil {
			resolvers = append(resolvers, net.JoinHostPort(a.String(), "53"))
		}
	}
	resolvers = append(resolvers, "1.1.1.1:53", "8.8.8.8:53")
	resolver := &net.Resolver{
		PreferGo: true,
		Dial: func(ctx context.Context, network, _ string) (net.Conn, error) {
			var last error
			for _, r := range resolvers {
				d := net.Dialer{Timeout: 2 * time.Second, Control: control}
				c, err := d.DialContext(ctx, network, r)
				if err == nil {
					return c, nil
				}
				last = err
			}
			return nil, last
		},
	}

	out := map[string]int{}
	var mu sync.Mutex
	var wg sync.WaitGroup
	sem := make(chan struct{}, 16)
	for _, s := range db.Snapshot().Servers {
		if s.Addr == "" || udpOnly[s.Type] {
			continue
		}
		wg.Add(1)
		go func(id, addr string) {
			defer wg.Done()
			sem <- struct{}{}
			defer func() { <-sem }()
			ms := tcpPing(resolver, control, addr)
			mu.Lock()
			out[id] = ms
			mu.Unlock()
		}(s.ID, s.Addr)
	}
	wg.Wait()
	db.SetPings(out)
}

func tcpPing(r *net.Resolver, control func(string, string, syscall.RawConn) error, addr string) int {
	host, port := splitAddr(addr)
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	ip, err := netip.ParseAddr(host)
	if err != nil {
		ips, err := r.LookupNetIP(ctx, "ip4", host)
		if err != nil || len(ips) == 0 {
			return -1
		}
		ip = ips[0]
	}
	d := net.Dialer{Timeout: 3 * time.Second, Control: control}
	start := time.Now()
	c, err := d.DialContext(ctx, "tcp", net.JoinHostPort(ip.String(), strconv.Itoa(port)))
	if err != nil {
		return -1
	}
	ms := int(time.Since(start).Milliseconds())
	c.Close()
	return max(ms, 1)
}

func splitAddr(addr string) (string, int) {
	addr = strings.TrimPrefix(addr, "https://")
	if i := strings.IndexByte(addr, '/'); i >= 0 {
		addr = addr[:i]
	}
	if h, p, err := net.SplitHostPort(addr); err == nil {
		if n, err := strconv.Atoi(p); err == nil {
			return strings.Trim(h, "[]"), n
		}
		return strings.Trim(h, "[]"), 443
	}
	return addr, 443
}
