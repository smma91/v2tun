//go:build linux

package main

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

// Start at login: an XDG autostart entry (works on GNOME, KDE, Xfce, …).
// No password is needed: the network permission is a file capability.
func autostartFile() string {
	d, err := os.UserConfigDir()
	if err != nil {
		return ""
	}
	return filepath.Join(d, "autostart", "v2tun.desktop")
}

func autostartEnabled() bool {
	_, err := os.Stat(autostartFile())
	return err == nil
}

func setAutostart(on bool) error {
	f := autostartFile()
	if f == "" {
		return os.ErrNotExist
	}
	if !on {
		if err := os.Remove(f); err != nil && !os.IsNotExist(err) {
			return err
		}
		return nil
	}
	exe := exePath()
	if strings.ContainsAny(exe, " \"'\\") {
		exe = `"` + strings.ReplaceAll(exe, `"`, `\"`) + `"`
	}
	entry := fmt.Sprintf(`[Desktop Entry]
Type=Application
Name=v2tun
Comment=System-wide tunnel with kill switch
Exec=%s --autostart
Icon=v2tun
Terminal=false
X-GNOME-Autostart-enabled=true
X-GNOME-Autostart-Delay=10
`, exe)
	if err := os.MkdirAll(filepath.Dir(f), 0o755); err != nil {
		return err
	}
	return os.WriteFile(f, []byte(entry), 0o644)
}
