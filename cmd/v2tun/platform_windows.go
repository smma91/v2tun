//go:build windows

package main

import (
	_ "embed"
	"os"
	"path/filepath"
	"strconv"
)

const platform = "windows"

// v2rayNExe is the v2rayN program name on this system.
const v2rayNExe = "v2rayN.exe"

// Places a portable v2rayN is usually unpacked to.
func v2rayNSearchDirs() []string {
	home, _ := os.UserHomeDir()
	return []string{`C:\`, `D:\`, os.Getenv("ProgramFiles"), home,
		filepath.Join(home, "Desktop"), filepath.Join(home, "Downloads"), filepath.Join(home, "Documents")}
}

// baseDataDir: settings, servers and the log, shared by all users of the PC
// and readable only by administrators (see secure_windows.go).
func baseDataDir() string { return filepath.Join(os.Getenv("ProgramData"), "v2tun") }

// directMark is set on connections that must bypass the tunnel (Linux only;
// Windows binds them to the real interface instead).
const directMark = 0

//go:embed assets/tray_on.ico
var trayIconOn []byte

//go:embed assets/tray_off.ico
var trayIconOff []byte

// exeFilter is the file-type filter of the "pick a program" dialog.
const exeFilter = "*.exe"

// hasPrivileges: the manifest makes Windows start v2tun as administrator.
func hasPrivileges() bool { return true }

// grantPermission: nothing to grant, Windows asks for administrator rights
// when v2tun starts.
func grantPermission(a *App) error { return nil }

const proxyRule = "v2tun local proxy"

// allowProxyLAN opens Windows Firewall for the local proxy when other devices
// may use it (Wi-Fi / hotspot networks are often "public", where incoming
// connections are blocked), and closes it again when they may not.
func allowProxyLAN(port int, on bool) {
	run("netsh", "advfirewall", "firewall", "delete", "rule", "name="+proxyRule)
	exe, err := os.Executable()
	if !on || err != nil {
		return
	}
	for _, proto := range []string{"TCP", "UDP"} {
		run("netsh", "advfirewall", "firewall", "add", "rule", "name="+proxyRule, "dir=in", "action=allow",
			"program="+exe, "protocol="+proto, "localport="+strconv.Itoa(port), "remoteip=localsubnet")
	}
}

// trayAvailable: Windows always has a notification area.
func trayAvailable() bool { return true }

// handleSignals: Windows ends GUI apps through the window messages instead.
func handleSignals(a *App) {}
