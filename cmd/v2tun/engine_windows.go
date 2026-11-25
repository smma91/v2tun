//go:build windows

package main

import (
	"errors"
	"net/netip"
	"strconv"
	"time"
)

func prepareTun() error { return ensureWintun() }

func (e *Engine) configureTun(dev *tunDevice, dns []string) (uint32, error) {
	luid := dev.LUID()
	var idx uint32
	for i := 0; i < 50 && idx == 0; i++ {
		as, _ := listAdapters()
		for _, a := range as {
			if a.LUID == luid {
				idx = a.Index
			}
		}
		if idx == 0 {
			time.Sleep(100 * time.Millisecond)
		}
	}
	if idx == 0 {
		return 0, errors.New("tunnel adapter did not appear")
	}
	i := strconv.Itoa(int(idx))
	// No duplicate-address detection on a point-to-point tunnel: otherwise the
	// address stays "tentative" for seconds, Windows answers "host unreachable"
	// and caches the failed DNS lookups made meanwhile.
	run("netsh", "interface", "ipv4", "set", "interface", i, "dadtransmits=0", "metric=1")
	if err := run("netsh", "interface", "ipv4", "set", "address", "name="+i, "source=static", "address="+e.chooseTunAddr(luid), "mask="+tunMask); err != nil {
		return 0, err
	}
	if !waitAddrReady(luid, 8*time.Second) {
		e.logf("warn", "The tunnel address is not ready yet; continuing")
	}
	for n, d := range dns {
		if _, err := netip.ParseAddr(d); err != nil {
			continue
		}
		if n == 0 {
			run("netsh", "interface", "ipv4", "set", "dnsservers", "name="+i, "source=static", "address="+d, "register=none", "validate=no")
		} else {
			run("netsh", "interface", "ipv4", "add", "dnsservers", "name="+i, "address="+d, "index="+strconv.Itoa(n+1), "validate=no")
		}
	}
	// IPv6 is not tunnelled: the kill switch blocks it so it cannot leak.
	return idx, nil
}

// unconfigureTun: Windows removes the adapter's settings with the adapter.
func (e *Engine) unconfigureTun() {}

func addTunRoutes(idx uint32) error {
	for _, pfx := range []string{"0.0.0.0/1", "128.0.0.0/1"} {
		if err := run("netsh", "interface", "ipv4", "add", "route", pfx, strconv.Itoa(int(idx)), "0.0.0.0", "metric=1", "store=active"); err != nil {
			return err
		}
	}
	return nil
}

func flushDNSCache() { run("ipconfig", "/flushdns") }
