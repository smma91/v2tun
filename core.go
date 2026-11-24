//go:build windows

package main

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	mrand "math/rand/v2"
	"net"
	"net/http"
	"strconv"
	"strings"
	"sync"
	"time"

	box "github.com/sagernet/sing-box"
	"github.com/sagernet/sing-box/common/urltest"
	"github.com/sagernet/sing-box/include"
	"github.com/sagernet/sing-box/option"
	sjson "github.com/sagernet/sing/common/json"
	M "github.com/sagernet/sing/common/metadata"
	N "github.com/sagernet/sing/common/network"
)

const testURL = "https://www.gstatic.com/generate_204"

// coreManager runs the embedded sing-box core. It exposes one local SOCKS/HTTP
// port that the tunnel engine uses exactly like v2rayN's port. All outgoing
// connections of the core are bound to the real network interface, so they
// never enter the tunnel (no routing loop).
type coreManager struct {
	logf func(level, format string, args ...any)

	mu     sync.Mutex
	inst   *box.Box
	cancel context.CancelFunc
	port   int
	// local proxy settings (see SetListen)
	wantPort int
	lan      bool
	tags     map[string]bool // server id -> present in the running config
	uplink   string
}

// freePortTCPUDP finds a port usable for both TCP and UDP. Windows (Hyper-V,
// WSL) reserves whole port ranges, so a free TCP port can be forbidden for UDP.
func freePortTCPUDP() (int, error) {
	// The reserved ranges are usually in the dynamic range (49152+); try
	// random ports below it and check that both protocols can bind.
	for i := 0; i < 300; i++ {
		p := 20000 + mrand.IntN(20000)
		l, err := net.Listen("tcp4", "127.0.0.1:"+strconv.Itoa(p))
		if err != nil {
			continue
		}
		u, err := net.ListenPacket("udp4", "127.0.0.1:"+strconv.Itoa(p))
		l.Close()
		if err != nil {
			continue
		}
		u.Close()
		return p, nil
	}
	return 0, errors.New("no free TCP+UDP port")
}

func freePort() (int, error) {
	l, err := net.Listen("tcp4", "127.0.0.1:0")
	if err != nil {
		return 0, err
	}
	defer l.Close()
	return l.Addr().(*net.TCPAddr).Port, nil
}

// coreConfig builds a sing-box configuration from the server list.
func coreConfig(port int, uplink uplink, servers []*Server, selected string) ([]byte, map[string]bool, error) {
	var outbounds, endpoints []any
	var tags []string
	present := map[string]bool{}
	viaXray := map[string]bool{}
	for _, s := range servers {
		var ob map[string]any
		if needsXray(s.Link) {
			// VLESS XHTTP: a dedicated embedded Xray instance, reached over local SOCKS.
			p, err := xrays.ensure(s.ID, s.Link, uplink.Name)
			if err != nil {
				writeLogFile(LogLine{Level: "warn", Msg: "xray " + s.Name + ": " + err.Error()})
				continue
			}
			ob = map[string]any{"type": "socks", "server": "127.0.0.1", "server_port": p, "version": "5"}
			viaXray[s.ID] = true
		} else {
			var err error
			if ob, err = s.outbound(); err != nil {
				continue
			}
		}
		ob["tag"] = s.ID
		// WireGuard, OpenVPN and OpenConnect are "endpoints" in sing-box.
		if t := ob["type"]; t == "wireguard" || t == "openvpn-client" || t == "openconnect" {
			endpoints = append(endpoints, ob)
		} else {
			outbounds = append(outbounds, ob)
		}
		tags = append(tags, s.ID)
		present[s.ID] = true
	}
	xrays.keep(viaXray)
	if len(tags) == 0 {
		return nil, nil, trErr("No usable server in the list")
	}
	if selected == "" || (selected != "auto" && !present[selected]) {
		selected = "auto"
	}
	// Switching servers (auto "fastest" or a manual choice) only affects NEW
	// connections: open ones (downloads, calls, games) keep their server until
	// they end, instead of being cut and timing out. A higher tolerance also
	// stops "auto" from hopping between servers of similar speed.
	outbounds = append(outbounds,
		map[string]any{"type": "urltest", "tag": "auto", "outbounds": tags, "url": testURL,
			"interval": "3m", "tolerance": 150, "idle_timeout": "30m", "interrupt_exist_connections": false},
		map[string]any{"type": "selector", "tag": "select", "outbounds": append([]string{"auto"}, tags...),
			"default": selected, "interrupt_exist_connections": false},
		map[string]any{"type": "direct", "tag": "direct"},
	)

	dns := "1.1.1.1"
	if len(uplink.DNS) > 0 {
		dns = uplink.DNS[0].String()
	}
	cfg := map[string]any{
		"log": map[string]any{"level": "warn"},
		"dns": map[string]any{
			// Server names are resolved outside the tunnel, on the real network.
			"servers": []any{map[string]any{"type": "udp", "tag": "bootstrap", "server": dns}},
			"final":   "bootstrap",
		},
		"inbounds":  []any{map[string]any{"type": "mixed", "tag": "in", "listen": "127.0.0.1", "listen_port": port}},
		"outbounds": outbounds,
		"route": map[string]any{
			"final":                   "select",
			"default_interface":       uplink.Name,
			"default_domain_resolver": "bootstrap",
		},
	}
	if len(endpoints) > 0 {
		cfg["endpoints"] = endpoints
	}
	b, err := json.Marshal(cfg)
	return b, present, err
}

