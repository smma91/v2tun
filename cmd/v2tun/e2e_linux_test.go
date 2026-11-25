//go:build linux

package main

import (
	"net"
	"net/netip"
	"os"
	"os/exec"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/google/nftables"
)

// End-to-end tests on a real Linux system (root or the network capabilities,
// internet and at least one working server). Run with V2TUN_E2E=1; set
// V2TUN_DATADIR to a folder with a servers.json. Each test removes what it
// set up. V2TUN_E2E_EXE: a built v2tun, for the watchdog test.

func e2e(t *testing.T) {
	if os.Getenv("V2TUN_E2E") == "" {
		t.Skip("set V2TUN_E2E=1")
	}
}

func sh(name string, args ...string) (string, error) {
	out, err := exec.Command(name, args...).CombinedOutput()
	return strings.TrimSpace(string(out)), err
}

func publicIP(extra ...string) (string, error) {
	return sh("curl", append([]string{"-s", "-m", "12", "https://api.ipify.org"}, extra...)...)
}

func tunPresent() bool {
	_, err := os.Stat("/sys/class/net/" + tunName)
	return err == nil
}

func nftTables() []string {
	c, err := nftables.New()
	if err != nil {
		return nil
	}
	ts, _ := c.ListTables()
	var out []string
	for _, t := range ts {
		if t.Name == ksTable || t.Name == dnsTable {
			out = append(out, t.Name)
		}
	}
	return out
}

// pickServer uses V2TUN_SERVER, or a server other than the selected one (the
// host may already be connected through the selected one).
func pickServer(db ServerStore) string {
	if id := os.Getenv("V2TUN_SERVER"); id != "" {
		return id
	}
	for _, s := range db.Usable() {
		if s.ID != db.Selected {
			return s.ID
		}
	}
	return db.Selected
}

func startTunnel(t *testing.T, killSwitch bool) (*Engine, *coreManager, uplink) {
	t.Helper()
	db := loadServerDB().Snapshot()
	if len(db.Usable()) == 0 {
		t.Skip("no servers")
	}
	up, err := (&App{}).coreUplink()
	if err != nil {
		t.Fatal(err)
	}
	core := &coreManager{}
	if err := core.Start(up, db.Usable(), pickServer(db)); err != nil {
		t.Fatal(err)
	}
	e := &Engine{logf: func(l, f string, a ...any) { t.Logf("[%s] "+f, append([]any{l}, a...)...) }}
	s := loadSettings()
	s.ProxySource, s.SocksAddr, s.KillSwitch, s.MatchTimezone, s.AllowLAN = "builtin", core.Addr(), killSwitch, false, false
	if err := e.Start(s); err != nil {
		core.Stop()
		t.Fatal(err)
	}
	return e, core, up
}

func checkClean(t *testing.T, realIP string) {
	t.Helper()
	if tunPresent() {
		t.Error("tun interface still present")
	}
	if n := nftTables(); len(n) != 0 {
		t.Errorf("firewall tables left: %v", n)
	}
	ip, err := publicIP()
	if ip != realIP {
		t.Errorf("internet after stop: %q err=%v (want real %s)", ip, err, realIP)
	}
}

func TestE2EKillSwitchOn(t *testing.T)  { testTunnel(t, true) }
func TestE2EKillSwitchOff(t *testing.T) { testTunnel(t, false) }

func testTunnel(t *testing.T, ks bool) {
	e2e(t)
	realIP, err := publicIP()
	if err != nil || realIP == "" {
		t.Fatalf("no internet before test: %v", err)
	}
	e, core, up := startTunnel(t, ks)
	stopped := false
	defer func() {
		if !stopped {
			e.Stop()
			core.Stop()
		}
	}()
	time.Sleep(2 * time.Second)
	t.Logf("firewall tables: %v", nftTables())

	// 1. Another program's exit IP is the server's.
	exit, err := publicIP()
	t.Logf("real %s, through tunnel %q (err %v)", realIP, exit, err)
	if exit == "" || exit == realIP {
		t.Errorf("exit IP %q: tunnel does not carry traffic or leaks", exit)
	}
	// 2. The app's own exit-IP lookup and leak test.
	info, err := exitInfo()
	t.Logf("app exitInfo: %+v err=%v", info, err)
	if info.IP == "" || info.IP == realIP {
		t.Errorf("app exit IP %q", info.IP)
	}
	lr := leakCheck()
	t.Logf("leak test: %+v", lr)
	if !lr.Pass {
		t.Error("leak test did not pass")
	}
	// 3. Bypass attempt: a program forcing the real interface.
	byp, err := publicIP("--interface", up.Name)
	t.Logf("bypass via %s: %q err=%v", up.Name, byp, err)
	if ks && byp != "" {
		t.Error("kill switch ON but a program reached the internet outside the tunnel")
	}
	// 4. DNS to a resolver on the local network is captured into the tunnel
	// (FakeIP answer), never sent outside.
	for _, d := range append(up.DNS, up.Gateway) {
		if !d.IsPrivate() {
			continue
		}
		out, _ := sh("nslookup", "-timeout=3", "-retry=1", "example.com", d.String())
		t.Logf("DNS via %s:\n%s", d, out)
		if !strings.Contains(out, "198.18.") && !strings.Contains(out, "198.19.") {
			t.Errorf("DNS through %s was not captured", d)
		}
	}
	// 5. Names work through the tunnel (FakeIP + the server resolving them).
	if out, err := sh("curl", "-s", "-o", "/dev/null", "-w", "%{http_code}", "-m", "15", "https://www.youtube.com/"); err != nil || out == "000" {
		t.Errorf("HTTPS by name through the tunnel failed: %q %v", out, err)
	} else {
		t.Logf("https://www.youtube.com/ -> %s", out)
	}
	// 5b. QUIC (UDP 443) is refused at once, not left hanging.
	if c, err := net.Dial("udp4", "1.1.1.1:443"); err == nil {
		c.Write([]byte("quic"))
		c.SetReadDeadline(time.Now().Add(2 * time.Second))
		_, err := c.Read(make([]byte, 64))
		c.Close()
		t.Logf("QUIC attempt: %v", err)
		if err == nil || !strings.Contains(err.Error(), "refused") {
			t.Errorf("QUIC not refused at once: %v", err)
		}
	}
	// 6. IPv6 to the internet is refused (no leak).
	if out, err := sh("curl", "-6", "-s", "-m", "5", "https://api64.ipify.org"); err == nil && out != "" {
		t.Errorf("IPv6 leak: %q", out)
	}
	// 7. Disconnect: fast and clean.
	start := time.Now()
	e.Stop()
	core.Stop()
	stopped = true
	d := time.Since(start)
	t.Logf("disconnect took %v", d.Round(time.Millisecond))
	if d > 6*time.Second {
		t.Errorf("disconnect too slow: %v", d)
	}
	time.Sleep(time.Second)
	checkClean(t, realIP)
}

