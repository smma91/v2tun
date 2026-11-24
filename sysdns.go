//go:build windows

package main

import (
	"encoding/json"
	"fmt"
	"net/netip"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"unsafe"

	"golang.org/x/sys/windows"
	"golang.org/x/sys/windows/registry"
)

// DNS presets for the real network connection (outside the tunnel).
var dnsPresets = map[string]struct{ v4, v6 []string }{
	"cloudflare": {[]string{"1.1.1.1", "1.0.0.1"}, []string{"2606:4700:4700::1111", "2606:4700:4700::1001"}},
	"google":     {[]string{"8.8.8.8", "8.8.4.4"}, []string{"2001:4860:4860::8888", "2001:4860:4860::8844"}},
}

type NetAdapter struct {
	Name        string   `json:"name"`
	Description string   `json:"description"`
	Index       uint32   `json:"index"`
	GUID        string   `json:"guid"`
	DNS4        []string `json:"dns4"`
	DNS6        []string `json:"dns6"`
	Static      bool     `json:"static"`
	Preset      string   `json:"preset"` // cloudflare, google, dhcp or custom
	Uplink      bool     `json:"uplink"`
	HasBackup   bool     `json:"hasBackup"`
}

// dnsBackup is what the connection used before we first changed it.
type dnsBackup struct {
	Static4 []string `json:"static4"`
	Static6 []string `json:"static6"`
}

var backupMu sync.Mutex

func backupPath() string { return filepath.Join(dataDir(), "dns-backup.json") }

func loadBackups() map[string]dnsBackup {
	m := map[string]dnsBackup{}
	if b, err := os.ReadFile(backupPath()); err == nil {
		json.Unmarshal(b, &m)
	}
	return m
}

func saveBackups(m map[string]dnsBackup) {
	b, _ := json.MarshalIndent(m, "", "  ")
	writeSecret(backupPath(), b)
}

