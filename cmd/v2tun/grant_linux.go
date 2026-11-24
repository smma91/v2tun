//go:build linux

package main

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"syscall"
	"time"
)

const installDir = "/opt/v2tun"

// grantPermission gives v2tun its network capabilities (asks for the password
// through polkit), then restarts it so they take effect.
//
//   - normal install / unpacked archive: setcap on the program itself
//   - AppImage: it runs from a read-only, nosuid mount where capabilities are
//     ignored, so the program is copied to /opt/v2tun first and started from
//     there (a menu entry is added for it)
func grantPermission(a *App) error {
	if _, err := exec.LookPath("pkexec"); err != nil {
		return trErr("pkexec was not found. Run in a terminal: sudo setcap cap_net_admin,cap_net_raw+ep %s", exePath())
	}
	exe, err := os.Executable()
	if err != nil {
		return err
	}
	target := exe
	script := `setcap cap_net_admin,cap_net_raw+ep "$1"`
	args := []string{"sh", "-c", script, "sh", exe}
	if os.Getenv("APPIMAGE") != "" {
		target = filepath.Join(installDir, "v2tun")
		script = `set -e; mkdir -p ` + installDir + `; install -m 0755 "$1" ` + installDir + `/v2tun; ` +
			`if [ -f "$2" ]; then install -m 0644 "$2" ` + installDir + `/libcronet.so; fi; ` +
			`if [ -f "$3" ]; then install -m 0644 "$3" ` + installDir + `/v2tun.png; fi; ` +
			`setcap cap_net_admin,cap_net_raw+ep ` + installDir + `/v2tun`
		args = []string{"sh", "-c", script, "sh", exe, filepath.Join(filepath.Dir(exe), "libcronet.so"),
			filepath.Join(os.Getenv("APPDIR"), "v2tun.png")}
	}
	if out, err := exec.Command("pkexec", args...).CombinedOutput(); err != nil {
		return fmt.Errorf("%v %s", err, out)
	}
	if target != exe {
		writeMenuEntry(target)
	}
	a.logf("info", "Network permission granted; restarting v2tun")
	// The new capabilities apply to a newly started program only.
	cmd := exec.Command("sh", "-c", `sleep 1; exec "$0" "$@"`, target)
	cmd.SysProcAttr = &syscall.SysProcAttr{Setsid: true}
	if err := cmd.Start(); err != nil {
		return err
	}
	go func() {
		time.Sleep(200 * time.Millisecond)
		a.quit()
	}()
	return nil
}

// writeMenuEntry adds v2tun to the applications menu (for the AppImage copy).
func writeMenuEntry(exe string) {
	d, err := os.UserHomeDir()
	if err != nil {
		return
	}
	dir := filepath.Join(d, ".local", "share", "applications")
	os.MkdirAll(dir, 0o755)
	os.WriteFile(filepath.Join(dir, "v2tun.desktop"), []byte(`[Desktop Entry]
Type=Application
Name=v2tun
Comment=System-wide tunnel with kill switch
Exec=`+exe+`
Icon=`+filepath.Join(installDir, "v2tun.png")+`
Terminal=false
Categories=Network;
`), 0o644)
}
