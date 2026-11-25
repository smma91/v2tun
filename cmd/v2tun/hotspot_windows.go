//go:build windows

package main

import (
	"bufio"
	"encoding/base64"
	"encoding/binary"
	"errors"
	"fmt"
	"io"
	"net/netip"
	"os"
	"os/exec"
	"strconv"
	"strings"
	"sync/atomic"
	"syscall"
	"time"
	"unicode/utf16"
	"unsafe"
)

// The access point is a Wi-Fi Direct group in "legacy" mode: ordinary phones
// and laptops join it like any Wi-Fi. It needs no Mobile Hotspot settings,
// only a Wi-Fi card with Wi-Fi Direct (almost every card since Windows 10).
// The WinRT publisher lives in a small hidden PowerShell process that ends,
// taking the access point down, as soon as its input closes: when v2tun
// stops it, exits, or is killed.
const apScript = `
$ErrorActionPreference = 'Stop'
try {
  $null = [Windows.Devices.WiFiDirect.WiFiDirectAdvertisementPublisher, Windows.Devices.WiFiDirect, ContentType = WindowsRuntime]
  $null = [Windows.Security.Credentials.PasswordCredential, Windows.Security.Credentials, ContentType = WindowsRuntime]
  $pub = New-Object Windows.Devices.WiFiDirect.WiFiDirectAdvertisementPublisher
  $adv = $pub.Advertisement
  $adv.IsAutonomousGroupOwnerEnabled = $true
  $adv.LegacySettings.IsEnabled = $true
  $adv.LegacySettings.Ssid = $env:V2TUN_AP_SSID
  $cred = New-Object Windows.Security.Credentials.PasswordCredential
  $cred.Password = $env:V2TUN_AP_PASS
  $adv.LegacySettings.Passphrase = $cred
  $pub.Start()
} catch { [Console]::Out.WriteLine("error " + $_.Exception.Message); [Console]::Out.Flush(); exit 1 }
$last = ''
# Stream.ReadAsync really runs in the background (Console.In.ReadLineAsync
# blocks in Windows PowerShell); it completes when v2tun closes the input.
$buf = New-Object byte[] 64
$in = [Console]::OpenStandardInput().ReadAsync($buf, 0, 64)
while ($true) {
  $s = [string]$pub.Status
  if ($s -ne $last) { [Console]::Out.WriteLine("status " + $s); [Console]::Out.Flush(); $last = $s }
  if ($s -eq 'Aborted' -or $s -eq 'Stopped') { break }
  if ($in.Wait(500)) { break }
}
try { $pub.Stop() } catch {}
`

const apPrefix = "192.168.137.0/24" // the address range Windows uses for shared connections

type accessPoint struct {
	cmd    *exec.Cmd
	stdin  io.WriteCloser
	done   atomic.Bool
	index  uint32 // the Wi-Fi Direct virtual adapter
	name   string // its connection name (for connection sharing)
	ics    bool   // Windows connection sharing set up: tunnel -> hotspot
	luid   uint64
	addr   netip.Addr
	subnet netip.Prefix
	dhcp   *dhcpServer // only when Windows itself does not hand out addresses
	dns    *dnsRelay
}

func startAccessPoint(ssid, pass string) (*accessPoint, error) {
	if len(pass) < 8 || len(pass) > 63 {
		return nil, trErr("The hotspot password must have 8 to 63 characters")
	}
	if len(ssid) == 0 || len(ssid) > 32 {
		return nil, trErr("The hotspot name must have 1 to 32 characters")
	}
	enc := utf16.Encode([]rune(apScript))
	b := make([]byte, 2*len(enc))
	for i, c := range enc {
		binary.LittleEndian.PutUint16(b[2*i:], c)
	}
	cmd := exec.Command("powershell.exe", "-NoProfile", "-NonInteractive", "-ExecutionPolicy", "Bypass",
		"-EncodedCommand", base64.StdEncoding.EncodeToString(b))
	cmd.Env = append(os.Environ(), "V2TUN_AP_SSID="+ssid, "V2TUN_AP_PASS="+pass)
	cmd.SysProcAttr = &syscall.SysProcAttr{HideWindow: true, CreationFlags: 0x08000000}
	stdin, err := cmd.StdinPipe()
	if err != nil {
		return nil, err
	}
	stdout, err := cmd.StdoutPipe()
	if err != nil {
		return nil, err
	}
	if err := cmd.Start(); err != nil {
		return nil, err
	}
	ap := &accessPoint{cmd: cmd, stdin: stdin}
	started := make(chan error, 1)
	go func() {
		defer recoverPanic("hotspot")
		sc := bufio.NewScanner(stdout)
		for sc.Scan() {
			l := strings.TrimSpace(sc.Text())
			switch {
			case l == "status Started":
				started <- nil
			case l == "status Aborted":
				started <- trErr("Wi-Fi could not start the hotspot. Turn Wi-Fi on (it does not need to be connected) and try again")
			case strings.HasPrefix(l, "error "):
				started <- fmt.Errorf("Wi-Fi Direct: %s", strings.TrimPrefix(l, "error "))
			}
		}
		cmd.Wait()
		ap.done.Store(true)
		started <- errors.New("hotspot helper ended")
	}()
	select {
	case err = <-started:
	case <-time.After(20 * time.Second):
		err = errors.New("Wi-Fi Direct did not start in time")
	}
	if err != nil {
		ap.Close()
		return nil, err
	}
	if err := ap.findAdapter(); err != nil {
		ap.Close()
		return nil, err
	}
	return ap, nil
}