// staticDNS reads the manually configured servers (empty = automatic/DHCP).
func staticDNS(guid string, v6 bool) []string {
	svc := `Tcpip`
	if v6 {
		svc = `Tcpip6`
	}
	k, err := registry.OpenKey(registry.LOCAL_MACHINE, `SYSTEM\CurrentControlSet\Services\`+svc+`\Parameters\Interfaces\`+guid, registry.QUERY_VALUE)
	if err != nil {
		return nil
	}
	defer k.Close()
	s, _, err := k.GetStringValue("NameServer")
	if err != nil {
		return nil
	}
	return strings.FieldsFunc(s, func(r rune) bool { return r == ',' || r == ' ' })
}

// internetAdapters lists connected adapters that have a default gateway,
// i.e. the connections that can carry internet (Wi-Fi, Ethernet, hotspot...).
func internetAdapters() ([]NetAdapter, error) {
	size := uint32(32 * 1024)
	var buf []byte
	for i := 0; ; i++ {
		buf = make([]byte, size)
		err := windows.GetAdaptersAddresses(windows.AF_UNSPEC, windows.GAA_FLAG_INCLUDE_GATEWAYS, 0, (*windows.IpAdapterAddresses)(unsafe.Pointer(&buf[0])), &size)
		if err == nil {
			break
		}
		if err != windows.ERROR_BUFFER_OVERFLOW || i > 4 {
			return nil, err
		}
	}
	exclude := map[uint64]bool{}
	if all, err := listAdapters(); err == nil {
		for _, x := range all {
			if x.Name == tunName || isTunnelAdapter(x.Name+" "+x.Description) {
				exclude[x.LUID] = true
			}
		}
	}
	up, _ := findUplink(exclude)
	backups := loadBackups()
	var out []NetAdapter
	for p := (*windows.IpAdapterAddresses)(unsafe.Pointer(&buf[0])); p != nil; p = p.Next {
		if p.OperStatus != windows.IfOperStatusUp || p.FirstGatewayAddress == nil || p.IfType == windows.IF_TYPE_SOFTWARE_LOOPBACK {
			continue
		}
		name := windows.UTF16PtrToString(p.FriendlyName)
		desc := windows.UTF16PtrToString(p.Description)
		if name == tunName || isTunnelAdapter(name+" "+desc) {
			continue
		}
		a := NetAdapter{Name: name, Description: desc, Index: p.IfIndex, GUID: windows.BytePtrToString(p.AdapterName)}
		for d := p.FirstDnsServerAddress; d != nil; d = d.Next {
			if ip, ok := netip.AddrFromSlice(d.Address.IP()); ok {
				if ip = ip.Unmap(); ip.Is4() {
					a.DNS4 = append(a.DNS4, ip.String())
				} else if !ip.IsLinkLocalUnicast() && !strings.HasPrefix(ip.String(), "fec0:") {
					a.DNS6 = append(a.DNS6, ip.String())
				}
			}
		}
		a.Static = len(staticDNS(a.GUID, false)) > 0
		a.Preset = presetOf(a)
		a.Uplink = p.Luid == up.LUID
		_, a.HasBackup = backups[a.GUID]
		out = append(out, a)
	}
	return out, nil
}

func isTunnelAdapter(s string) bool {
	s = strings.ToLower(s)
	for _, k := range []string{"tap-windows", "wintun", "wireguard", "openvpn", "anyconnect", "sing-tun", "singbox", "tun"} {
		if strings.Contains(s, k) {
			return true
		}
	}
	return false
}

func presetOf(a NetAdapter) string {
	if !a.Static {
		return "dhcp"
	}
	for name, p := range dnsPresets {
		if strings.Join(a.DNS4, ",") == strings.Join(p.v4, ",") {
			return name
		}
	}
	return "custom"
}

func setDNS(index uint32, family string, servers []string) error {
	i := strconv.Itoa(int(index))
	if len(servers) == 0 {
		return run("netsh", "interface", family, "set", "dnsservers", "name="+i, "source=dhcp")
	}
	if err := run("netsh", "interface", family, "set", "dnsservers", "name="+i, "source=static", "address="+servers[0], "register=primary", "validate=no"); err != nil {
		return err
	}
	for n, s := range servers[1:] {
		if err := run("netsh", "interface", family, "add", "dnsservers", "name="+i, "address="+s, "index="+strconv.Itoa(n+2), "validate=no"); err != nil {
			return err
		}
	}
	return nil
}

// applyAdapterDNS sets the DNS of a real connection: "cloudflare", "google",
// "dhcp" (automatic) or "restore" (what it had before v2tun changed it).
func applyAdapterDNS(index uint32, preset string) error {
	as, err := internetAdapters()
	if err != nil {
		return err
	}
	var a *NetAdapter
	for i := range as {
		if as[i].Index == index {
			a = &as[i]
		}
	}
	if a == nil {
		return trErr("Connection not found or not connected to the internet")
	}

	backupMu.Lock()
	defer backupMu.Unlock()
	backups := loadBackups()

	var v4, v6 []string
	switch preset {
	case "cloudflare", "google":
		v4, v6 = dnsPresets[preset].v4, dnsPresets[preset].v6
	case "dhcp":
	case "restore":
		b, ok := backups[a.GUID]
		if !ok {
			return trErr("There is no backup for this connection")
		}
		v4, v6 = b.Static4, b.Static6
	default:
		return fmt.Errorf("unknown preset %q", preset)
	}

	if _, ok := backups[a.GUID]; !ok && preset != "restore" {
		backups[a.GUID] = dnsBackup{Static4: staticDNS(a.GUID, false), Static6: staticDNS(a.GUID, true)}
		saveBackups(backups)
	}
	if err := setDNS(index, "ipv4", v4); err != nil {
		return err
	}
	// IPv6 may be disabled on the adapter; that is not an error for the user.
	setDNS(index, "ipv6", v6)
	if preset == "restore" {
		delete(backups, a.GUID)
		saveBackups(backups)
	}
	run("ipconfig", "/flushdns")
	return nil
}
