//go:build linux

package main

import (
	"errors"
	"net"
	"os"
	"os/exec"
	"strings"
)

// proxyBackup is the desktop proxy setting before v2tun changed it.
// GNOME (and desktops using its settings: Cinnamon, Budgie, Unity, …) via
// gsettings; KDE Plasma via kioslaverc. Browsers and most apps follow them.
type proxyBackup struct {
	Desktop string            `json:"desktop"` // "gnome" or "kde"
	Values  map[string]string `json:"values"`
}

var gnomeKeys = [][2]string{
	{"org.gnome.system.proxy", "mode"},
	{"org.gnome.system.proxy", "ignore-hosts"},
	{"org.gnome.system.proxy.http", "host"}, {"org.gnome.system.proxy.http", "port"},
	{"org.gnome.system.proxy.https", "host"}, {"org.gnome.system.proxy.https", "port"},
	{"org.gnome.system.proxy.socks", "host"}, {"org.gnome.system.proxy.socks", "port"},
}

var kdeKeys = []string{"ProxyType", "httpProxy", "httpsProxy", "socksProxy", "NoProxyFor"}

func desktopKind() string {
	d := strings.ToLower(os.Getenv("XDG_CURRENT_DESKTOP"))
	if strings.Contains(d, "kde") {
		return "kde"
	}
	if _, err := exec.LookPath("gsettings"); err == nil {
		return "gnome"
	}
	return ""
}

func kdeConfig() string {
	for _, n := range []string{"kwriteconfig6", "kwriteconfig5"} {
		if _, err := exec.LookPath(n); err == nil {
			return n
		}
	}
	return ""
}

func output(name string, args ...string) string {
	out, _ := exec.Command(name, args...).Output()
	return strings.TrimSpace(string(out))
}

func readSystemProxy() proxyBackup {
	b := proxyBackup{Desktop: desktopKind(), Values: map[string]string{}}
	switch b.Desktop {
	case "gnome":
		for _, k := range gnomeKeys {
			b.Values[k[0]+" "+k[1]] = output("gsettings", "get", k[0], k[1])
		}
	case "kde":
		r := strings.Replace(kdeConfig(), "write", "read", 1)
		for _, k := range kdeKeys {
			b.Values[k] = output(r, "--file", "kioslaverc", "--group", "Proxy Settings", "--key", k)
		}
	}
	return b
}

const proxyBypassList = "['localhost', '127.0.0.0/8', '::1', '10.0.0.0/8', '172.16.0.0/12', '192.168.0.0/16']"

func setSystemProxy(server string) error {
	host, port, err := net.SplitHostPort(server)
	if err != nil {
		return err
	}
	switch desktopKind() {
	case "gnome":
		for _, kv := range [][3]string{
			{"org.gnome.system.proxy.http", "host", host}, {"org.gnome.system.proxy.http", "port", port},
			{"org.gnome.system.proxy.https", "host", host}, {"org.gnome.system.proxy.https", "port", port},
			{"org.gnome.system.proxy.socks", "host", host}, {"org.gnome.system.proxy.socks", "port", port},
			{"org.gnome.system.proxy", "ignore-hosts", proxyBypassList},
			{"org.gnome.system.proxy", "mode", "manual"},
		} {
			if out, err := exec.Command("gsettings", "set", kv[0], kv[1], kv[2]).CombinedOutput(); err != nil {
				return errors.New(strings.TrimSpace(string(out)))
			}
		}
		return nil
	case "kde":
		w := kdeConfig()
		if w == "" {
			return errors.New("kwriteconfig not found")
		}
		for _, kv := range [][2]string{
			{"httpProxy", "http://" + host + " " + port}, {"httpsProxy", "http://" + host + " " + port},
			{"socksProxy", "socks://" + host + " " + port},
			{"NoProxyFor", "localhost,127.0.0.0/8,10.0.0.0/8,172.16.0.0/12,192.168.0.0/16"},
			{"ProxyType", "1"},
		} {
			if out, err := exec.Command(w, "--file", "kioslaverc", "--group", "Proxy Settings", "--key", kv[0], kv[1]).CombinedOutput(); err != nil {
				return errors.New(strings.TrimSpace(string(out)))
			}
		}
		kdeReload()
		return nil
	}
	return errors.New("this desktop has no system proxy setting v2tun knows; set the proxy in your apps")
}

func restoreSystemProxy(b proxyBackup) {
	switch b.Desktop {
	case "gnome":
		for _, k := range gnomeKeys {
			if v := b.Values[k[0]+" "+k[1]]; v != "" {
				exec.Command("gsettings", "set", k[0], k[1], v).Run()
			}
		}
	case "kde":
		if w := kdeConfig(); w != "" {
			for _, k := range kdeKeys {
				if v := b.Values[k]; v != "" {
					exec.Command(w, "--file", "kioslaverc", "--group", "Proxy Settings", "--key", k, v).Run()
				} else {
					exec.Command(w, "--file", "kioslaverc", "--group", "Proxy Settings", "--key", k, "--delete").Run()
				}
			}
			kdeReload()
		}
	}
}

// kdeReload tells running KDE apps that the proxy settings changed.
func kdeReload() {
	exec.Command("dbus-send", "--type=signal", "/KIO/Scheduler", "org.kde.KIO.Scheduler.reparseSlaveConfiguration", "string:").Run()
}