// findAdapter waits for the Wi-Fi Direct adapter of the group and its address.
// Windows' connection sharing service normally gives it 192.168.137.1 and
// serves DHCP and DNS to the devices; if it does not, v2tun does.
func (ap *accessPoint) findAdapter() error {
	deadline := time.Now().Add(12 * time.Second)
	for time.Now().Before(deadline) {
		as, _ := listAdapters()
		for _, a := range as {
			if !a.Up || !strings.Contains(a.Description, "Wi-Fi Direct Virtual Adapter") {
				continue
			}
			ap.index, ap.luid, ap.name = a.Index, a.LUID, a.Name
			for _, ip := range a.V4 {
				if !ip.IsLinkLocalUnicast() {
					ap.addr, ap.subnet = ip, netip.PrefixFrom(ip, 24).Masked()
					return ap.ensureDHCP()
				}
			}
		}
		time.Sleep(500 * time.Millisecond)
	}
	if ap.index == 0 {
		return errors.New("the Wi-Fi Direct adapter did not appear")
	}
	// No address from Windows: set one and serve DHCP ourselves.
	ap.subnet = netip.MustParsePrefix(apPrefix)
	ap.addr = netip.MustParseAddr("192.168.137.1")
	i := strconv.Itoa(int(ap.index))
	if err := run("netsh", "interface", "ipv4", "set", "address", "name="+i, "source=static", "address=192.168.137.1", "mask=255.255.255.0"); err != nil {
		return err
	}
	return ap.ensureDHCP()
}

// ensureDHCP starts the built-in DHCP server unless Windows already listens
// on the hotspot's DHCP port.
func (ap *accessPoint) ensureDHCP() error {
	time.Sleep(2 * time.Second) // the sharing service binds its port shortly after the address
	if udpListening(ap.addr, 67) {
		return nil
	}
	d, err := startDHCP(ap.addr, ap.subnet, []netip.Addr{netip.MustParseAddr("1.1.1.1"), netip.MustParseAddr("8.8.8.8")})
	if err != nil {
		return fmt.Errorf("DHCP: %w", err)
	}
	ap.dhcp = d
	return nil
}

// startDNS answers the devices' DNS on the hotspot address (after connection
// sharing is set up, which may reconfigure the adapter).
func (ap *accessPoint) startDNS(upstream string, onQuery func(netip.Addr), onFail func(error)) error {
	if ap.dns != nil {
		return nil
	}
	var err error
	for i := 0; i < 10; i++ { // the address may come back a moment after ICS reconfigured it
		if ap.dns, err = startDNSRelay(ap.addr, upstream, onQuery); err == nil {
			ap.dns.onFail = onFail
			return nil
		}
		time.Sleep(500 * time.Millisecond)
	}
	return fmt.Errorf("DNS: %w", err)
}

func (ap *accessPoint) Alive() bool { return !ap.done.Load() }

// Clients counts the devices seen on the hotspot recently (ARP table).
func (ap *accessPoint) Clients() int {
	size := uint32(16 * 1024)
	var buf []byte
	for i := 0; i < 4; i++ {
		buf = make([]byte, size)
		r, _, _ := procGetIpNetTable.Call(uintptr(unsafe.Pointer(&buf[0])), uintptr(unsafe.Pointer(&size)), 0)
		if r == 0 {
			break
		}
		if r != 122 { // ERROR_INSUFFICIENT_BUFFER
			return 0
		}
	}
	n := int(binary.LittleEndian.Uint32(buf))
	count := 0
	for i := 0; i < n; i++ {
		row := buf[4+i*24:]
		if len(row) < 24 {
			break
		}
		idx, typ := binary.LittleEndian.Uint32(row), binary.LittleEndian.Uint32(row[20:])
		ip := netip.AddrFrom4([4]byte(row[16:20]))
		if idx == ap.index && (typ == 3 || typ == 4) && ap.subnet.Contains(ip) && ip != ap.addr && ip.As4()[3] != 255 {
			count++
		}
	}
	return count
}

var procGetIpNetTable = iphlpapi.NewProc("GetIpNetTable")

func (ap *accessPoint) Close() {
	if ap.dhcp != nil {
		ap.dhcp.Close()
	}
	if ap.dns != nil {
		ap.dns.Close()
	}
	ap.stdin.Close() // the helper stops the access point and exits
	go func() {
		time.Sleep(5 * time.Second)
		if !ap.done.Load() && ap.cmd.Process != nil {
			ap.cmd.Process.Kill()
		}
	}()
}

