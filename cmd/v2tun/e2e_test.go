//go:build windows

package main

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"syscall"
	"testing"
	"time"

	"github.com/tailscale/wf"
)

// End-to-end tests on the real system. They need Administrator rights and a
// working server in the app's list; run with V2TUN_E2E=1. Each test tears the
// tunnel down itself, so the machine is left as it was.

func e2e(t *testing.T) {
	if os.Getenv("V2TUN_E2E") == "" {
		t.Skip("set V2TUN_E2E=1")
	}
}

func sh(name string, args ...string) (string, error) {
	cmd := exec.Command(name, args...)
	cmd.SysProcAttr = &syscall.SysProcAttr{HideWindow: true}
	out, err := cmd.CombinedOutput()
	return strings.TrimSpace(string(out)), err
}

// publicIP asks from an outside program (curl is not allowed by the kill switch).
func publicIP(extra ...string) (string, error) {
	args := append([]string{"-s", "-m", "12", "https://api.ipify.org"}, extra...)
	return sh("curl.exe", args...)
}

func v2tunRules() int {
	s, err := wf.New(&wf.Options{Name: "v2tun-e2e-check"})
	if err != nil {
		return -1
	}
	defer s.Close()
	rules, _ := s.Rules()
	n := 0
	for _, r := range rules {
		if strings.HasPrefix(r.Name, "v2tun: ") {
			n++
		}
	}
	return n
}

func tunPresent() bool {
	as, _ := listAdapters()
	for _, a := range as {
		if a.Name == tunName {
			return true
		}
	}
	return false
}

func hostsClean() bool {
	b, _ := os.ReadFile(hostsPath())
	return !strings.Contains(string(b), hostsBegin)
}

// startTunnel brings up the embedded core and the system tunnel.
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
	if err := core.Start(up, db.Usable(), db.Selected); err != nil {
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
		t.Error("tunnel adapter still present")
	}
	if n := v2tunRules(); n != 0 {
		t.Errorf("%d firewall rules left", n)
	}
	if !hostsClean() {
		t.Error("hosts file still pinned")
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

	// 1. Exit IP from another program: must be the server's, not the real one.
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
	// 3. Bypass attempt: force a program out of the real interface.
	byp, err := publicIP("--interface", up.Addr.String())
	t.Logf("bypass via %s: %q err=%v", up.Addr, byp, err)
	if ks && byp == realIP {
		t.Error("kill switch ON but a program reached the internet outside the tunnel")
	}
	// 4. DNS outside the tunnel (router / ISP resolver on the local network)
	// must be blocked in both modes. Public resolvers like 8.8.8.8 are routed
	// into the tunnel anyway, so only local ones can leak.
	for _, d := range append(up.DNS, up.Gateway) {
		if !d.IsPrivate() {
			continue
		}
		out, _ := sh("nslookup", "-timeout=3", "-retry=1", "example.com", d.String())
		leaked := strings.Contains(out, "Addresses:") || strings.Count(out, "Address:") >= 2
		t.Logf("DNS via %s outside tunnel leaked=%v", d, leaked)
		if leaked {
			t.Errorf("DNS leak through %s", d)
		}
	}
	// 5. DNS through the tunnel works.
	out, _ := sh("nslookup", "-timeout=5", "www.youtube.com")
	if !strings.Contains(out, "Address") || strings.Count(out, "Address") < 2 {
		t.Errorf("DNS through tunnel failed: %s", out)
	}
	// 6. Disconnect: fast and clean.
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

// The app is killed while connected: the watchdog (real v2tun.exe) must
// restore the system.
func TestE2EKilledWhileConnected(t *testing.T) {
	e2e(t)
	if os.Getenv("V2TUN_E2E_CHILD") == "1" { // the process that gets killed
		startTunnel(t, true)
		time.Sleep(10 * time.Minute)
		return
	}
	exe, _ := filepath.Abs(filepath.Join("..", "..", "v2tun.exe"))
	if _, err := os.Stat(exe); err != nil {
		t.Skip("build v2tun.exe first")
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
	if n := v2tunRules(); n == 0 {
		t.Error("kill switch rules missing while connected")
	}
	child.Process.Kill() // like "End task" in Task Manager
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
	fmt.Println("killed child, system clean")
}
