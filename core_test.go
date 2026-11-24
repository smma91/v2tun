//go:build windows

package main

import (
	"encoding/base64"
	"encoding/json"
	"io"
	"net/http"
	"net/url"
	"testing"
	"time"
)

func TestParseLinks(t *testing.T) {
	vm, _ := json.Marshal(map[string]any{"v": "2", "ps": "vm", "add": "a.com", "port": "443", "id": "11111111-1111-1111-1111-111111111111",
		"net": "ws", "path": "/ws?ed=2048", "host": "h.com", "tls": "tls", "sni": "s.com"})
	links := []string{
		"vless://11111111-1111-1111-1111-111111111111@1.2.3.4:443?type=tcp&security=reality&sni=www.speedtest.net&fp=chrome&pbk=PUBKEY&sid=ab&flow=xtls-rprx-vision#Reality%20DE",
		"vless://11111111-1111-1111-1111-111111111111@cdn.com:443?type=ws&security=tls&path=%2Fws&host=h.com&sni=h.com#ws",
		"vless://11111111-1111-1111-1111-111111111111@cdn.com:443?type=grpc&serviceName=gs&security=tls#grpc",
		"vless://11111111-1111-1111-1111-111111111111@cdn.com:443?type=xhttp&security=tls#xhttp",
		"vmess://" + base64.StdEncoding.EncodeToString(vm),
		"trojan://pass@t.com:443?sni=t.com#trojan",
		"ss://" + base64.RawURLEncoding.EncodeToString([]byte("chacha20-ietf-poly1305:pw")) + "@5.6.7.8:8388#ss",
		"ss://" + base64.StdEncoding.EncodeToString([]byte("aes-256-gcm:pw@5.6.7.8:8388")) + "#ss-legacy",
		"hysteria2://auth@h.com:443?sni=h.com&obfs=salamander&obfs-password=x&insecure=1&mport=20000-30000#hy2",
		"hy2://auth@h.com:443#hy2b",
		"tuic://11111111-1111-1111-1111-111111111111:pw@t.com:443?congestion_control=bbr&alpn=h3&sni=t.com#tuic",
		"wireguard://cHJpdmF0ZQ==@9.9.9.9:51820?publickey=cHVibGlj&address=10.0.0.2/32,fd00::2/128&mtu=1280&reserved=1,2,3#wg",
		"ssh://root:secret@ssh.example.com:22#ssh",
		"socks://" + base64.StdEncoding.EncodeToString([]byte("u:p")) + "@1.1.1.1:1080#socks",
		"anytls://pw@a.com:443?sni=a.com#anytls",
	}
	for _, l := range links {
		s := newServer(l, "")
		b, _ := json.Marshal(func() any { ob, _ := s.outbound(); return ob }())
		t.Logf("%-10s %-14s %-22s err=%q %s", s.Type, s.Name, s.Addr, s.Error, b)
	}
	if got := len(splitLinks(base64.StdEncoding.EncodeToString([]byte(links[0] + "\n" + links[5])))); got != 2 {
		t.Fatalf("base64 sub: %d", got)
	}
}

func TestXrayXHTTP(t *testing.T) {
	link := "vless://11111111-1111-1111-1111-111111111111@example.com:443?type=xhttp&security=tls&path=%2Fx&mode=auto&sni=example.com#xhttp"
	s := newServer(link, "")
	if s.Error != "" || s.Type != "vless-xhttp" {
		t.Fatalf("server: %+v", s)
	}
	up, _ := findUplink(nil)
	if _, err := xrays.ensure(s.ID, s.Link, up.Name); err != nil {
		t.Fatalf("xray: %v", err)
	}
	raw, present, err := coreConfig(12345, up, []*Server{s}, "")
	if err != nil || !present[s.ID] {
		t.Fatalf("config: %v", err)
	}
	t.Logf("sing-box sees: %s", raw)
	defer xrays.stopAll()
	if len(xrays.insts) != 1 {
		t.Fatal("xray instance not started")
	}
}

// Runs the embedded core with v2rayN's SOCKS port as its "server".
func TestCoreThroughSocks(t *testing.T) {
	if !socksAlive("127.0.0.1:10808") {
		t.Skip("v2rayN SOCKS not running")
	}
	up, err := findUplink(nil)
	if err != nil {
		t.Fatal(err)
	}
	t.Logf("binding to %q", up.Name)
	srv := newServer("socks://127.0.0.1:10808#v2rayN", "")
	var c coreManager
	if err := c.Start(up, []*Server{srv}, srv.ID); err != nil {
		t.Fatal(err)
	}
	defer c.Stop()
	t.Logf("core listening on %s", c.Addr())
	t.Logf("delays: %v", c.Test([]string{srv.ID}))

	pu, _ := url.Parse("http://" + c.Addr())
	hc := http.Client{Timeout: 15 * time.Second, Transport: &http.Transport{Proxy: http.ProxyURL(pu)}}
	resp, err := hc.Get("https://ipinfo.io/json")
	if err != nil {
		t.Fatal(err)
	}
	b, _ := io.ReadAll(resp.Body)
	resp.Body.Close()
	t.Logf("via core: %s", b)
}
