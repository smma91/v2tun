//go:build windows || linux

package main

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"testing"

	box "github.com/sagernet/sing-box"
	"github.com/sagernet/sing-box/include"
	"github.com/sagernet/sing-box/option"
	sjson "github.com/sagernet/sing/common/json"
)

// validate asks sing-box itself whether it accepts the outbound / endpoint.
func validate(t *testing.T, s *Server) {
	t.Helper()
	up, _ := findUplink(nil)
	raw, _, err := coreConfig(19999, up, []*Server{s}, "")
	if err != nil {
		t.Errorf("%s: config: %v", s.Name, err)
		return
	}
	ctx := include.Context(context.Background())
	opts, err := sjson.UnmarshalExtendedContext[option.Options](ctx, raw)
	if err != nil {
		t.Errorf("%s: sing-box rejects options: %v", s.Name, err)
		return
	}
	inst, err := box.New(box.Options{Context: ctx, Options: opts})
	if err != nil {
		t.Errorf("%s: sing-box rejects config: %v", s.Name, err)
		return
	}
	inst.Close()
	t.Logf("OK %-14s %-22s %s", s.Type, s.Name, s.Addr)
}

func TestNewFormats(t *testing.T) {
	noCronet := ensureCronet() != nil
	if noCronet && platform == "windows" {
		t.Fatal("libcronet.dll missing")
	}
	for _, l := range []string{
		"naive+https://user:pass@naive.example.com:443#naive",
		"naive+quic://user:pass@naive.example.com:443#naive-quic",
		"anyconnect://me:pw@vpn.example.com?group=Staff#corp",
		"fortinet://me:pw@fw.example.com:10443#forti",
	} {
		s := newServer(l, "")
		if s.Error != "" {
			t.Errorf("%s: %s", l, s.Error)
			continue
		}
		if noCronet && s.Type == "naive" { // Linux: libcronet.so is shipped next to the program, not the test
			continue
		}
		validate(t, s)
	}

	clash := `
proxies:
  - {name: "vm-ws", type: vmess, server: a.com, port: 443, uuid: 11111111-1111-1111-1111-111111111111, alterId: 0, cipher: auto, tls: true, servername: a.com, network: ws, ws-opts: {path: /ws, headers: {Host: a.com}}}
  - {name: "vl-reality", type: vless, server: 1.2.3.4, port: 443, uuid: 11111111-1111-1111-1111-111111111111, flow: xtls-rprx-vision, tls: true, servername: www.speedtest.net, client-fingerprint: chrome, reality-opts: {public-key: AKzyvYfbOd6PTdMmPu87eU8uPAZyqRCD3jUD8Tn0F2k, short-id: ab}}
  - {name: "ss", type: ss, server: 5.6.7.8, port: 8388, cipher: aes-256-gcm, password: pw}
  - {name: "trojan-grpc", type: trojan, server: t.com, port: 443, password: pw, network: grpc, grpc-opts: {grpc-service-name: gs}, sni: t.com}
  - {name: "hy2", type: hysteria2, server: h.com, port: 443, password: pw, sni: h.com, obfs: salamander, obfs-password: x}
  - {name: "tuic", type: tuic, server: t.com, port: 443, uuid: 11111111-1111-1111-1111-111111111111, password: pw, alpn: [h3], congestion-controller: bbr}
  - {name: "wg", type: wireguard, server: 9.9.9.9, port: 51820, ip: 10.0.0.2, private-key: 0ClzLbtD3xFQhwKA8iXqNtLqJ5vSdV2tO6MW6Vc8b1s=, public-key: bmXOC+F1FxEMF9dyiK2H5/1SUtzH0JuVo51h2wPfgyo=, mtu: 1280}
  - {name: "ssh", type: ssh, server: s.com, port: 22, username: root, password: pw}
`
	list := splitLinks(clash)
	if len(list) != 8 {
		t.Fatalf("clash: got %d proxies", len(list))
	}
	for _, l := range list {
		validate(t, newServer(l, "sub"))
	}

	// Real OpenVPN Connect profiles on this machine.
	profiles, _ := filepath.Glob(filepath.Join(os.Getenv("APPDATA"), "OpenVPN Connect", "profiles", "*.ovpn"))
	for _, p := range profiles {
		link, err := ovpnFileToLink(p, "testuser", "testpass")
		if err != nil {
			t.Logf("ovpn %s: %v", filepath.Base(p), err)
			continue
		}
		var m map[string]any
		json.Unmarshal([]byte(link), &m)
		validate(t, newServer(link, ""))
	}
}
