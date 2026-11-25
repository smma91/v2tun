//go:build windows || linux

package main

import (
	"context"
	"fmt"
	"io"
	"net"
	"net/netip"
	"strconv"
	"sync"
	"sync/atomic"
	"time"

	"github.com/xjasonlyu/tun2socks/v2/core/adapter"
	M "github.com/xjasonlyu/tun2socks/v2/metadata"
	"github.com/xjasonlyu/tun2socks/v2/tunnel/statistic"
	"golang.org/x/net/dns/dnsmessage"
)

// handler receives every TCP/UDP flow captured by the tunnel and decides,
// per connection, whether it goes to v2rayN (SOCKS) or straight out:
//
//  1. the proxy core itself            -> direct (otherwise it loops)
//  2. split tunnel by application      -> direct if the app rule says so
//  3. split tunnel by domain / IP      -> direct if the domain rule says so
//  4. everything else                  -> SOCKS
type handler struct {
	d     *routerDialer
	dns   *dnsMap
	rules atomic.Pointer[rules]
	logf  func(level, format string, args ...any)

	dnsTCP *dnsTCP
	fake   *fakeIP // nil: real DNS through the tunnel instead of remote DNS
	// reject answers a blocked UDP packet with ICMP "port unreachable"
	reject func(src netip.Addr, srcPort uint16, dst netip.Addr, dstPort uint16)

	hotspot     atomic.Pointer[netip.Prefix] // address range of hotspot devices
	hotspotSeen sync.Map                     // device address -> logged
	blockQUIC   atomic.Bool
	open        sync.Map // flows currently in the tunnel

	errMu   sync.Mutex
	errSeen map[string]time.Time
}

// dialFailed logs a failed outgoing connection (at most once a minute per target).
// watchHotspot: connections from this range come from hotspot devices.
func (h *handler) watchHotspot(p netip.Prefix) {
	h.hotspot.Store(&p)
	h.hotspotSeen.Clear()
}

// noteShared: the first connection of hotspot devices through connection
// sharing (logged once per destination port, so the log shows what works).
func (h *handler) noteShared(m *M.Metadata) {
	if _, seen := h.hotspotSeen.LoadOrStore("shared"+strconv.Itoa(int(m.DstPort)), true); !seen && h.logf != nil {
		h.logf("info", "Hotspot devices' traffic reaches the tunnel (port %d)", m.DstPort)
	}
}

func (h *handler) noteHotspot(src netip.Addr) {
	if p := h.hotspot.Load(); p != nil && p.Contains(src) {
		if _, seen := h.hotspotSeen.LoadOrStore(src, true); !seen && h.logf != nil {
			h.logf("info", "Hotspot device %s is using the tunnel", src)
		}
	}
}

func (h *handler) dialFailed(kind string, m *M.Metadata, direct bool, err error) {
	if h.logf == nil {
		return
	}
	key := kind + m.DestinationAddress()
	h.errMu.Lock()
	if h.errSeen == nil {
		h.errSeen = map[string]time.Time{}
	}
	if t, ok := h.errSeen[key]; ok && time.Since(t) < time.Minute {
		h.errMu.Unlock()
		return
	}
	h.errSeen[key] = time.Now()
	h.errMu.Unlock()
	via := "SOCKS"
	if direct {
		via = T("direct")
	}
	h.logf("warn", "%s %s via %s failed: %v", kind, m.DestinationAddress(), via, err)
}

func (h *handler) HandleTCP(c adapter.TCPConn) {
	h.open.Store(c, struct{}{})
	go func() { defer recoverPanic("tcp"); defer h.open.Delete(c); h.tcp(c) }()
}

func (h *handler) HandleUDP(c adapter.UDPConn) {
	h.open.Store(c, struct{}{})
	go func() { defer recoverPanic("udp"); defer h.open.Delete(c); h.udp(c) }()
}

