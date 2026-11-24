package main

import (
	"crypto/tls"
	"net"
	"net/netip"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestRules(t *testing.T) {
	r := compileRules(Settings{
		AppMode: modeExclude, Apps: []string{"telegram.exe", osPath(`C:\Games\game.exe`)},
		DomainMode: modeExclude, Domains: []string{".lan", "*.example.org", "10.0.0.0/8", "example.com"},
	})
	ip := netip.MustParseAddr("1.2.3.4")
	cases := []struct {
		domain string
		ip     netip.Addr
		direct bool
	}{
		{"www.example.com", ip, true}, {"example.com", ip, true}, {"notexample.com", ip, false},
		{"nas.lan", ip, true}, {"example.org", ip, true}, {"google.com", ip, false},
		{"", netip.MustParseAddr("10.1.2.3"), true}, {"", ip, false},
	}
	for _, c := range cases {
		if got := r.domainDirect(c.domain, c.ip); got != c.direct {
			t.Errorf("domain %q %s: got %v", c.domain, c.ip, got)
		}
	}
	if !r.appDirect(osPath(`C:\Users\x\AppData\Telegram.exe`)) || !r.appDirect(osPath(`c:\games\GAME.exe`)) || r.appDirect(osPath(`C:\x\chrome.exe`)) {
		t.Error("app exclude")
	}
	inc := compileRules(Settings{AppMode: modeInclude, Apps: []string{"chrome.exe"}, DomainMode: modeInclude, Domains: []string{"google.com"}})
	if inc.appDirect(osPath(`C:\x\chrome.exe`)) || !inc.appDirect(osPath(`C:\x\telegram.exe`)) {
		t.Error("app include")
	}
	if inc.domainDirect("mail.google.com", ip) || !inc.domainDirect("bing.com", ip) {
		t.Error("domain include")
	}
	if empty := compileRules(Settings{AppMode: modeInclude, DomainMode: modeInclude}); empty.appDirect("x.exe") || empty.domainDirect("a.com", ip) {
		t.Error("empty lists must not bypass anything")
	}
}

func TestSniff(t *testing.T) {
	c, s := net.Pipe()
	go tls.Client(c, &tls.Config{ServerName: "www.example.net"}).Handshake()
	s.SetReadDeadline(time.Now().Add(2 * time.Second))
	buf := make([]byte, 4096)
	n, _ := s.Read(buf)
	if h := sniffHost(buf[:n]); h != "www.example.net" {
		t.Fatalf("sni = %q", h)
	}
	if h := sniffHost([]byte("GET / HTTP/1.1\r\nHost: shop.example.com:8080\r\n\r\n")); h != "shop.example.com" {
		t.Fatalf("host = %q", h)
	}
}

// osPath turns a Windows test path into this system's form (on Linux:
// C:\Games\game.exe -> /c/Games/game.exe).
func osPath(p string) string {
	if filepath.Separator == '\\' {
		return p
	}
	return "/" + strings.ReplaceAll(strings.Replace(p, ":", "", 1), "\\", "/")
}
