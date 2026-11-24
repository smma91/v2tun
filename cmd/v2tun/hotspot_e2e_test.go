//go:build windows

package main

import (
	"testing"
	"time"
)

// The access point alone (no tunnel): it starts, its adapter and address are
// found, DHCP is served, and it goes away again. Needs Wi-Fi on.
func TestE2EHotspotAP(t *testing.T) {
	e2e(t)
	start := time.Now()
	ap, err := startAccessPoint("v2tun-test", "v2tun-test-1234", "1.1.1.1:53")
	if err != nil {
		t.Fatal(err)
	}
	t.Logf("started in %v: adapter %d, %s (%s), own DHCP: %v", time.Since(start).Round(time.Millisecond), ap.index, ap.addr, ap.subnet, ap.dhcp != nil)
	if !ap.addr.IsValid() || !ap.Alive() {
		t.Error("no address / not alive")
	}
	t.Logf("devices: %d; DNS on %s:53: %v", ap.Clients(), ap.addr, udpListening(ap.addr, 53))
	ap.Close()
	time.Sleep(3 * time.Second)
	if ap.Alive() {
		t.Error("helper still running after Close")
	}
	as, _ := listAdapters()
	for _, a := range as {
		if a.Index == ap.index && a.Up {
			t.Error("Wi-Fi Direct adapter still up")
		}
	}
}
