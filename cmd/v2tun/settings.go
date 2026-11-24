package main

import (
	"crypto/rand"
	"encoding/json"
	"os"
	"path/filepath"
)

type Settings struct {
	SocksAddr     string   `json:"socksAddr"`
	V2rayNDir     string   `json:"v2rayNDir"`
	DNS           []string `json:"dns"`
	KillSwitch    bool     `json:"killSwitch"`
	AllowLAN      bool     `json:"allowLan"`
	MatchTimezone bool     `json:"matchTimezone"`
	AutoConnect   bool     `json:"autoConnect"` // connect as soon as the app starts
	// QUIC (UDP 443/80) is dropped in the tunnel unless allowed, so browsers
	// use TCP; needed when the proxy server does not relay UDP.
	AllowQUIC bool `json:"allowQuic"`

	// ProxySource: "v2rayn" (use v2rayN's SOCKS port) or "builtin" (embedded core).
	ProxySource string `json:"proxySource"`
	Lang        string `json:"lang"` // en (default), fa, ar, zh

	// Local proxy of the built-in core (SOCKS5 + HTTP on one port), like
	// v2rayN's "Local: socks 10808". Mode "proxy" runs only this proxy, with
	// no system tunnel; SystemProxy then points Windows' proxy setting at it.
	Mode        string `json:"mode"` // "tun" (default) or "proxy"
	ProxyPort   int    `json:"proxyPort"`
	ProxyLAN    bool   `json:"proxyLan"` // listen on all interfaces (other devices may use it)
	SystemProxy bool   `json:"systemProxy"`

	// With the kill switch off, IPv6 and DNS outside the tunnel are still
	// blocked unless this is set (stored inverted so the default is "on").
	DisableLeakGuard bool `json:"disableLeakGuard"`

	// LocalDNS turns off remote DNS (FakeIP): names are then resolved by the
	// DNS servers above, through the tunnel. Default: the server resolves.
	LocalDNS    bool `json:"localDns"`
	ExitOnClose bool `json:"exitOnClose"` // false: closing the window keeps running in the tray

	// Wi-Fi hotspot that shares the tunnel with other devices (phones, …)
	// while connected in tunnel mode.
	Hotspot     bool   `json:"hotspot"`
	HotspotSSID string `json:"hotspotSsid"`
	HotspotPass string `json:"hotspotPass"`

	// Split tunnel (see rules.go): mode is "exclude" or "include".
	AppMode    string   `json:"appMode"`
	Apps       []string `json:"apps"`
	DomainMode string   `json:"domainMode"`
	Domains    []string `json:"domains"`
}

func defaultSettings() Settings {
	return Settings{
		SocksAddr:   "127.0.0.1:10808",
		V2rayNDir:   findV2rayNDir(),
		DNS:         []string{"1.1.1.1", "8.8.8.8"},
		KillSwitch:  true,
		Mode:        "tun",
		ProxyPort:   defaultProxyPort,
		SystemProxy: true,
	}
}

func dataDir() string {
	d := baseDataDir()
	if t := os.Getenv("V2TUN_DATADIR"); t != "" { // tests: never touch the real app's state
		d = t
	}
	os.MkdirAll(d, 0o700)
	return d
}

func settingsPath() string { return filepath.Join(dataDir(), "settings.json") }

func loadSettings() Settings {
	s := defaultSettings()
	if b, err := os.ReadFile(settingsPath()); err == nil {
		json.Unmarshal(b, &s)
	}
	if s.SocksAddr == "" {
		s.SocksAddr = "127.0.0.1:10808"
	}
	if len(s.DNS) == 0 {
		s.DNS = []string{"1.1.1.1", "8.8.8.8"}
	}
	if s.V2rayNDir == "" {
		s.V2rayNDir = findV2rayNDir()
	}
	if s.ProxySource != "builtin" {
		s.ProxySource = "v2rayn"
	}
	switch s.Lang {
	case "en", "fa", "ar", "zh":
	default:
		s.Lang = "en"
	}
	if s.AppMode != modeInclude {
		s.AppMode = modeExclude
	}
	if s.DomainMode != modeInclude {
		s.DomainMode = modeExclude
	}
	normalizeProxy(&s)
	if hotspotDefaults(&s) && os.Getenv("V2TUN_DATADIR") == "" {
		saveSettings(s)
	}
	return s
}

const defaultProxyPort = 2080

func normalizeProxy(s *Settings) {
	if s.Mode != "proxy" {
		s.Mode = "tun"
	}
	if s.ProxyPort < 1 || s.ProxyPort > 65535 {
		s.ProxyPort = defaultProxyPort
	}
}

// hotspotDefaults generates the hotspot name and password once (the user can
// change them). Reports whether something was generated.
func hotspotDefaults(s *Settings) bool {
	gen := s.HotspotSSID == "" || len(s.HotspotPass) < 8
	if s.HotspotSSID == "" {
		s.HotspotSSID = "v2tun-" + randomString(4, "ABCDEFGHJKLMNPQRSTUVWXYZ23456789")
	}
	if len(s.HotspotPass) < 8 {
		s.HotspotPass = randomString(10, "abcdefghjkmnpqrstuvwxyz23456789")
	}
	return gen
}

func randomString(n int, alphabet string) string {
	b := make([]byte, n)
	rand.Read(b)
	for i := range b {
		b[i] = alphabet[int(b[i])%len(alphabet)]
	}
	return string(b)
}

func saveSettings(s Settings) error {
	b, _ := json.MarshalIndent(s, "", "  ")
	return writeSecret(settingsPath(), b)
}
