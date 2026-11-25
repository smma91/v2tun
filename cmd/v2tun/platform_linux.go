//go:build linux

package main

import (
	_ "embed"
	"os"
	"path/filepath"

	"github.com/godbus/dbus/v5"
	"golang.org/x/sys/unix"
)

const platform = "linux"

// v2rayNExe is the v2rayN program name on this system.
const v2rayNExe = "v2rayN"

// Places v2rayN for Linux is usually installed or unpacked to.
func v2rayNSearchDirs() []string {
	home, _ := os.UserHomeDir()
	return []string{"/opt", "/usr/local", "/usr/lib", home, filepath.Join(home, ".local", "share"),
		filepath.Join(home, "Desktop"), filepath.Join(home, "Downloads"), filepath.Join(home, "Applications")}
}

// baseDataDir: settings, servers and the log of this user (mode 0700).
func baseDataDir() string {
	d, err := os.UserConfigDir()
	if err != nil {
		d = os.TempDir()
	}
	return filepath.Join(d, "v2tun")
}

// directMark ("vt") is set on every connection that must bypass the tunnel:
// the firewall lets marked traffic out on the real interface.
const directMark = 0x7674

//go:embed assets/tray_on.png
var trayIconOn []byte

//go:embed assets/tray_off.png
var trayIconOff []byte

const exeFilter = "*"

// netCaps are the capabilities the tunnel needs: CAP_NET_ADMIN (TUN device,
// routes, firewall, socket marks) and CAP_NET_RAW (binding sockets to the real
// interface, ICMP ping). v2tun gets them as file capabilities, so the window
// runs as the normal user (root GUIs do not work on Wayland).
var netCaps = []uintptr{unix.CAP_NET_ADMIN, unix.CAP_NET_RAW}

// trayAvailable: a StatusNotifierItem host is running (KDE, Xfce, Cinnamon,
// Ubuntu's GNOME with its AppIndicator extension, …).
func trayAvailable() bool {
	conn, err := dbus.SessionBus()
	if err != nil {
		return false
	}
	var has bool
	err = conn.BusObject().Call("org.freedesktop.DBus.NameHasOwner", 0, "org.kde.StatusNotifierWatcher").Store(&has)
	return err == nil && has
}

// allowProxyLAN: nothing to open on Linux (v2tun does not filter incoming
// connections; the kill switch lets replies to them out).
func allowProxyLAN(port int, on bool) {}

func hasPrivileges() bool {
	hdr := unix.CapUserHeader{Version: unix.LINUX_CAPABILITY_VERSION_3}
	var data [2]unix.CapUserData
	if err := unix.Capget(&hdr, &data[0]); err != nil {
		return false
	}
	for _, c := range netCaps {
		if data[c/32].Effective&(1<<(c%32)) == 0 {
			return false
		}
	}
	return true
}
