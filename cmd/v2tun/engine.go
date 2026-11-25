//go:build windows || linux

package main

import (
	"context"
	"fmt"
	"net/netip"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"github.com/xjasonlyu/tun2socks/v2/core"
	"github.com/xjasonlyu/tun2socks/v2/proxy/socks5"
	"github.com/xjasonlyu/tun2socks/v2/tunnel/statistic"
	"gvisor.dev/gvisor/pkg/tcpip/stack"
)

const (
	tunAddr = "172.19.233.1"
	tunMask = "255.255.255.252"
	tunMTU  = 1500
)

type Engine struct {
	logf     func(level, format string, args ...any)
	onUplink func(uplink) // the real network changed (the embedded core must rebind)

	mu       sync.Mutex
	running  bool
	dev      *tunDevice
	tunIndex uint32 // interface index of the tunnel
	stk      *stack.Stack
	ks       *killSwitch
	dialer   *routerDialer
	handler  *handler
	cancel   context.CancelFunc
	monDone  chan struct{} // closed when monitor returns
	tz       *tzManager

	settings Settings
	since    time.Time

	// pinMu guards servers and the hosts-file pins: monitor updates them
	// without e.mu, and must never pin again after Stop unpinned.
	pinMu   sync.Mutex
	servers []string

	directUp, directDown atomic.Int64
}

type EngineInfo struct {
	Running    bool     `json:"running"`
	Since      int64    `json:"since"`
	Uplink     string   `json:"uplink"`
	Servers    []string `json:"servers"`
	KillSwitch bool     `json:"killSwitch"`
	Proxied    int64    `json:"proxied"`
	Direct     int64    `json:"direct"`
	UpTotal    int64    `json:"upTotal"`
	DownTotal  int64    `json:"downTotal"`
	Conns      int      `json:"conns"`
	SocksOK    bool     `json:"socksOk"`
}

func (e *Engine) Running() bool {
	e.mu.Lock()
	defer e.mu.Unlock()
	return e.running
}

// conflict reports another TUN (e.g. v2rayN's own TUN mode) that already
// captures the default route.
func conflict() string {
	as, _ := listAdapters()
	for _, a := range as {
		d := strings.ToLower(a.Name + " " + a.Description)
		if a.Up && a.Name != tunName && (strings.Contains(d, "singbox") || strings.Contains(d, "sing-tun") ||
			strings.Contains(d, "mihomo") || strings.Contains(d, "meta tunnel") || strings.Contains(d, "clash")) {
			return a.Name
		}
	}
	return ""
}

