//go:build windows

package main

import (
	"encoding/json"
	"net/netip"
	"os"
	"path/filepath"
	"regexp"
	"strings"
)

// Proxy cores v2rayN can run. Their own traffic (to the proxy server, and
// "direct" rules such as geoip:ir) must bypass the tunnel, otherwise it loops.
var coreNames = map[string]bool{
	"xray.exe": true, "v2ray.exe": true, "sing-box.exe": true, "mihomo.exe": true,
	"clash.exe": true, "clash-meta.exe": true, "hysteria.exe": true, "hysteria2.exe": true,
	"tuic-client.exe": true, "naive.exe": true, "juicity-client.exe": true, "brook.exe": true,
	"shadowquic.exe": true,
}

func findV2rayNDir() string {
	if p := processPathByName("v2rayN.exe"); p != "" {
		return filepath.Dir(p)
	}
	home, _ := os.UserHomeDir()
	var pats []string
	for _, base := range []string{`C:\`, `D:\`, os.Getenv("ProgramFiles"), home,
		filepath.Join(home, "Desktop"), filepath.Join(home, "Downloads"), filepath.Join(home, "Documents")} {
		pats = append(pats, filepath.Join(base, "v2rayN*", "v2rayN.exe"), filepath.Join(base, "v2rayN*", "*", "v2rayN.exe"))
	}
	// Several copies are common: take the one used most recently.
	best, bestMod := "", int64(-1)
	for _, p := range pats {
		m, _ := filepath.Glob(p)
		for _, exe := range m {
			d := filepath.Dir(exe)
			if mod := configModTime(d); mod > bestMod {
				best, bestMod = d, mod
			}
		}
	}
	return best
}

// coreExecutables lists every proxy core shipped with v2rayN.
func coreExecutables(dir string) []string {
	var out []string
	if dir == "" {
		return out
	}
	filepath.WalkDir(filepath.Join(dir, "bin"), func(p string, d os.DirEntry, err error) error {
		if err == nil && !d.IsDir() && coreNames[strings.ToLower(d.Name())] {
			out = append(out, p)
		}
		return nil
	})
	return out
}

func isCorePath(p string) bool {
	return p != "" && coreNames[strings.ToLower(filepath.Base(p))]
}

var yamlServerRE = regexp.MustCompile(`(?m)^\s*-?\s*server:\s*["']?([^"'\s#]+)`)

// proxyServers returns the server hosts of the configuration v2rayN is running.
func proxyServers(dir string) []string {
	if dir == "" {
		return nil
	}
	seen := map[string]bool{}
	var out []string
	add := func(h string) {
		h = strings.Trim(strings.TrimSpace(h), "[]")
		if h == "" || seen[h] || h == "localhost" {
			return
		}
		if a, err := netip.ParseAddr(h); err == nil && (a.IsLoopback() || a.IsPrivate() || a.IsUnspecified()) {
			return
		}
		seen[h] = true
		out = append(out, h)
	}
	for _, name := range []string{"config.json", "configPre.json"} {
		b, err := os.ReadFile(filepath.Join(dir, "binConfigs", name))
		if err != nil {
			continue
		}
		var root map[string]any
		if json.Unmarshal(b, &root) != nil {
			continue
		}
		obs, _ := root["outbounds"].([]any)
		for _, o := range obs {
			m, _ := o.(map[string]any)
			kind, _ := m["protocol"].(string)
			if kind == "" {
				kind, _ = m["type"].(string)
			}
			switch kind {
			case "freedom", "blackhole", "dns", "direct", "block", "selector", "urltest", "loopback":
				continue
			}
			walkServers(o, add)
		}
	}
	if b, err := os.ReadFile(filepath.Join(dir, "binConfigs", "config.yaml")); err == nil {
		for _, m := range yamlServerRE.FindAllStringSubmatch(string(b), -1) {
			add(m[1])
		}
	}
	return out
}

// walkServers finds "address" (xray/v2ray) and "server" (sing-box) fields.
func walkServers(v any, add func(string)) {
	switch t := v.(type) {
	case map[string]any:
		for k, x := range t {
			if s, ok := x.(string); ok && (k == "address" || k == "server") {
				add(s)
				continue
			}
			walkServers(x, add)
		}
	case []any:
		for _, x := range t {
			walkServers(x, add)
		}
	}
}

func configModTime(dir string) int64 {
	var newest int64
	for _, n := range []string{"config.json", "configPre.json", "config.yaml"} {
		if fi, err := os.Stat(filepath.Join(dir, "binConfigs", n)); err == nil && fi.ModTime().Unix() > newest {
			newest = fi.ModTime().Unix()
		}
	}
	return newest
}
