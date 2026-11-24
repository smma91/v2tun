//go:build windows

package main

import (
	"net/netip"
	"os"
	"path/filepath"
	"sort"
	"strings"
)

const (
	hostsBegin = "# >>> v2tun (auto, removed on disconnect)"
	hostsEnd   = "# <<< v2tun"
)

func hostsPath() string {
	return filepath.Join(os.Getenv("SystemRoot"), "System32", "drivers", "etc", "hosts")
}

// pinHosts writes the proxy server names into the hosts file. While the tunnel
// is up all DNS goes through the proxy, so the core itself could never resolve
// its own server without this (chicken-and-egg).
func pinHosts(pins map[string][]netip.Addr) error {
	b, err := os.ReadFile(hostsPath())
	if err != nil {
		return err
	}
	text := stripPins(string(b))
	if len(pins) > 0 {
		var names []string
		for n := range pins {
			names = append(names, n)
		}
		sort.Strings(names)
		var sb strings.Builder
		sb.WriteString(strings.TrimRight(text, "\r\n"))
		sb.WriteString("\r\n" + hostsBegin + "\r\n")
		for _, n := range names {
			for _, ip := range pins[n] {
				sb.WriteString(ip.String() + " " + n + "\r\n")
			}
		}
		sb.WriteString(hostsEnd + "\r\n")
		text = sb.String()
	}
	if err := os.WriteFile(hostsPath(), []byte(text), 0o644); err != nil {
		return err
	}
	run("ipconfig", "/flushdns")
	return nil
}

func unpinHosts() error {
	b, err := os.ReadFile(hostsPath())
	if err != nil {
		return err
	}
	if !strings.Contains(string(b), hostsBegin) {
		return nil
	}
	if err := os.WriteFile(hostsPath(), []byte(stripPins(string(b))), 0o644); err != nil {
		return err
	}
	run("ipconfig", "/flushdns")
	return nil
}

func stripPins(s string) string {
	for {
		i := strings.Index(s, hostsBegin)
		if i < 0 {
			return s
		}
		j := strings.Index(s[i:], hostsEnd)
		if j < 0 {
			return strings.TrimRight(s[:i], "\r\n") + "\r\n"
		}
		s = strings.TrimRight(s[:i], "\r\n") + "\r\n" + strings.TrimLeft(s[i+j+len(hostsEnd):], "\r\n")
	}
}
