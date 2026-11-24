package main

import (
	"encoding/binary"
	"net/netip"
	"strings"
	"sync"

	"golang.org/x/net/dns/dnsmessage"
)

// FakeIP ("remote DNS"): Windows' DNS questions are answered locally with an
// address from a reserved range; when an app then connects to that address,
// the tunnel sends the *domain name* to the proxy server, which resolves it
// itself. DNS therefore never leaves the PC and leak tests see the server's
// own resolver, in the same place as the exit IP.
//
// 198.18.0.0/15 is reserved for benchmarking (RFC 2544) and never routed on
// the internet; the tunnel's default route covers it.
var fakePrefix = netip.MustParsePrefix("198.18.0.0/15")

func isFake(ip netip.Addr) bool { return ip.Is4() && fakePrefix.Contains(ip) }

type fakeIP struct {
	mu     sync.Mutex
	next   uint32
	byName map[string]netip.Addr
	byIP   map[netip.Addr]string
}

func newFakeIP() *fakeIP {
	return &fakeIP{next: 1, byName: map[string]netip.Addr{}, byIP: map[netip.Addr]string{}}
}

// get returns the stable fake address of a name (reusing old ones when the
// pool of 131 070 addresses is exhausted).
func (f *fakeIP) get(name string) netip.Addr {
	name = strings.ToLower(strings.TrimSuffix(name, "."))
	f.mu.Lock()
	defer f.mu.Unlock()
	if ip, ok := f.byName[name]; ok {
		return ip
	}
	base := fakePrefix.Addr().As4()
	size := uint32(1) << (32 - fakePrefix.Bits())
	n := binary.BigEndian.Uint32(base[:]) + f.next
	f.next++
	if f.next >= size-1 {
		f.next = 1
	}
	var b [4]byte
	binary.BigEndian.PutUint32(b[:], n)
	ip := netip.AddrFrom4(b)
	if old, ok := f.byIP[ip]; ok {
		delete(f.byName, old)
	}
	f.byName[name], f.byIP[ip] = ip, name
	return ip
}

func (f *fakeIP) lookup(ip netip.Addr) (string, bool) {
	f.mu.Lock()
	defer f.mu.Unlock()
	n, ok := f.byIP[ip]
	return n, ok
}

// fakeReply answers DNS query q locally, or returns nil when the query must
// be forwarded. A gets a fake address; AAAA and HTTPS/SVCB get an empty
// answer (IPv6 is not tunnelled, and HTTPS records would carry real
// addresses that skip the name).
func (f *fakeIP) fakeReply(q []byte) (resp []byte, name string) {
	var p dnsmessage.Parser
	h, err := p.Start(q)
	if err != nil {
		return nil, ""
	}
	qq, err := p.Question()
	if err != nil || qq.Class != dnsmessage.ClassINET {
		return nil, ""
	}
	name = strings.TrimSuffix(strings.ToLower(qq.Name.String()), ".")
	if name == "" || !strings.Contains(name, ".") || strings.HasSuffix(name, ".local") || strings.HasSuffix(name, ".arpa") {
		return nil, ""
	}
	switch qq.Type {
	case dnsmessage.TypeA, dnsmessage.TypeAAAA, dnsmessage.Type(64), dnsmessage.Type(65):
	default:
		return nil, ""
	}
	b := dnsmessage.NewBuilder(nil, dnsmessage.Header{ID: h.ID, Response: true, RecursionDesired: h.RecursionDesired, RecursionAvailable: true})
	b.EnableCompression()
	b.StartQuestions()
	b.Question(qq)
	b.StartAnswers()
	if qq.Type == dnsmessage.TypeA {
		ip := f.get(name)
		b.AResource(dnsmessage.ResourceHeader{Name: qq.Name, Class: dnsmessage.ClassINET, TTL: 60}, dnsmessage.AResource{A: ip.As4()})
	}
	out, err := b.Finish()
	if err != nil {
		return nil, ""
	}
	return out, name
}
