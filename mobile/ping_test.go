package mobile

import (
	"net"
	"testing"
)

func TestTCPPing(t *testing.T) {
	l, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	go func() {
		for {
			c, err := l.Accept()
			if err != nil {
				return
			}
			c.Close()
		}
	}()
	addr := l.Addr().String()
	if ms := tcpPing(net.DefaultResolver, nil, addr); ms < 1 {
		t.Fatalf("open port: got %d", ms)
	}
	l.Close()
	if ms := tcpPing(net.DefaultResolver, nil, addr); ms != -1 {
		t.Fatalf("closed port: got %d", ms)
	}
	for in, want := range map[string]int{"a.com:8443": 8443, "a.com": 443, "[::1]:80": 80, "https://a.com/x": 443} {
		if _, p := splitAddr(in); p != want {
			t.Errorf("%s: port %d, want %d", in, p, want)
		}
	}
}
