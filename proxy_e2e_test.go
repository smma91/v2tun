//go:build windows

package main

import (
	"net"
	"strconv"
	"testing"
)

func TestE2ELocalProxy(t *testing.T) {
	e2e(t)
	db := loadServerDB().Snapshot()
	if len(db.Usable()) == 0 {
		t.Skip("no servers")
	}
	up, err := (&App{}).coreUplink()
	if err != nil {
		t.Fatal(err)
	}
	realIP, _ := publicIP()

	// 1. Fixed port, localhost only: SOCKS5 and HTTP on the same port.
	core := &coreManager{}
	core.SetListen(22080, false)
	if err := core.Start(up, db.Usable(), db.Selected); err != nil {
		t.Fatal(err)
	}
	if core.Addr() != "127.0.0.1:22080" {
		t.Errorf("addr %s", core.Addr())
	}
	for _, p := range []string{"socks5h://127.0.0.1:22080", "http://127.0.0.1:22080"} {
		ip, err := publicIP("-x", p)
		t.Logf("via %s: %q err=%v (real %s)", p, ip, err, realIP)
		if ip == "" || ip == realIP {
			t.Errorf("proxy %s does not work", p)
		}
	}
	lanAddr := net.JoinHostPort(up.Addr.String(), "22080")
	if c, err := net.Dial("tcp4", lanAddr); err == nil {
		c.Close()
		t.Errorf("LAN off but %s accepts connections", lanAddr)
	}

	// 2. The same port again while it is in use: clear error.
	other := &coreManager{}
	other.SetListen(22080, false)
	if err := other.Start(up, db.Usable(), db.Selected); err == nil {
		other.Stop()
		t.Error("second core on a busy port did not fail")
	} else {
		t.Logf("busy port: %v", err)
	}
	core.Stop()

	// 3. LAN access on.
	core.SetListen(22081, true)
	if err := core.Start(up, db.Usable(), db.Selected); err != nil {
		t.Fatal(err)
	}
	ip, err := publicIP("-x", "socks5h://"+net.JoinHostPort(up.Addr.String(), strconv.Itoa(22081)))
	t.Logf("via LAN address %s:22081: %q err=%v", up.Addr, ip, err)
	if ip == "" || ip == realIP {
		t.Error("LAN access does not work")
	}
	core.Stop()
}

func TestE2ESystemProxyRestore(t *testing.T) {
	e2e(t)
	t.Setenv("V2TUN_DATADIR", t.TempDir())
	before := readSystemProxy()
	if err := setSystemProxy("127.0.0.1:22080"); err != nil {
		t.Fatal(err)
	}
	if now := readSystemProxy(); now.Enable != 1 || now.Server != "127.0.0.1:22080" {
		restoreSystemProxy(before)
		t.Fatalf("not set: %+v", now)
	}
	// As if v2tun crashed in "proxy only" mode: the stale state is repaired.
	saveState(runState{PID: 999999, Proxy: &before})
	if !cleanupStale() {
		restoreSystemProxy(before)
		t.Fatal("cleanup did not run")
	}
	if after := readSystemProxy(); after != before {
		restoreSystemProxy(before)
		t.Fatalf("not restored: before %+v after %+v", before, after)
	}
	t.Logf("Windows proxy restored: %+v", before)
}