func (c *coreManager) Running() bool {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.inst != nil
}

func (c *coreManager) Addr() string {
	c.mu.Lock()
	defer c.mu.Unlock()
	return "127.0.0.1:" + strconv.Itoa(c.port)
}

// SetListen fixes the local proxy port (0 = pick a free one) and whether it
// accepts connections from other devices on the network.
func (c *coreManager) SetListen(port int, lan bool) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.wantPort, c.lan = port, lan
}

// ListenInfo: the configured port and LAN exposure of the running core.
func (c *coreManager) ListenInfo() (port int, lan bool) {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.port, c.lan
}

// Start (re)starts the core with the given servers. The local port is kept
// across restarts so the tunnel does not need to know about them.
func (c *coreManager) Start(up uplink, servers []*Server, selected string) error {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.stopLocked()
	if c.wantPort > 0 {
		c.port = c.wantPort
	} else if c.port == 0 {
		p, err := freePort()
		if err != nil {
			return err
		}
		c.port = p
	}
	listen := "127.0.0.1"
	if c.lan {
		listen = "0.0.0.0"
	}
	if l, err := net.Listen("tcp4", net.JoinHostPort(listen, strconv.Itoa(c.port))); err != nil {
		return trErr("Port %d is in use; choose another port in Settings", c.port)
	} else {
		l.Close()
	}
	raw, present, err := coreConfig(c.port, up, servers, selected)
	if err != nil {
		return err
	}
	raw = []byte(strings.Replace(string(raw), `"listen":"127.0.0.1"`, `"listen":"`+listen+`"`, 1))
	if err != nil {
		return err
	}
	ctx, cancel := context.WithCancel(include.Context(context.Background()))
	opts, err := sjson.UnmarshalExtendedContext[option.Options](ctx, raw)
	if err != nil {
		cancel()
		return fmt.Errorf("core config: %w", err)
	}
	inst, err := box.New(box.Options{Context: ctx, Options: opts})
	if err != nil {
		cancel()
		return fmt.Errorf("core: %w", err)
	}
	if err := inst.Start(); err != nil {
		inst.Close()
		cancel()
		return fmt.Errorf("core start: %w", err)
	}
	c.inst, c.cancel, c.tags, c.uplink = inst, cancel, present, up.Name
	return nil
}

func (c *coreManager) stopLocked() {
	if c.inst != nil {
		c.inst.Close()
		c.cancel()
		c.inst = nil
	}
}

// stopOnly stops this sing-box instance but leaves the shared Xray helpers
// running (used by the temporary test core while the real one is in use).
func (c *coreManager) stopOnly() {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.stopLocked()
}

func (c *coreManager) Stop() {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.stopLocked()
	xrays.stopAll()
}

