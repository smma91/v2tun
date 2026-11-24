//go:build windows

package main

import (
	"golang.org/x/sys/windows"
	"golang.org/x/sys/windows/registry"
)

// Windows (WinINet) proxy settings of the signed-in user: used by browsers,
// Store apps and most programs that follow the system proxy.
const inetSettingsKey = `Software\Microsoft\Windows\CurrentVersion\Internet Settings`

// proxyBackup is what the user had before v2tun changed the system proxy.
type proxyBackup struct {
	Enable   uint32 `json:"enable"`
	Server   string `json:"server"`
	Override string `json:"override"`
}

var (
	wininet               = windows.NewLazySystemDLL("wininet.dll")
	procInternetSetOption = wininet.NewProc("InternetSetOptionW")
)

const (
	internetOptionSettingsChanged = 39
	internetOptionRefresh         = 37
)

// notifyProxyChanged makes running programs reload the proxy settings now.
func notifyProxyChanged() {
	procInternetSetOption.Call(0, internetOptionSettingsChanged, 0, 0)
	procInternetSetOption.Call(0, internetOptionRefresh, 0, 0)
}

func readSystemProxy() proxyBackup {
	var b proxyBackup
	k, err := registry.OpenKey(registry.CURRENT_USER, inetSettingsKey, registry.QUERY_VALUE)
	if err != nil {
		return b
	}
	defer k.Close()
	if v, _, err := k.GetIntegerValue("ProxyEnable"); err == nil {
		b.Enable = uint32(v)
	}
	b.Server, _, _ = k.GetStringValue("ProxyServer")
	b.Override, _, _ = k.GetStringValue("ProxyOverride")
	return b
}

// Local and private addresses never go through the proxy.
const proxyBypass = "localhost;127.*;10.*;172.16.*;172.17.*;172.18.*;172.19.*;172.20.*;172.21.*;172.22.*;172.23.*;172.24.*;172.25.*;172.26.*;172.27.*;172.28.*;172.29.*;172.30.*;172.31.*;192.168.*;<local>"

func setSystemProxy(server string) error {
	k, err := registry.OpenKey(registry.CURRENT_USER, inetSettingsKey, registry.SET_VALUE)
	if err != nil {
		return err
	}
	defer k.Close()
	if err := k.SetStringValue("ProxyServer", server); err != nil {
		return err
	}
	k.SetStringValue("ProxyOverride", proxyBypass)
	if err := k.SetDWordValue("ProxyEnable", 1); err != nil {
		return err
	}
	notifyProxyChanged()
	return nil
}

func restoreSystemProxy(b proxyBackup) {
	k, err := registry.OpenKey(registry.CURRENT_USER, inetSettingsKey, registry.SET_VALUE)
	if err != nil {
		return
	}
	defer k.Close()
	k.SetDWordValue("ProxyEnable", b.Enable)
	if b.Server == "" {
		k.DeleteValue("ProxyServer")
	} else {
		k.SetStringValue("ProxyServer", b.Server)
	}
	if b.Override == "" {
		k.DeleteValue("ProxyOverride")
	} else {
		k.SetStringValue("ProxyOverride", b.Override)
	}
	notifyProxyChanged()
}