// closeAll ends every flow still open in the tunnel (used on disconnect).
func (h *handler) closeAll() {
	h.open.Range(func(k, _ any) bool {
		if c, ok := k.(io.Closer); ok {
			c.Close()
		}
		return true
	})
}

// resolveReal finds a real IPv4 address for a FakeIP name: outside the tunnel
// for direct traffic, otherwise through the proxy (DNS over TCP).
func (h *handler) resolveReal(name string, direct bool) (netip.Addr, error) {
	if direct {
		ips, err := resolveDirect(h.d.getUplink(), name)
		if err != nil {
			return netip.Addr{}, err
		}
		for _, ip := range ips {
			if ip.Is4() {
				return ip, nil
			}
		}
		return netip.Addr{}, fmt.Errorf("no IPv4 address for %s", name)
	}
	n, err := dnsmessage.NewName(name + ".")
	if err != nil {
		return netip.Addr{}, err
	}
	msg := dnsmessage.Message{Header: dnsmessage.Header{ID: uint16(time.Now().UnixNano()), RecursionDesired: true},
		Questions: []dnsmessage.Question{{Name: n, Type: dnsmessage.TypeA, Class: dnsmessage.ClassINET}}}
	q, err := msg.Pack()
	if err != nil {
		return netip.Addr{}, err
	}
	var resp []byte
	for _, srv := range []string{"1.1.1.1", "8.8.8.8"} {
		if resp, err = h.dnsTCP.query(netip.MustParseAddr(srv), q); err == nil {
			break
		}
	}
	if err != nil {
		return netip.Addr{}, err
	}
	var p dnsmessage.Message
	if err := p.Unpack(resp); err != nil {
		return netip.Addr{}, err
	}
	for _, a := range p.Answers {
		if r, ok := a.Body.(*dnsmessage.AResource); ok {
			return netip.AddrFrom4(r.A), nil
		}
	}
	return netip.Addr{}, fmt.Errorf("no IPv4 address for %s", name)
}

func addr(a []byte) netip.Addr {
	ip, _ := netip.AddrFromSlice(a)
	return ip.Unmap()
}

func (h *handler) tcp(origin adapter.TCPConn) {
	defer origin.Close()
	id := origin.ID()
	m := &M.Metadata{Network: M.TCP, SrcIP: addr(id.RemoteAddress.AsSlice()), SrcPort: id.RemotePort,
		DstIP: addr(id.LocalAddress.AsSlice()), DstPort: id.LocalPort}
	r := h.rules.Load()

	h.noteHotspot(m.SrcIP)
	path := ownerPath(func() uint32 { return tcpOwner(m.SrcIP, m.SrcPort) })
	core := isCorePath(path)
	direct := core || r.appDirect(path)
	shared := path == "" && h.hotspot.Load() != nil && tcpOwner(m.SrcIP, m.SrcPort) == 0
	if shared {
		h.noteShared(m)
	}

	// FakeIP: the destination stands for a domain; the server resolves it.
	var host string
	if h.fake != nil && isFake(m.DstIP) {
		var ok bool
		if host, ok = h.fake.lookup(m.DstIP); !ok {
			return // stale address from before a restart: the app will look it up again
		}
	}

	// Domain rules: name from the DNS answers we saw, else sniff the first
	// packet (TLS SNI / HTTP Host) — that also covers browsers using DoH.
	var first []byte
	if !direct && r.needsDomain() {
		domain := host
		if domain == "" {
			domain = h.dns.lookup(m.DstIP)
		}
		if domain == "" && (m.DstPort == 443 || m.DstPort == 80 || m.DstPort == 8443 || m.DstPort == 8080) {
			buf := make([]byte, 4096)
			origin.SetReadDeadline(time.Now().Add(400 * time.Millisecond))
			n, _ := origin.Read(buf)
			origin.SetReadDeadline(time.Time{})
			first = buf[:n]
			domain = sniffHost(first)
		}
		direct = r.domainDirect(domain, m.DstIP)
	}

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	var remote net.Conn
	var err error
	switch {
	case host != "" && !direct:
		remote, err = h.d.dialHost(ctx, host, m.DstPort) // name resolved by the server
	case host != "": // a direct app reached a fake address: resolve for real, outside
		if m.DstIP, err = h.resolveReal(host, true); err == nil {
			remote, err = h.d.dialTCP(ctx, m, true, core)
		}
	default:
		remote, err = h.d.dialTCP(ctx, m, direct, core)
	}
	cancel()
	if err != nil {
		if shared {
			h.logf("warn", "Hotspot: connection to %s failed: %v", firstNonEmpty(host, m.DestinationAddress()), err)
		}
		h.dialFailed("TCP", m, direct, err)
		return
	}
	remote = statistic.NewTCPTracker(remote, m, statistic.DefaultManager)
	defer remote.Close()
	if len(first) > 0 {
		if _, err := remote.Write(first); err != nil {
			return
		}
	}
	pipe(origin, remote)
}