// Select switches the active server ("auto" = fastest) without a restart.
func (c *coreManager) Select(id string) bool {
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.inst == nil {
		return false
	}
	ob, ok := c.inst.Outbound().Outbound("select")
	if !ok {
		return false
	}
	s, ok := ob.(interface{ SelectOutbound(string) bool })
	return ok && s.SelectOutbound(id)
}

// Test measures the real delay (handshake + HTTP request) through each server.
func (c *coreManager) Test(ids []string) map[string]int {
	c.mu.Lock()
	inst := c.inst
	c.mu.Unlock()
	out := map[string]int{}
	if inst == nil {
		return out
	}
	var mu sync.Mutex
	var wg sync.WaitGroup
	sem := make(chan struct{}, 12)
	for _, id := range ids {
		ob, ok := inst.Outbound().Outbound(id)
		var d N.Dialer = ob
		if !ok {
			ep, ok2 := inst.Endpoint().Get(id)
			if !ok2 {
				continue
			}
			if d, ok2 = ep.(N.Dialer); !ok2 {
				continue
			}
		}
		wg.Add(1)
		go func(id string, d N.Dialer) {
			defer wg.Done()
			sem <- struct{}{}
			defer func() { <-sem }()
			ctx, cancel := context.WithTimeout(context.Background(), 8*time.Second)
			defer cancel()
			ms, err := urltest.URLTest(ctx, testURL, d)
			mu.Lock()
			if err != nil {
				out[id] = -1
			} else {
				out[id] = int(ms)
			}
			mu.Unlock()
		}(id, d)
	}
	wg.Wait()
	return out
}

// serverDialer returns the outbound (or endpoint) of one server.
func (c *coreManager) serverDialer(id string) (N.Dialer, bool) {
	c.mu.Lock()
	inst := c.inst
	c.mu.Unlock()
	if inst == nil {
		return nil, false
	}
	if ob, ok := inst.Outbound().Outbound(id); ok {
		return ob, true
	}
	if ep, ok := inst.Endpoint().Get(id); ok {
		d, ok := ep.(N.Dialer)
		return d, ok
	}
	return nil, false
}

// Download sources for the speed test, tried in order.
var speedURLs = []string{
	"https://speed.cloudflare.com/__down?bytes=50000000",
	"http://cachefly.cachefly.net/50mb.test",
}

// Speed downloads through one server for up to dur and returns Mbit/s.
func (c *coreManager) Speed(id string, dur time.Duration) (float64, error) {
	d, ok := c.serverDialer(id)
	if !ok {
		return 0, errors.New("server not in the running core")
	}
	tr := &http.Transport{
		DialContext: func(ctx context.Context, network, addr string) (net.Conn, error) {
			return d.DialContext(ctx, N.NetworkTCP, M.ParseSocksaddr(addr))
		},
		ForceAttemptHTTP2:     true,
		ResponseHeaderTimeout: 10 * time.Second,
		DisableCompression:    true,
	}
	defer tr.CloseIdleConnections()
	client := &http.Client{Transport: tr}
	var lastErr error
	for _, u := range speedURLs {
		ctx, cancel := context.WithTimeout(context.Background(), dur+12*time.Second)
		req, _ := http.NewRequestWithContext(ctx, "GET", u, nil)
		resp, err := client.Do(req)
		if err != nil {
			cancel()
			lastErr = err
			continue
		}
		if resp.StatusCode != 200 {
			resp.Body.Close()
			cancel()
			lastErr = fmt.Errorf("HTTP %d", resp.StatusCode)
			continue
		}
		// Measure from the first byte, so the handshake does not count.
		buf := make([]byte, 64*1024)
		var n int64
		var start time.Time
		for {
			k, err := resp.Body.Read(buf)
			if k > 0 {
				if start.IsZero() {
					start = time.Now()
				}
				n += int64(k)
			}
			if err != nil || (!start.IsZero() && time.Since(start) >= dur) {
				break
			}
		}
		resp.Body.Close()
		cancel()
		if start.IsZero() || n < 64*1024 {
			lastErr = errors.New("no data")
			continue
		}
		secs := time.Since(start).Seconds()
		return float64(n) * 8 / secs / 1e6, nil
	}
	return 0, lastErr
}