// Windows Internet Connection Sharing (what Mobile Hotspot uses inside): the
// tunnel adapter is the shared ("public") connection and the hotspot adapter
// the home network. ICS gives the devices addresses and DNS, translates their
// traffic onto the tunnel and opens Windows Firewall for it. Set through the
// HNetCfg COM API; an empty public name turns sharing off again.
const icsScript = `
$ErrorActionPreference = 'Stop'
$m = New-Object -ComObject HNetCfg.HNetShare
$pub = $env:V2TUN_ICS_PUBLIC
$priv = $env:V2TUN_ICS_PRIVATE
$conns = @($m.EnumEveryConnection)
foreach ($c in $conns) {
  $p = $m.NetConnectionProps.Invoke($c)
  $cfg = $m.INetSharingConfigurationForINetConnection.Invoke($c)
  if ($cfg.SharingEnabled -and ($pub -eq '' -or $p.Name -eq $pub -or $p.Name -eq $priv -or $cfg.SharingConnectionType -eq 0)) { $cfg.DisableSharing() }
}
if ($pub -ne '') {
  foreach ($c in $conns) {
    $p = $m.NetConnectionProps.Invoke($c)
    $cfg = $m.INetSharingConfigurationForINetConnection.Invoke($c)
    if ($p.Name -eq $pub) { $cfg.EnableSharing(0) }
  }
  foreach ($c in $conns) {
    $p = $m.NetConnectionProps.Invoke($c)
    $cfg = $m.INetSharingConfigurationForINetConnection.Invoke($c)
    if ($p.Name -eq $priv) { $cfg.EnableSharing(1) }
  }
}
'ok'
`

func setICS(public, private string) error {
	enc := utf16.Encode([]rune(icsScript))
	b := make([]byte, 2*len(enc))
	for i, c := range enc {
		binary.LittleEndian.PutUint16(b[2*i:], c)
	}
	cmd := exec.Command("powershell.exe", "-NoProfile", "-NonInteractive", "-ExecutionPolicy", "Bypass",
		"-EncodedCommand", base64.StdEncoding.EncodeToString(b))
	cmd.Env = append(os.Environ(), "V2TUN_ICS_PUBLIC="+public, "V2TUN_ICS_PRIVATE="+private)
	cmd.SysProcAttr = &syscall.SysProcAttr{HideWindow: true, CreationFlags: 0x08000000}
	out, err := cmd.CombinedOutput()
	if err != nil || !strings.Contains(string(out), "ok") {
		msg := strings.TrimSpace(string(out))
		if i := strings.IndexByte(msg, '\n'); i > 0 {
			msg = msg[:i]
		}
		return fmt.Errorf("%v %s", err, msg)
	}
	return nil
}

const relayRule = "v2tun hotspot DNS"

// allowRelay opens Windows Firewall for v2tun's DNS relay on the hotspot
// (the hotspot is an "unidentified", public network).
func allowRelay() {
	exe, err := os.Executable()
	if err != nil {
		return
	}
	run("netsh", "advfirewall", "firewall", "delete", "rule", "name="+relayRule)
	for _, proto := range []string{"UDP", "TCP"} {
		run("netsh", "advfirewall", "firewall", "add", "rule", "name="+relayRule, "dir=in", "action=allow",
			"program="+exe, "protocol="+proto, "localport=53", "localip=192.168.137.0/24")
	}
}

func removeRelayRule() { run("netsh", "advfirewall", "firewall", "delete", "rule", "name="+relayRule) }

// routeHotspot sends the hotspot's traffic into the tunnel: connection sharing
// from the tunnel adapter (or, if Windows refuses it, plain IP forwarding).
func (e *Engine) routeHotspot(ap *accessPoint) error {
	e.mu.Lock()
	defer e.mu.Unlock()
	if !e.running {
		return trErr("Connect first: the hotspot shares the tunnel")
	}
	allowRelay()
	if err := setICS(tunName, ap.name); err != nil {
		e.logf("warn", "Connection sharing (ICS) is not available (%v); using IP forwarding", err)
	} else {
		ap.ics = true
		e.logf("info", "Connection sharing: tunnel → %s", ap.name)
	}
	for _, i := range []uint32{ap.index, e.tunIndex} {
		if err := run("netsh", "interface", "ipv4", "set", "interface", strconv.Itoa(int(i)), "forwarding=enabled"); err != nil {
			return err
		}
	}
	if e.ks != nil {
		if err := e.ks.allowHotspot(ap.luid); err != nil {
			return err
		}
	}
	e.handler.watchHotspot(ap.subnet)
	return nil
}

func (e *Engine) unrouteHotspot(ap *accessPoint) {
	e.mu.Lock()
	defer e.mu.Unlock()
	if ap.ics {
		setICS("", "")
		ap.ics = false
	}
	removeRelayRule()
	run("netsh", "interface", "ipv4", "set", "interface", strconv.Itoa(int(ap.index)), "forwarding=disabled")
	if e.running {
		run("netsh", "interface", "ipv4", "set", "interface", strconv.Itoa(int(e.tunIndex)), "forwarding=disabled")
	}
	if e.ks != nil {
		e.ks.removeHotspot()
	}
}