func pipe(a, b net.Conn) {
	var wg sync.WaitGroup
	wg.Add(2)
	cp := func(dst, src net.Conn) {
		defer wg.Done()
		io.CopyBuffer(dst, src, make([]byte, 32*1024))
		if cw, ok := dst.(interface{ CloseWrite() error }); ok {
			cw.CloseWrite()
		}
		dst.SetReadDeadline(time.Now().Add(60 * time.Second))
	}
	go cp(b, a)
	go cp(a, b)
	wg.Wait()
}

const udpTimeout = 60 * time.Second

func (h *handler) udp(origin adapter.UDPConn) {
	defer origin.Close()
	id := origin.ID()
	m := &M.Metadata{Network: M.UDP, SrcIP: addr(id.RemoteAddress.AsSlice()), SrcPort: id.RemotePort,
		DstIP: addr(id.LocalAddress.AsSlice()), DstPort: id.LocalPort}
	r := h.rules.Load()

	h.noteHotspot(m.SrcIP)
	path := ownerPath(func() uint32 { return udpOwner(m.SrcPort) })
	core := isCorePath(path)
	direct := core || r.appDirect(path)

	if !core && m.DstPort == 53 {
		h.dnsSession(origin, m, direct, r)
		return
	}
	var host string
	if h.fake != nil && isFake(m.DstIP) {
		var ok bool
		if host, ok = h.fake.lookup(m.DstIP); !ok {
			return
		}
	}
	if !direct && r.needsDomain() {
		domain := host
		if domain == "" {
			domain = h.dns.lookup(m.DstIP)
		}
		// QUIC hides the name; without a known name, refuse it so the browser
		// falls back to TCP, where the SNI can be read and the rule applied.
		if domain == "" && m.DstPort == 443 && len(r.suffixes) > 0 {
			return
		}
		direct = r.domainDirect(domain, m.DstIP)
	}
	// QUIC (HTTP/3) through a proxy that does not relay UDP just hangs.
	// Refusing it like a host without that port (ICMP "port unreachable")
	// makes apps switch to TCP at once instead of retrying and waiting
	// (YouTube, Google, Apple services; also saves phones' battery).
	if !direct && h.blockQUIC.Load() && (m.DstPort == 443 || m.DstPort == 80) {
		if h.reject != nil {
			h.reject(m.SrcIP, m.SrcPort, m.DstIP, m.DstPort)
		}
		return
	}
	if host != "" { // UDP needs a real address: resolve it (through the server unless direct)
		ip, err := h.resolveReal(host, direct)
		if err != nil {
			h.dialFailed("UDP", m, direct, err)
			return
		}
		m.DstIP = ip
	}

	pc, err := h.d.dialUDP(m, direct, core)
	if err != nil {
		h.dialFailed("UDP", m, direct, err)
		return
	}
	pc = statistic.NewUDPTracker(pc, m, statistic.DefaultManager)
	defer pc.Close()
	to := m.UDPAddr()
	pipePackets(origin, pc, to)
}