// The app is killed while connected: the watchdog must remove the firewall.
func TestE2EKilledWhileConnected(t *testing.T) {
	e2e(t)
	if os.Getenv("V2TUN_E2E_CHILD") == "1" {
		startTunnel(t, true)
		time.Sleep(10 * time.Minute)
		return
	}
	exe := os.Getenv("V2TUN_E2E_EXE")
	if exe == "" {
		t.Skip("set V2TUN_E2E_EXE to a built v2tun")
	}
	realIP, err := publicIP()
	if err != nil {
		t.Fatal(err)
	}
	self, _ := os.Executable()
	child := exec.Command(self, "-test.run", "^TestE2EKilledWhileConnected$", "-test.v")
	child.Env = append(os.Environ(), "V2TUN_E2E_CHILD=1")
	if err := child.Start(); err != nil {
		t.Fatal(err)
	}
	wd := exec.Command(exe, "--watchdog", strconv.Itoa(child.Process.Pid))
	wd.Env = os.Environ()
	if err := wd.Start(); err != nil {
		child.Process.Kill()
		t.Fatal(err)
	}
	deadline := time.Now().Add(40 * time.Second)
	for !tunPresent() && time.Now().Before(deadline) {
		time.Sleep(500 * time.Millisecond)
	}
	if !tunPresent() {
		child.Process.Kill()
		t.Fatal("child did not connect")
	}
	time.Sleep(3 * time.Second)
	if n := nftTables(); len(n) != 2 {
		t.Errorf("firewall tables while connected: %v", n)
	}
	child.Process.Kill() // like kill -9
	child.Wait()
	done := make(chan error, 1)
	go func() { done <- wd.Wait() }()
	select {
	case <-done:
	case <-time.After(15 * time.Second):
		t.Error("watchdog did not finish")
	}
	time.Sleep(time.Second)
	if _, ok := loadState(); ok {
		t.Error("state not cleared by watchdog")
	}
	checkClean(t, realIP)
}

// QUIC is refused at once by the tunnel (no proxy server needed: the refusal
// happens before anything is sent to it). A dummy listener stands in for v2rayN.
func TestE2EQUICRefused(t *testing.T) {
	e2e(t)
	l, err := net.Listen("tcp4", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	defer l.Close()
	go func() {
		for {
			c, err := l.Accept()
			if err != nil {
				return
			}
			c.Read(make([]byte, 3)) // SOCKS5 greeting: no authentication
			c.Write([]byte{5, 0})
			c.Close()
		}
	}()
	e := &Engine{logf: func(l, f string, a ...any) { t.Logf("[%s] "+f, append([]any{l}, a...)...) }}
	s := defaultSettings()
	s.ProxySource, s.SocksAddr, s.KillSwitch, s.MatchTimezone = "v2rayn", l.Addr().String(), false, false
	if err := e.Start(s); err != nil {
		t.Fatal(err)
	}
	defer e.Stop()
	time.Sleep(time.Second)
	for _, dst := range []string{"1.1.1.1:443", "8.8.8.8:80"} {
		c, err := net.Dial("udp4", dst)
		if err != nil {
			t.Fatal(err)
		}
		start := time.Now()
		c.Write([]byte("quic"))
		c.SetReadDeadline(time.Now().Add(3 * time.Second))
		_, err = c.Read(make([]byte, 64))
		c.Close()
		t.Logf("%s: %v after %v", dst, err, time.Since(start).Round(time.Millisecond))
		if err == nil || !strings.Contains(err.Error(), "refused") {
			t.Errorf("%s: not refused at once: %v", dst, err)
		}
	}
	// Other UDP is not refused (it is forwarded; here the dummy proxy fails it).
	c, _ := net.Dial("udp4", "1.1.1.1:5353")
	c.Write([]byte("x"))
	c.SetReadDeadline(time.Now().Add(1500 * time.Millisecond))
	_, err = c.Read(make([]byte, 64))
	c.Close()
	if err != nil && strings.Contains(err.Error(), "refused") {
		t.Error("other UDP was refused too")
	}
}

func TestProcIP(t *testing.T) {
	if a, ok := parseProcIP("0100007F"); !ok || a != netip.MustParseAddr("127.0.0.1") {
		t.Fatalf("v4: %v", a)
	}
	if a, ok := parseProcIP("0000000000000000FFFF00000100007F"); !ok || a != netip.MustParseAddr("127.0.0.1") {
		t.Fatalf("mapped: %v", a)
	}
}
