//go:build windows

package main

import (
	"context"
	"errors"
	"fmt"
	"io"
	"net"
	"sync"
	"sync/atomic"
	"time"

	M "github.com/xjasonlyu/tun2socks/v2/metadata"
	"github.com/xjasonlyu/tun2socks/v2/proxy/socks5"
)

// routerDialer sends every connection captured by the tunnel to v2rayN's
// SOCKS port, except connections made by the proxy core itself (its link to
// the server and its "direct" routing rules): those go straight out of the
// real interface. Without this the core's traffic would loop into itself.
type routerDialer struct {
	socks     *socks5.Socks5
	socksAddr string // same proxy, for name-based connections (dialHost)

	mu     sync.RWMutex
	uplink uplink

	proxied, direct atomic.Int64
	onDirect        func(proc string, m *M.Metadata)

	// Bytes of the core's own traffic (it also crosses the tunnel), so the
	// UI can show only what applications really used.
	counter, counterDown *atomic.Int64
}

type countConn struct {
	net.Conn
	up, down *atomic.Int64
}

func (c *countConn) Read(b []byte) (int, error) {
	n, err := c.Conn.Read(b)
	c.down.Add(int64(n))
	return n, err
}

func (c *countConn) Write(b []byte) (int, error) {
	n, err := c.Conn.Write(b)
	c.up.Add(int64(n))
	return n, err
}

func (c *countConn) CloseRead() error {
	if t, ok := c.Conn.(*net.TCPConn); ok {
		return t.CloseRead()
	}
	return nil
}

func (c *countConn) CloseWrite() error {
	if t, ok := c.Conn.(*net.TCPConn); ok {
		return t.CloseWrite()
	}
	return nil
}

type countPacketConn struct {
	net.PacketConn
	up, down *atomic.Int64
}

func (c *countPacketConn) ReadFrom(b []byte) (int, net.Addr, error) {
	n, a, err := c.PacketConn.ReadFrom(b)
	c.down.Add(int64(n))
	return n, a, err
}

func (c *countPacketConn) WriteTo(b []byte, a net.Addr) (int, error) {
	n, err := c.PacketConn.WriteTo(b, a)
	c.up.Add(int64(n))
	return n, err
}

func (d *routerDialer) setUplink(u uplink) {
	d.mu.Lock()
	d.uplink = u
	d.mu.Unlock()
}

func (d *routerDialer) getUplink() uplink {
	d.mu.RLock()
	defer d.mu.RUnlock()
	return d.uplink
}

// ownerIsCore finds the process that opened the connection. The tunnel sees
// the packet after the OS created the socket, so the entry is in the table;
// retry briefly to cover the rare race.
func ownerPath(lookup func() uint32) string {
	for i := 0; i < 3; i++ {
		if pid := lookup(); pid != 0 {
			return pidPath(pid)
		}
		time.Sleep(15 * time.Millisecond)
	}
	return ""
}

// dialHost connects through the SOCKS proxy to a *domain name*, so the proxy
// server resolves it (FakeIP / remote DNS). Minimal SOCKS5 CONNECT, no auth.
func (d *routerDialer) dialHost(ctx context.Context, host string, port uint16) (net.Conn, error) {
	d.proxied.Add(1)
	if len(host) > 255 {
		return nil, errors.New("host name too long")
	}
	var nd net.Dialer
	c, err := nd.DialContext(ctx, "tcp", d.socksAddr)
	if err != nil {
		return nil, err
	}
	if dl, ok := ctx.Deadline(); ok {
		c.SetDeadline(dl)
	}
	fail := func(err error) (net.Conn, error) { c.Close(); return nil, err }
	if _, err := c.Write([]byte{5, 1, 0}); err != nil {
		return fail(err)
	}
	var b [262]byte
	if _, err := io.ReadFull(c, b[:2]); err != nil {
		return fail(err)
	}
	if b[0] != 5 || b[1] != 0 {
		return fail(errors.New("socks: authentication required"))
	}
	req := append([]byte{5, 1, 0, 3, byte(len(host))}, host...)
	req = append(req, byte(port>>8), byte(port))
	if _, err := c.Write(req); err != nil {
		return fail(err)
	}
	if _, err := io.ReadFull(c, b[:4]); err != nil {
		return fail(err)
	}
	if b[1] != 0 {
		return fail(fmt.Errorf("socks: connect to %s failed (code %d)", host, b[1]))
	}
	skip := map[byte]int{1: 4, 4: 16}[b[3]]
	if b[3] == 3 {
		if _, err := io.ReadFull(c, b[:1]); err != nil {
			return fail(err)
		}
		skip = int(b[0])
	}
	if _, err := io.ReadFull(c, b[:skip+2]); err != nil {
		return fail(err)
	}
	c.SetDeadline(time.Time{})
	return c, nil
}

// dialTCP opens the outgoing side of a captured TCP connection.
// core marks the proxy core's own traffic (counted apart in the statistics).
func (d *routerDialer) dialTCP(ctx context.Context, m *M.Metadata, direct, core bool) (net.Conn, error) {
	if !direct {
		d.proxied.Add(1)
		return d.socks.DialContext(ctx, m)
	}
	d.direct.Add(1)
	c, err := directDialer(d.getUplink()).DialContext(ctx, "tcp4", m.DestinationAddress())
	if err != nil || !core {
		return c, err
	}
	return &countConn{Conn: c, up: d.counter, down: d.counterDown}, nil
}

func (d *routerDialer) dialUDP(m *M.Metadata, direct, core bool) (net.PacketConn, error) {
	if !direct {
		d.proxied.Add(1)
		return d.socks.DialUDP(m)
	}
	d.direct.Add(1)
	pc, err := directListenUDP(d.getUplink())
	if err != nil || !core {
		return pc, err
	}
	return &countPacketConn{PacketConn: pc, up: d.counter, down: d.counterDown}, nil
}

// socksAlive checks that v2rayN's SOCKS inbound accepts a handshake.
func socksAlive(addr string) bool {
	c, err := net.DialTimeout("tcp", addr, 2*time.Second)
	if err != nil {
		return false
	}
	defer c.Close()
	c.SetDeadline(time.Now().Add(2 * time.Second))
	if _, err := c.Write([]byte{5, 1, 0}); err != nil {
		return false
	}
	b := make([]byte, 2)
	_, err = c.Read(b)
	return err == nil && b[0] == 5 && b[1] == 0
}
