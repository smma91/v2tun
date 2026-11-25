package mobile

import (
	"encoding/json"
	"strings"
	"testing"

	"github.com/sagernet/sing-box/experimental/libbox"

	"v2tun/internal/profile"
)

// Every combination of settings must give a configuration sing-box accepts.
func TestBuildConfig(t *testing.T) {
	Init(t.TempDir(), []byte(`{}`))
	n, err := AddText(strings.Join([]string{
		"ss://YWVzLTI1Ni1nY206cHc@5.6.7.8:8388#ss",
		"vless://11111111-1111-1111-1111-111111111111@v.example.com:443?security=reality&sni=www.microsoft.com&pbk=Z84J2IelR9ch3k8VtlVhhs5ycBUlXA7wHBWcBLvjFT8&sid=ab&fp=chrome&type=tcp#reality",
		"hysteria2://auth@h.example.com:443?sni=h.example.com#hy2",
	}, "\n"))
	if err != nil || n != 3 {
		t.Fatalf("added %d: %v", n, err)
	}
	cases := []Settings{
		defaultSettings(),
		{LocalDNS: true, AllowQUIC: true, ProxyLAN: true, ProxyPort: 10808},
		{AppMode: "include", Apps: []string{"com.android.chrome"}, DomainMode: "include", Domains: []string{"google.com", ".youtube.com", "8.8.8.8", "10.0.0.0/8"}},
		{AppMode: "exclude", Apps: []string{"org.telegram.messenger"}, DomainMode: "exclude", Domains: []string{"*.example.org", "192.168.0.0/16"}},
	}
	for i, c := range cases {
		b, _ := json.Marshal(c)
		if err := SaveSettings(string(b)); err != nil {
			t.Fatal(err)
		}
		cfg, err := BuildConfig()
		if err != nil {
			t.Fatalf("case %d: %v", i, err)
		}
		if err := libbox.CheckConfig(cfg); err != nil {
			t.Errorf("case %d rejected by sing-box: %v\n%s", i, err, cfg)
		}
	}
}

func TestCheckConfigCatches(t *testing.T) {
	if err := libbox.CheckConfig(`{"inbounds":[{"type":"tun","stack":"nope"}],"route":{"rules":[{"action":"frobnicate"}]}}`); err == nil {
		t.Fatal("CheckConfig accepted a broken config")
	} else {
		t.Log("broken config rejected:", err)
	}
}

func TestSplitRules(t *testing.T) {
	ips, suf := splitRules([]string{"Example.com", ".com", "*.example.org", "10.0.0.0/8", "8.8.8.8", "# note", ""})
	if strings.Join(ips, ",") != "10.0.0.0/8,8.8.8.8/32" || strings.Join(suf, ",") != "example.com,com,example.org" {
		t.Fatalf("ips %v suffixes %v", ips, suf)
	}
}

func TestNoServers(t *testing.T) {
	Init(t.TempDir(), nil)
	if _, err := BuildConfig(); err == nil {
		t.Fatal("no servers must be an error")
	}
	_ = profile.TestURL
}