func pipePackets(origin net.PacketConn, remote net.PacketConn, to net.Addr) {
	var wg sync.WaitGroup
	wg.Add(2)
	go func() { // app -> remote
		defer recoverPanic("handler")
		defer wg.Done()
		buf := make([]byte, 65535)
		for {
			origin.SetReadDeadline(time.Now().Add(udpTimeout))
			n, _, err := origin.ReadFrom(buf)
			if err != nil {
				remote.SetReadDeadline(time.Now())
				return
			}
			if _, err := remote.WriteTo(buf[:n], to); err != nil {
				return
			}
		}
	}()
	go func() { // remote -> app
		defer recoverPanic("handler")
		defer wg.Done()
		buf := make([]byte, 65535)
		for {
			remote.SetReadDeadline(time.Now().Add(udpTimeout))
			n, _, err := remote.ReadFrom(buf)
			if err != nil {
				origin.SetReadDeadline(time.Now())
				return
			}
			if _, err := origin.WriteTo(buf[:n], nil); err != nil {
				return
			}
		}
	}()
	wg.Wait()
}

// dnsSession forwards DNS queries one by one, choosing the path per queried
// domain (a bypassed domain is resolved outside the tunnel too, so local CDNs
// answer with local addresses), and records the answers for domain rules.
func (h *handler) dnsSession(origin adapter.UDPConn, m *M.Metadata, appDirect bool, r *rules) {
	to := m.UDPAddr()
	var mu sync.Mutex
	ups := map[bool]net.PacketConn{}
	defer func() {
		mu.Lock()
		for _, pc := range ups {
			pc.Close()
		}
		mu.Unlock()
	}()

	upstream := func(direct bool) (net.PacketConn, error) {
		mu.Lock()
		defer mu.Unlock()
		if pc := ups[direct]; pc != nil {
			return pc, nil
		}
		pc, err := h.d.dialUDP(m, direct, false)
		if err != nil {
			h.dialFailed("DNS", m, direct, err)
			return nil, err
		}
		pc = statistic.NewUDPTracker(pc, m, statistic.DefaultManager)
		ups[direct] = pc
		go func() {
			defer recoverPanic("handler")
			buf := make([]byte, 65535)
			for {
				pc.SetReadDeadline(time.Now().Add(udpTimeout))
				n, _, err := pc.ReadFrom(buf)
				if err != nil {
					return
				}
				h.dns.record(buf[:n])
				origin.WriteTo(buf[:n], nil)
			}
		}()
		return pc, nil
	}

	buf := make([]byte, 65535)
	for {
		origin.SetReadDeadline(time.Now().Add(udpTimeout))
		n, _, err := origin.ReadFrom(buf)
		if err != nil {
			return
		}
		q := append([]byte(nil), buf[:n]...)
		direct := appDirect || r.domainDirect(questionName(q), netip.Addr{})
		if !direct && h.fake != nil {
			// Remote DNS: answer locally, the server resolves the name later.
			if resp, _ := h.fake.fakeReply(q); resp != nil {
				h.dns.record(resp)
				origin.WriteTo(resp, nil)
				continue
			}
		}
		if direct {
			// Outside the tunnel plain UDP DNS works fine.
			if pc, err := upstream(true); err == nil {
				pc.WriteTo(q, to)
			}
			continue
		}
		// Through the proxy: DNS over TCP (the proxy may not carry UDP).
		go func() {
			defer recoverPanic("handler")
			resp, err := h.dnsTCP.query(m.DstIP, q)
			for _, alt := range []string{"1.1.1.1", "8.8.8.8"} { // the asked resolver may be unreachable via this server
				if err == nil {
					break
				}
				if a := netip.MustParseAddr(alt); a != m.DstIP {
					resp, err = h.dnsTCP.query(a, q)
				}
			}
			if err != nil {
				h.dialFailed("DNS", m, false, err)
				return
			}
			h.dns.record(resp)
			origin.WriteTo(resp, nil)
		}()
	}
}
