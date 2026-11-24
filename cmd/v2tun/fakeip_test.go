package main

import (
	"testing"

	"golang.org/x/net/dns/dnsmessage"
)

func query(t *testing.T, name string, typ dnsmessage.Type) []byte {
	n, _ := dnsmessage.NewName(name)
	b, err := (&dnsmessage.Message{Header: dnsmessage.Header{ID: 42, RecursionDesired: true},
		Questions: []dnsmessage.Question{{Name: n, Type: typ, Class: dnsmessage.ClassINET}}}).Pack()
	if err != nil {
		t.Fatal(err)
	}
	return b
}

func TestFakeIP(t *testing.T) {
	f := newFakeIP()
	resp, name := f.fakeReply(query(t, "www.YouTube.com.", dnsmessage.TypeA))
	var m dnsmessage.Message
	if err := m.Unpack(resp); err != nil || m.ID != 42 || len(m.Answers) != 1 {
		t.Fatalf("A reply: %v %+v", err, m)
	}
	ip := m.Answers[0].Body.(*dnsmessage.AResource).A
	a := f.get("www.youtube.com")
	if a.As4() != ip || !isFake(a) || name != "www.youtube.com" {
		t.Fatalf("mapping %v %v %q", a, ip, name)
	}
	if n, ok := f.lookup(a); !ok || n != "www.youtube.com" {
		t.Fatalf("lookup %q", n)
	}
	if f.get("other.com") == a {
		t.Fatal("two names, one address")
	}
	resp, _ = f.fakeReply(query(t, "www.youtube.com.", dnsmessage.TypeAAAA))
	m = dnsmessage.Message{}
	if m.Unpack(resp) != nil || len(m.Answers) != 0 || m.RCode != dnsmessage.RCodeSuccess {
		t.Fatalf("AAAA must be empty NOERROR: %+v", m)
	}
	for _, q := range [][]byte{query(t, "example.com.", dnsmessage.TypeMX), query(t, "printer.local.", dnsmessage.TypeA), query(t, "1.0.168.192.in-addr.arpa.", dnsmessage.TypePTR)} {
		if r, _ := f.fakeReply(q); r != nil {
			t.Fatal("must be forwarded, not faked")
		}
	}
}