func (e *Engine) Start(s Settings) (err error) {
	e.mu.Lock()
	defer e.mu.Unlock()
	if e.running {
		return nil
	}
	if c := conflict(); c != "" {
		return trErr("v2rayN's TUN mode is on (%s). Turn it off in v2rayN first", c)
	}
	builtin := s.ProxySource == "builtin"
	if builtin {
		s.V2rayNDir = "" // the embedded core lives in this process
	}
	if !socksAlive(s.SocksAddr) {
		if builtin {
			return trErr("The built-in core does not respond on %s", s.SocksAddr)
		}
		return trErr("The SOCKS proxy at %s is not available. Start v2rayN and connect to a server", s.SocksAddr)
	}
	if err := prepareTun(); err != nil {
		return err
	}

	var cleanup []func()
	defer func() {
		if err != nil {
			for i := len(cleanup) - 1; i >= 0; i-- {
				cleanup[i]()
			}
		}
	}()

	// v2rayN mode: everything is found from the running v2rayN, nothing to configure.
	if p := processPathByName(v2rayNExe); !builtin && p != "" {
		s.V2rayNDir = filepath.Dir(p)
		e.logf("info", "v2rayN running: %s", s.V2rayNDir)
	}

	unpinHosts()
	up, err := findUplink(nil)
	if err != nil {
		return err
	}
	e.logf("info", "Main internet: %s", up)

	cfgMod := configModTime(s.V2rayNDir)
	e.pinServers(context.Background(), up, proxyServers(s.V2rayNDir))
	cleanup = append(cleanup, func() { unpinHosts() })

	dev, err := openTun(tunMTU)
	if err != nil {
		return err
	}
	cleanup = append(cleanup, dev.Close)
	idx, err := e.configureTun(dev, s.DNS)
	if err != nil {
		return err
	}
	cleanup = append(cleanup, func() { e.unconfigureTun() })
	e.tunIndex = idx

	socks, err := socks5.New(s.SocksAddr, "", "")
	if err != nil {
		return err
	}
	e.dialer = &routerDialer{socks: socks, socksAddr: s.SocksAddr, uplink: up}
	e.dialer.counter = &e.directUp
	e.dialer.counterDown = &e.directDown
	e.handler = &handler{d: e.dialer, dns: newDNSMap(), logf: e.logf, dnsTCP: newDNSTCP(e.dialer)}
	e.handler.blockQUIC.Store(!s.AllowQUIC)
	e.handler.rules.Store(compileRules(s))
	if !s.LocalDNS {
		e.handler.fake = newFakeIP()
	}
	statistic.DefaultManager.ResetStatistic()
	e.directUp.Store(0)
	e.directDown.Store(0)

	stk, err := core.CreateStack(&core.Config{LinkEndpoint: dev, TransportHandler: e.handler})
	if err != nil {
		return fmt.Errorf("network stack: %w", err)
	}
	cleanup = append(cleanup, func() { stk.Close(); stk.Wait() })
	e.handler.reject = rejecter(stk)

	// Firewall: the full kill switch, or at least the IPv6 / DNS leak guard
	// (both optional).
	if s.KillSwitch || !s.DisableLeakGuard {
		apps := coreExecutables(s.V2rayNDir)
		if !builtin { // cores started by v2rayN (or a similar client) wherever they are
			apps = append(apps, runningCores()...)
		}
		if exe, err := os.Executable(); err == nil {
			apps = append(apps, exe)
		}
		if s.V2rayNDir != "" {
			apps = append(apps, filepath.Join(s.V2rayNDir, v2rayNExe))
		}
		ks, err := enableKillSwitch(dev.LUID(), apps, s.AllowLAN, s.KillSwitch)
		if err != nil {
			return fmt.Errorf("kill switch: %w", err)
		}
		e.ks = ks
		cleanup = append(cleanup, func() { ks.Close(); e.ks = nil })
		if s.KillSwitch && platform == "linux" {
			e.logf("info", "Kill switch enabled")
		} else if s.KillSwitch {
			e.logf("info", "Kill switch enabled (%d allowed apps)", len(apps))
		} else {
			e.logf("info", "Kill switch is off; IPv6 and DNS leak protection enabled")
		}
	}

	if err := addTunRoutes(idx); err != nil {
		return fmt.Errorf("route: %w", err)
	}
	flushDNSCache()

	ctx, cancel := context.WithCancel(context.Background())
	e.cancel = cancel
	e.dev, e.stk = dev, stk
	e.settings = s
	e.since = time.Now()
	e.running = true
	if s.MatchTimezone {
		e.tz = newTZManager()
	}
	e.monDone = make(chan struct{})
	go e.monitor(ctx, e.monDone, dev.LUID(), e.dialer, s, cfgMod, e.tz)

	st := runState{PID: os.Getpid(), Since: time.Now().Unix()}
	if e.tz != nil {
		st.OrigTZ = e.tz.orig
	}
	saveState(st)
	e.logf("info", "System-wide tunnel up → %s", s.SocksAddr)
	return nil
}

// pinServers resolves the proxy servers outside the tunnel and pins them in
// the hosts file, so the core can always reach its server.
// Nothing is changed once ctx is canceled (the tunnel is being stopped).
func (e *Engine) pinServers(ctx context.Context, up uplink, servers []string) {
	pins := map[string][]netip.Addr{}
	for _, h := range servers {
		if _, err := netip.ParseAddr(h); err == nil {
			continue
		}
		if ctx.Err() != nil {
			return
		}
		ips, err := resolveDirect(up, h)
		if err != nil {
			e.logf("warn", "resolve %s: %v", h, err)
			continue
		}
		pins[h] = ips
	}
	e.pinMu.Lock()
	defer e.pinMu.Unlock()
	if ctx.Err() != nil {
		return
	}
	e.servers = servers
	if len(pins) > 0 {
		if err := pinHosts(pins); err != nil {
			e.logf("warn", "hosts: %v", err)
			return
		}
		for h, ips := range pins {
			e.logf("info", "Server %s → %v", h, ips)
		}
	}
}

