package mobile

import (
	"encoding/json"
	"os"
	"path/filepath"

	"v2tun/internal/i18n"
)

// Settings of the app (the subset of the desktop settings that make sense on
// Android, see docs/PLATFORMS.md).
type Settings struct {
	Lang string `json:"lang"` // en, fa, ar, zh

	// DNS: by default the server resolves names (FakeIP); otherwise these
	// servers are asked through the tunnel.
	LocalDNS bool     `json:"localDns"`
	DNS      []string `json:"dns"`

	AllowQUIC bool `json:"allowQuic"` // only if the servers relay UDP

	// Split tunneling. Apps are package names (Android does the app part).
	AppMode    string   `json:"appMode"` // "exclude" (default) or "include"
	Apps       []string `json:"apps"`
	DomainMode string   `json:"domainMode"`
	Domains    []string `json:"domains"`

	// Local SOCKS5 + HTTP proxy, optionally for other devices on the same
	// Wi-Fi or on this phone's hotspot (the Android way to share the tunnel).
	ProxyPort int  `json:"proxyPort"`
	ProxyLAN  bool `json:"proxyLan"`

	AutoConnect bool `json:"autoConnect"` // connect when the device starts
}

func defaultSettings() Settings {
	return Settings{Lang: "en", DNS: []string{"1.1.1.1", "8.8.8.8"}, AppMode: "exclude", DomainMode: "exclude", ProxyPort: 2080}
}

func settingsPath() string { return filepath.Join(dataDir, "settings.json") }

func loadSettings() Settings {
	s := defaultSettings()
	if b, err := os.ReadFile(settingsPath()); err == nil {
		json.Unmarshal(b, &s)
	}
	return normalize(s)
}

func normalize(s Settings) Settings {
	s.Lang = i18n.ValidLang(s.Lang)
	if s.AppMode != "include" {
		s.AppMode = "exclude"
	}
	if s.DomainMode != "include" {
		s.DomainMode = "exclude"
	}
	if s.ProxyPort < 1024 || s.ProxyPort > 65535 { // no root: ports below 1024 are not allowed
		s.ProxyPort = 2080
	}
	if len(s.DNS) == 0 {
		s.DNS = []string{"1.1.1.1", "8.8.8.8"}
	}
	return s
}

// GetSettings returns the settings as JSON.
func GetSettings() string {
	mu.Lock()
	defer mu.Unlock()
	b, _ := json.Marshal(settings)
	return string(b)
}

// SaveSettings stores settings given as JSON (changes to the tunnel apply on
// the next connect).
func SaveSettings(js string) error {
	s := defaultSettings()
	if err := json.Unmarshal([]byte(js), &s); err != nil {
		return err
	}
	s = normalize(s)
	mu.Lock()
	settings = s
	mu.Unlock()
	i18n.SetLang(s.Lang)
	b, _ := json.MarshalIndent(s, "", "  ")
	return os.WriteFile(settingsPath(), b, 0o600)
}
