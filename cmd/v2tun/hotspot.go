//go:build windows || linux

package main

import (
	"net"
	"net/netip"
	"sync"
	"time"
)

// Hotspot: a Wi-Fi access point on this computer whose devices use the
// tunnel. The access point itself is per platform (hotspot_windows.go: Wi-Fi
// Direct "legacy" AP; hotspot_linux.go: NetworkManager). Their traffic is
// forwarded into the tunnel, and the firewall stops it from going anywhere
// else, so if the tunnel drops they have no internet instead of a leak.

type HotspotInfo struct {
	On      bool   `json:"on"`
	SSID    string `json:"ssid"`
	Clients int    `json:"clients"`
	Error   string `json:"error"`
}

type hotspotManager struct {
	mu      sync.Mutex
	ap      *accessPoint
	ssid    string
	err     string
	clients int
	stop    chan struct{}
}

// Start brings the access point up and routes it into the tunnel. Runs in the
// background (bringing Wi-Fi up takes seconds); errors show in Info and the log.
func (h *hotspotManager) Start(s Settings, e *Engine, logf func(string, string, ...any)) {
	h.Stop(e)
	h.mu.Lock()
	h.err, h.ssid, h.clients = "", s.HotspotSSID, 0
	stop := make(chan struct{})
	h.stop = stop
	h.mu.Unlock()
	go func() {
		defer recoverPanic("hotspot")
		var seen sync.Map
		onDNS := func(dev netip.Addr) { // first DNS question of each device: it reached v2tun
			if _, ok := seen.LoadOrStore(dev, true); !ok {
				logf("info", "Hotspot device %s asks DNS through v2tun", dev)
			}
		}
		ap, err := startAccessPoint(s.HotspotSSID, s.HotspotPass)
		if err == nil {
			if err = e.routeHotspot(ap); err == nil {
				if err = ap.startDNS(hotspotDNS(s), onDNS); err != nil {
					e.unrouteHotspot(ap)
				}
			}
			if err != nil {
				ap.Close()
			}
		}
		h.mu.Lock()
		if h.stop != stop { // stopped meanwhile
			h.mu.Unlock()
			if err == nil {
				e.unrouteHotspot(ap)
				ap.Close()
			}
			return
		}
		if err != nil {
			h.err = err.Error()
			h.mu.Unlock()
			logf("error", "Hotspot: %v", err)
			return
		}
		h.ap = ap
		h.mu.Unlock()
		logf("info", "Hotspot \"%s\" is on; its devices use the tunnel", s.HotspotSSID)
		t := time.NewTicker(5 * time.Second)
		defer t.Stop()
		for {
			select {
			case <-stop:
				return
			case <-t.C:
			}
			n, alive := ap.Clients(), ap.Alive()
			h.mu.Lock()
			h.clients = n
			if !alive && h.ap == ap {
				h.err = T("The hotspot stopped (Wi-Fi turned off?)")
			}
			h.mu.Unlock()
		}
	}()
}

// hotspotDNS: the first configured DNS server (queries to it are routed into
// the tunnel), as host:port.
func hotspotDNS(s Settings) string {
	for _, d := range s.DNS {
		if a, err := netip.ParseAddr(d); err == nil && a.Is4() {
			return net.JoinHostPort(d, "53")
		}
	}
	return "1.1.1.1:53"
}

func (h *hotspotManager) Stop(e *Engine) {
	h.mu.Lock()
	ap := h.ap
	if h.stop != nil {
		close(h.stop)
	}
	h.ap, h.stop, h.clients, h.err = nil, nil, 0, ""
	h.mu.Unlock()
	if ap != nil {
		e.unrouteHotspot(ap)
		ap.Close()
	}
}

func (h *hotspotManager) Info() HotspotInfo {
	h.mu.Lock()
	defer h.mu.Unlock()
	return HotspotInfo{On: h.ap != nil && h.err == "", SSID: h.ssid, Clients: h.clients, Error: h.err}
}