// monitor gets everything it needs as arguments and touches no Engine field
// except through pinServers, so a monitor that Stop stopped waiting for
// cannot interfere with the next Start.
func (e *Engine) monitor(ctx context.Context, done chan<- struct{}, luid uint64, d *routerDialer, s Settings, cfgMod int64, tz *tzManager) {
	defer close(done)
	defer recoverPanic("monitor")
	t := time.NewTicker(3 * time.Second)
	defer t.Stop()
	lastUp := d.getUplink().String()
	socksDown := false
	n := 0
	for {
		select {
		case <-ctx.Done():
			return
		case <-t.C:
		}
		n++
		if up, err := findUplink(map[uint64]bool{luid: true}); err == nil && up.String() != lastUp && ctx.Err() == nil {
			d.setUplink(up)
			lastUp = up.String()
			e.logf("info", "Network changed: %s", up)
			if e.onUplink != nil {
				go e.onUplink(up)
			}
		}
		if ok := socksAlive(s.SocksAddr); ok == socksDown && ctx.Err() == nil {
			socksDown = !ok
			if socksDown {
				e.logf("error", "Proxy (%s) does not respond; traffic stopped (no leak)", s.SocksAddr)
			} else {
				e.logf("info", "Proxy is available again")
			}
		}
		if m := configModTime(s.V2rayNDir); s.V2rayNDir != "" && m != cfgMod && ctx.Err() == nil {
			cfgMod = m
			servers := proxyServers(s.V2rayNDir)
			e.logf("info", "v2rayN settings changed: %s", strings.Join(servers, ", "))
			e.pinServers(ctx, d.getUplink(), servers)
		}
		if tz != nil && n%20 == 1 && ctx.Err() == nil {
			tz.Update(e.logf)
		}
	}
}

func (e *Engine) Stop() {
	e.mu.Lock()
	defer e.mu.Unlock()
	if !e.running {
		return
	}
	e.cancel()
	// Tear the tunnel down without waiting for open connections to finish:
	// close every flow first (a browser keeps dozens open), then the device
	// (Windows removes its routes with it), then the stack. Never block forever.
	// The goroutine works on its own copies, so if it is still running after
	// the timeout it cannot touch what a new Start sets up.
	h, dev, stk := e.handler, e.dev, e.stk
	conns := statistic.DefaultManager.Snapshot().Connections
	done := make(chan struct{})
	go func() {
		defer recoverPanic("engine")
		defer close(done)
		h.closeAll()
		for _, c := range conns {
			c.Close()
		}
		h.dnsTCP.close()
		dev.Close()
		stk.Close()
		stk.Wait()
	}()
	deadline := time.Now().Add(4 * time.Second)
	wait := func(ch <-chan struct{}) bool {
		t := time.NewTimer(time.Until(deadline))
		defer t.Stop()
		select {
		case <-ch:
			return true
		case <-t.C:
			return false
		}
	}
	if !wait(done) {
		e.logf("warn", "Closing open connections took too long; finishing the disconnect without waiting")
	}
	wait(e.monDone) // a late monitor is harmless: it checks its canceled context
	if e.tz != nil {
		e.tz.Restore(e.logf)
		e.tz = nil
	}
	e.pinMu.Lock()
	unpinHosts()
	e.servers = nil
	e.pinMu.Unlock()
	e.unconfigureTun()
	flushDNSCache()
	// Removed last, so nothing can leak while the tunnel is torn down.
	e.ks.Close()
	e.ks = nil
	clearState() // clean shutdown: nothing left for the watchdog to undo
	e.running = false
	e.logf("info", "Tunnel disconnected and network settings restored")
}

// UpdateRules applies new split-tunnel lists to new connections right away.
func (e *Engine) UpdateRules(s Settings) {
	e.mu.Lock()
	defer e.mu.Unlock()
	if e.running {
		e.handler.blockQUIC.Store(!s.AllowQUIC)
		e.handler.rules.Store(compileRules(s))
		e.logf("info", "Split tunneling rules updated (apps: %d, domains: %d)", len(s.Apps), len(s.Domains))
	}
}

func (e *Engine) Info() EngineInfo {
	e.mu.Lock()
	defer e.mu.Unlock()
	info := EngineInfo{Running: e.running}
	if !e.running {
		return info
	}
	snap := statistic.DefaultManager.Snapshot()
	info.Since = e.since.Unix()
	info.Uplink = e.dialer.getUplink().String()
	e.pinMu.Lock()
	info.Servers = e.servers
	e.pinMu.Unlock()
	info.KillSwitch = e.ks != nil && e.settings.KillSwitch
	info.Proxied = e.dialer.proxied.Load()
	info.Direct = e.dialer.direct.Load()
	info.UpTotal = snap.UploadTotal - e.directUp.Load()
	info.DownTotal = snap.DownloadTotal - e.directDown.Load()
	info.Conns = len(snap.Connections)
	info.SocksOK = true
	return info
}
