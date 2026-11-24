//go:build windows

package main

import (
	"context"
	"encoding/binary"
	"errors"
	"io"
	"net"
	"net/netip"
	"strings"
	"sync"
	"time"

	"golang.org/x/net/dns/dnsmessage"

	M "github.com/xjasonlyu/tun2socks/v2/metadata"
)

// dnsTCP resolves through the proxy with DNS-over-TCP. Many proxy servers
// do not relay UDP at all, so plain DNS over SOCKS-UDP would just time out.
// Connections are kept and reused, and answers are cached, because every new
// TCP connection costs a full round trip through the proxy server.
type dnsTCP struct {
	d *routerDialer

	mu   sync.Mutex
	idle map[netip.Addr][]idleConn

	cacheMu sync.Mutex
	cache   map[string]dnsCached
}

type dnsCached struct {
	resp  []byte
	until time.Time
}

func newDNSTCP(d *routerDialer) *dnsTCP {
	return &dnsTCP{d: d, idle: map[netip.Addr][]idleConn{}, cache: map[string]dnsCached{}}
}

func cacheKey(q []byte) string {
	var p dnsmessage.Parser
	if _, err := p.Start(q); err != nil {
		return ""
	}
	qq, err := p.Question()
	if err != nil {
		return ""
	}
	return strings.ToLower(qq.Name.String()) + "/" + qq.Type.String()
}

func minTTL(resp []byte) time.Duration {
	var p dnsmessage.Parser
	if _, err := p.Start(resp); err != nil {
		return 0
	}
	p.SkipAllQuestions()
	ttl := uint32(3600)
	n := 0
	for {
		h, err := p.AnswerHeader()
		if err != nil {
			break
		}
		n++
		ttl = min(ttl, h.TTL)
		p.SkipAnswer()
	}
	if n == 0 {
		return 30 * time.Second // NXDOMAIN / empty: short negative cache
	}
	return time.Duration(max(ttl, 60)) * time.Second
}

type idleConn struct {
	net.Conn
	since time.Time
}

// Proxy servers drop idle TCP connections without telling us; never reuse
// one that has been idle for long.
const dnsIdleMax = 15 * time.Second

// get returns a pooled connection (reused=true) or a new one.
func (c *dnsTCP) get(server netip.Addr, fresh bool) (conn net.Conn, reused bool, err error) {
	c.mu.Lock()
	for !fresh && len(c.idle[server]) > 0 {
		l := c.idle[server]
		ic := l[len(l)-1]
		c.idle[server] = l[:len(l)-1]
		if time.Since(ic.since) < dnsIdleMax {
			c.mu.Unlock()
			return ic.Conn, true, nil
		}
		ic.Close()
	}
	c.mu.Unlock()
	conn, err = c.dial(server)
	return conn, false, err
}

func (c *dnsTCP) flush(server netip.Addr) {
	c.mu.Lock()
	defer c.mu.Unlock()
	for _, ic := range c.idle[server] {
		ic.Close()
	}
	delete(c.idle, server)
}

func (c *dnsTCP) dial(server netip.Addr) (net.Conn, error) {
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	return c.d.dialTCP(ctx, &M.Metadata{Network: M.TCP, DstIP: server, DstPort: 53}, false, false)
}

func (c *dnsTCP) put(server netip.Addr, conn net.Conn) {
	c.mu.Lock()
	defer c.mu.Unlock()
	if len(c.idle[server]) >= 4 {
		conn.Close()
		return
	}
	c.idle[server] = append(c.idle[server], idleConn{conn, time.Now()})
}

func exchange(conn net.Conn, q []byte) ([]byte, error) {
	conn.SetDeadline(time.Now().Add(8 * time.Second))
	msg := make([]byte, 2+len(q))
	binary.BigEndian.PutUint16(msg, uint16(len(q)))
	copy(msg[2:], q)
	if _, err := conn.Write(msg); err != nil {
		return nil, err
	}
	var l [2]byte
	if _, err := io.ReadFull(conn, l[:]); err != nil {
		return nil, err
	}
	resp := make([]byte, binary.BigEndian.Uint16(l[:]))
	if _, err := io.ReadFull(conn, resp); err != nil {
		return nil, err
	}
	if len(resp) < 2 || resp[0] != q[0] || resp[1] != q[1] {
		return nil, errors.New("dns id mismatch")
	}
	conn.SetDeadline(time.Time{})
	return resp, nil
}

// query resolves q (raw DNS message) at server through the proxy.
func (c *dnsTCP) query(server netip.Addr, q []byte) ([]byte, error) {
	if len(q) < 12 {
		return nil, errors.New("short dns query")
	}
	key := server.String() + " " + cacheKey(q)
	c.cacheMu.Lock()
	if e, ok := c.cache[key]; ok && time.Now().Before(e.until) {
		c.cacheMu.Unlock()
		resp := append([]byte(nil), e.resp...)
		resp[0], resp[1] = q[0], q[1] // answer with the caller's query id
		return resp, nil
	}
	c.cacheMu.Unlock()

	var lastErr error
	fresh := false
	for try := 0; try < 3; try++ {
		conn, reused, err := c.get(server, fresh)
		if err != nil {
			return nil, err
		}
		resp, err := exchange(conn, q)
		if err != nil {
			conn.Close()
			lastErr = err
			if reused { // the pool is probably stale: drop it, use a new connection
				c.flush(server)
				fresh = true
			}
			continue
		}
		c.put(server, conn)
		c.cacheMu.Lock()
		if len(c.cache) > 5000 {
			c.cache = map[string]dnsCached{}
		}
		c.cache[key] = dnsCached{resp: append([]byte(nil), resp...), until: time.Now().Add(minTTL(resp))}
		c.cacheMu.Unlock()
		return resp, nil
	}
	return nil, lastErr
}

func (c *dnsTCP) close() {
	c.mu.Lock()
	defer c.mu.Unlock()
	for _, l := range c.idle {
		for _, ic := range l {
			ic.Close()
		}
	}
	c.idle = map[netip.Addr][]idleConn{}
}
