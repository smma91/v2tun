package main

import (
	"net/netip"
	"strings"
	"sync"
	"time"

	"golang.org/x/net/dns/dnsmessage"
)

// dnsMap remembers which domain each IP was resolved for, from the DNS answers
// that pass through the tunnel. It lets domain rules apply to connections that
// only carry an IP (QUIC, games, apps without TLS).
type dnsMap struct {
	mu sync.Mutex
	m  map[netip.Addr]dnsEntry
}

type dnsEntry struct {
	domain string
	until  time.Time
}

func newDNSMap() *dnsMap { return &dnsMap{m: map[netip.Addr]dnsEntry{}} }

func (d *dnsMap) lookup(ip netip.Addr) string {
	d.mu.Lock()
	defer d.mu.Unlock()
	e, ok := d.m[ip]
	if !ok || time.Now().After(e.until) {
		return ""
	}
	return e.domain
}

// record parses a DNS response and stores its A/AAAA answers.
func (d *dnsMap) record(resp []byte) {
	var p dnsmessage.Parser
	h, err := p.Start(resp)
	if err != nil || !h.Response {
		return
	}
	q, err := p.Question()
	if err != nil {
		return
	}
	p.SkipAllQuestions()
	name := strings.TrimSuffix(strings.ToLower(q.Name.String()), ".")
	now := time.Now()
	d.mu.Lock()
	defer d.mu.Unlock()
	if len(d.m) > 50000 {
		for k, e := range d.m {
			if now.After(e.until) {
				delete(d.m, k)
			}
		}
	}
	for {
		rh, err := p.AnswerHeader()
		if err != nil {
			return
		}
		ttl := time.Duration(max(rh.TTL, 300)) * time.Second // keep at least 5 min: apps cache longer than the TTL
		switch rh.Type {
		case dnsmessage.TypeA:
			r, err := p.AResource()
			if err != nil {
				return
			}
			d.m[netip.AddrFrom4(r.A)] = dnsEntry{name, now.Add(ttl)}
		case dnsmessage.TypeAAAA:
			r, err := p.AAAAResource()
			if err != nil {
				return
			}
			d.m[netip.AddrFrom16(r.AAAA)] = dnsEntry{name, now.Add(ttl)}
		default:
			if p.SkipAnswer() != nil {
				return
			}
		}
	}
}

// questionName returns the queried domain of a DNS request.
func questionName(q []byte) string {
	var p dnsmessage.Parser
	if _, err := p.Start(q); err != nil {
		return ""
	}
	qq, err := p.Question()
	if err != nil {
		return ""
	}
	return strings.TrimSuffix(strings.ToLower(qq.Name.String()), ".")
}
