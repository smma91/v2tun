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
	tz       *tzManager

	settings Settings
	since    time.Time
	servers  []string
	cfgMod   int64

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

	e.servers = proxyServers(s.V2rayNDir)
	e.cfgMod = configModTime(s.V2rayNDir)
	e.pinServers(up)
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
	go e.monitor(ctx, dev.LUID())

	if s.MatchTimezone {
		e.tz = newTZManager()
	}
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
func (e *Engine) pinServers(up uplink) {
	pins := map[string][]netip.Addr{}
	for _, h := range e.servers {
		if _, err := netip.ParseAddr(h); err == nil {
			continue
		}
		ips, err := resolveDirect(up, h)
		if err != nil {
			e.logf("warn", "resolve %s: %v", h, err)
			continue
		}
		pins[h] = ips
	}
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

func (e *Engine) monitor(ctx context.Context, luid uint64) {
	defer recoverPanic("monitor")
	t := time.NewTicker(3 * time.Second)
	defer t.Stop()
	lastUp := e.dialer.getUplink().String()
	socksDown := false
	n := 0
	for {
		select {
		case <-ctx.Done():
			return
		case <-t.C:
		}
		n++
		if up, err := findUplink(map[uint64]bool{luid: true}); err == nil && up.String() != lastUp {
			e.dialer.setUplink(up)
			lastUp = up.String()
			e.logf("info", "Network changed: %s", up)
			if e.onUplink != nil {
				go e.onUplink(up)
			}
		}
		if ok := socksAlive(e.settings.SocksAddr); ok == socksDown {
			socksDown = !ok
			if socksDown {
				e.logf("error", "Proxy (%s) does not respond; traffic stopped (no leak)", e.settings.SocksAddr)
			} else {
				e.logf("info", "Proxy is available again")
			}
		}
		if m := configModTime(e.settings.V2rayNDir); e.settings.V2rayNDir != "" && m != e.cfgMod {
			e.cfgMod = m
			e.servers = proxyServers(e.settings.V2rayNDir)
			e.logf("info", "v2rayN settings changed: %s", strings.Join(e.servers, ", "))
			e.pinServers(e.dialer.getUplink())
		}
		if e.tz != nil && n%20 == 1 {
			e.tz.Update(e.logf)
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
	done := make(chan struct{})
	go func() {
		defer recoverPanic("engine")
		defer close(done)
		e.handler.closeAll()
		for _, c := range statistic.DefaultManager.Snapshot().Connections {
			c.Close()
		}
		e.handler.dnsTCP.close()
		e.dev.Close()
		e.stk.Close()
		e.stk.Wait()
	}()
	select {
	case <-done:
	case <-time.After(4 * time.Second):
		e.logf("warn", "Closing open connections took too long; finishing the disconnect without waiting")
	}
	if e.tz != nil {
		e.tz.Restore(e.logf)
		e.tz = nil
	}
	unpinHosts()
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
	info.Servers = e.servers
	info.KillSwitch = e.ks != nil && e.settings.KillSwitch
	info.Proxied = e.dialer.proxied.Load()
	info.Direct = e.dialer.direct.Load()
	info.UpTotal = snap.UploadTotal - e.directUp.Load()
	info.DownTotal = snap.DownloadTotal - e.directDown.Load()
	info.Conns = len(snap.Connections)
	info.SocksOK = true
	return info
}
