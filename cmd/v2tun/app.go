//go:build windows || linux

package main

import (
	"context"
	"fmt"
	"net/netip"
	"os"
	"path/filepath"
	"slices"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/wailsapp/wails/v2/pkg/runtime"
)

type LogLine struct {
	Time  string `json:"time"`
	Level string `json:"level"`
	Msg   string `json:"msg"`

	// The untranslated message, so the log can be shown again in another language.
	format string
	args   []any
}

type Status struct {
	EngineInfo
	Exit        ExitInfo    `json:"exit"`
	Conflict    string      `json:"conflict"`
	SocksOK     bool        `json:"socksOk"`
	V2rayNFound bool        `json:"v2rayNFound"`
	UpRate      int64       `json:"upRate"`
	DownRate    int64       `json:"downRate"`
	Busy        bool        `json:"busy"`
	ProxyMode   bool        `json:"proxyMode"` // connected in "proxy only" mode
	Hotspot     HotspotInfo `json:"hotspot"`
}

type App struct {
	ctx    context.Context
	engine *Engine

	mu       sync.Mutex
	settings Settings
	logs     []LogLine
	exit     ExitInfo
	busy     bool
	lastUp   int64
	lastDown int64
	upRate   int64
	downRate int64

	// "proxy only" mode (no system tunnel)
	proxyOn      bool
	proxySince   time.Time
	proxyRestore *proxyBackup // Windows proxy setting to put back, if we changed it
	proxyTZ      *tzManager

	quitting  bool
	autostart bool
	launched  bool // started by Windows at logon

	hotspot hotspotManager

	db   *serverDB    // servers & subscriptions of the embedded core
	core *coreManager // embedded sing-box core
	ops  serverOps
}

func NewApp() *App {
	a := &App{settings: loadSettings(), db: loadServerDB()}
	setLang(a.settings.Lang)
	a.core = &coreManager{logf: a.logf}
	a.engine = &Engine{logf: a.logf, onUplink: func(uplink) {
		if a.builtin() && a.core.Running() {
			a.restartCoreIfRunning()
		}
	}}
	return a
}

func (a *App) startup(ctx context.Context) {
	a.ctx = ctx
	if cleanupStale() { // the previous run was killed while connected
		a.logf("warn", "The previous run was closed without disconnecting; network settings were restored")
	}
	startWatchdog()
	handleSignals(a)
	for _, p := range []string{settingsPath(), storePath(), backupPath(), statePath()} {
		if _, err := os.Stat(p); err == nil {
			protectFile(p) // files from older versions were readable by every user
		}
	}
	a.autostart = autostartEnabled()
	a.startTray()
	go a.ticker()
	go a.serverMaintenance()
	go func() {
		defer recoverPanic("app")
		for _, u := range a.db.MigrateSubs() {
			a.logf("info", "A subscription link that was stored as a server has been fixed")
			a.AddSubscription("", u)
		}
	}()
	if a.GetSettings().AutoConnect {
		go a.autoConnect()
	}
}

// autoConnect waits for v2rayN / the network (both may still be starting
// after logon) and connects.
func (a *App) autoConnect() {
	defer recoverPanic("autoconnect")
	a.logf("info", "Auto-connect: waiting for the network and proxy…")
	deadline := time.Now().Add(3 * time.Minute)
	for time.Now().Before(deadline) {
		if a.connected() {
			return
		}
		s := a.GetSettings()
		_, upErr := a.coreUplink()
		ready := socksAlive(s.SocksAddr)
		if s.ProxySource == "builtin" {
			ready = upErr == nil
		}
		if ready && conflict() == "" {
			a.Connect()
			return
		}
		time.Sleep(3 * time.Second)
	}
	a.logf("warn", "Auto-connect failed: v2rayN was not available")
}

// beforeClose: closing the window only hides it to the tray, unless the user
// chose "exit on close" or quits from the tray menu.
func (a *App) beforeClose(ctx context.Context) bool {
	a.mu.Lock()
	quitting, exit := a.quitting, a.settings.ExitOnClose
	a.mu.Unlock()
	if quitting || exit {
		a.hotspot.Stop(a.engine)
		a.engine.Stop()
		a.stopProxyMode()
		return false
	}
	a.hideWindow()
	return true
}

func (a *App) GetAutostart() bool {
	a.mu.Lock()
	defer a.mu.Unlock()
	return a.autostart
}

func (a *App) SetAutostart(on bool) error {
	if err := setAutostart(on); err != nil {
		a.logf("error", pick("Start with Windows: %v", "Start at login: %v"), err)
		return err
	}
	a.mu.Lock()
	a.autostart = on
	a.mu.Unlock()
	if on {
		a.logf("info", pick("v2tun will start when you sign in to Windows", "v2tun will start when you log in"))
	} else {
		a.logf("info", pick("Start with Windows turned off", "Start at login turned off"))
	}
	return nil
}

// appVersion is set at build time: -ldflags "-X main.appVersion=1.0.0".
var appVersion = "0.9.0-beta"

func (a *App) Version() string { return appVersion }

// SetLang switches the language of logs, errors and the tray menu.
func (a *App) SetLang(l string) {
	l = validLang(l)
	a.mu.Lock()
	a.settings.Lang = l
	s := a.settings
	a.mu.Unlock()
	setLang(l)
	saveSettings(s)
}

// CloseWindow is the ✕ of the custom title bar: same as closing a normal
// window (hide to the tray, or quit if "closing the window quits" is on).
func (a *App) CloseWindow() {
	if a.GetSettings().ExitOnClose {
		go a.quit()
		return
	}
	a.hideWindow()
}

// Quit exits completely (tunnel stopped, tray icon removed).
func (a *App) Quit() { go a.quit() }

// Platform is "windows" or "linux": the UI adapts a few texts and hides what
// the system does not support.
func (a *App) Platform() string { return platform }

// NeedsPermission: on Linux, v2tun does not have its network capabilities yet.
func (a *App) NeedsPermission() bool { return !hasPrivileges() }

// GrantPermission asks for the password once and gives v2tun its network
// capabilities (Linux), then restarts it.
func (a *App) GrantPermission() error {
	if err := grantPermission(a); err != nil {
		a.logf("error", "Permission: %v", err)
		return err
	}
	return nil
}

func (a *App) shutdown(ctx context.Context) {
	a.hotspot.Stop(a.engine)
	a.engine.Stop()
	a.stopProxyMode()
	a.core.Stop()
}

func (a *App) logf(level, format string, args ...any) {
	l := LogLine{Time: time.Now().Format("15:04:05"), Level: level, Msg: fmt.Sprintf(T(format), args...), format: format, args: args}
	a.mu.Lock()
	a.logs = append(a.logs, l)
	if len(a.logs) > 400 {
		a.logs = a.logs[len(a.logs)-400:]
	}
	a.mu.Unlock()
	writeLogFile(l)
	if a.ctx != nil {
		runtime.EventsEmit(a.ctx, "log", l)
	}
}

func (a *App) ticker() {
	defer recoverPanic("ticker")
	t := time.NewTicker(time.Second)
	for range t.C {
		st := a.Status()
		runtime.EventsEmit(a.ctx, "status", st)
	}
}

// ---- bound to the UI ----

func (a *App) GetSettings() Settings {
	a.mu.Lock()
	defer a.mu.Unlock()
	return a.settings
}

func (a *App) SaveSettings(s Settings) error {
	hotspotDefaults(&s) // an emptied name or a too short password: back to generated ones
	if s.ProxySource != "builtin" {
		s.ProxySource = "v2rayn"
	}
	a.mu.Lock()
	old := a.settings
	a.settings = s
	a.mu.Unlock()
	if !slices.Equal(old.Apps, s.Apps) || !slices.Equal(old.Domains, s.Domains) || old.AppMode != s.AppMode ||
		old.DomainMode != s.DomainMode || old.AllowQUIC != s.AllowQUIC {
		a.engine.UpdateRules(s)
	}
	modeChanged := old.Mode != s.Mode || old.ProxyPort != s.ProxyPort || old.ProxyLAN != s.ProxyLAN || old.SystemProxy != s.SystemProxy
	if old.ProxySource != s.ProxySource || (modeChanged && s.ProxySource == "builtin" && a.connected()) {
		if old.ProxySource != s.ProxySource {
			a.logf("info", "Proxy source: %s", map[string]string{"builtin": T("Built-in core"), "v2rayn": "v2rayN"}[s.ProxySource])
		}
		go func() {
			defer recoverPanic("app")
			if a.connected() { // reconnect through the new source
				a.Disconnect()
				a.Connect()
			}
			if s.ProxySource != "builtin" && !a.connected() {
				a.core.Stop()
			}
			a.emitServers()
		}()
	}
	// Hotspot turned on/off or renamed while the tunnel is up: apply now.
	if a.engine.Running() && (old.Hotspot != s.Hotspot || old.HotspotSSID != s.HotspotSSID || old.HotspotPass != s.HotspotPass) {
		if s.Hotspot {
			a.hotspot.Start(s, a.engine, a.logf)
		} else {
			a.hotspot.Stop(a.engine)
			a.logf("info", "Hotspot turned off")
		}
	}
	return saveSettings(s)
}

func (a *App) Logs() []LogLine {
	a.mu.Lock()
	defer a.mu.Unlock()
	out := make([]LogLine, 0, len(a.logs)) // never null: the UI iterates it
	for _, l := range a.logs {
		l.Msg = fmt.Sprintf(T(l.format), retranslateArgs(l.args)...) // in the current language
		out = append(out, l)
	}
	return out
}

func (a *App) Connect() error {
	a.mu.Lock()
	if a.busy {
		a.mu.Unlock()
		return nil
	}
	a.busy = true
	s := a.settings
	a.mu.Unlock()
	defer func() { a.mu.Lock(); a.busy = false; a.mu.Unlock() }()

	a.logf("info", "Connecting…")
	if s.ProxySource == "builtin" {
		if len(a.db.Snapshot().Usable()) == 0 {
			err := trErr("There are no servers in \"Servers\". Add a link or subscription")
			a.logf("error", "%v", err)
			return err
		}
		// Not connected yet, so nothing to interrupt: apply a pending server
		// list or a changed local proxy port now.
		port, lan := a.core.ListenInfo()
		if !a.core.Running() || a.corePending() || port != s.ProxyPort || lan != s.ProxyLAN {
			if err := a.startCore(); err != nil {
				a.logf("error", "Core: %v", err)
				return err
			}
		}
		s.SocksAddr = a.core.Addr()
		if s.Mode == "proxy" {
			return a.startProxyMode(s)
		}
	}
	if err := a.engine.Start(s); err != nil {
		a.logf("error", "%v", err)
		return err
	}
	if s.Hotspot {
		a.hotspot.Start(s, a.engine, a.logf)
	}
	go a.RefreshExit()
	return nil
}

func (a *App) Disconnect() {
	a.mu.Lock()
	a.busy = true
	a.mu.Unlock()
	a.hotspot.Stop(a.engine)
	a.engine.Stop()
	a.stopProxyMode()
	a.mu.Lock()
	a.busy = false
	a.exit = ExitInfo{}
	a.mu.Unlock()
}

// connected: the system tunnel or the "proxy only" mode is on.
func (a *App) connected() bool { return a.engine.Running() || a.proxyMode() }

func (a *App) proxyMode() bool {
	a.mu.Lock()
	defer a.mu.Unlock()
	return a.proxyOn
}

// startProxyMode: only the local SOCKS/HTTP proxy, no system tunnel. Optionally
// the Windows proxy setting points at it (restored on disconnect or crash).
func (a *App) startProxyMode(s Settings) error {
	st := runState{PID: os.Getpid(), Since: time.Now().Unix()}
	if s.SystemProxy {
		b := readSystemProxy()
		st.Proxy = &b
		saveState(st) // first, so a crash right now is still undone
		if err := setSystemProxy(a.core.Addr()); err != nil {
			clearState()
			a.logf("error", pick("Windows proxy: %v", "System proxy: %v"), err)
			return err
		}
	}
	exitViaProxy.Store(a.core.Addr())
	var tz *tzManager
	if s.MatchTimezone { // optional, as in tunnel mode
		if tz = newTZManager(); tz != nil {
			st.OrigTZ = tz.orig
		}
	}
	saveState(st)
	a.mu.Lock()
	a.proxyOn, a.proxySince, a.proxyRestore, a.proxyTZ = true, time.Now(), st.Proxy, tz
	a.mu.Unlock()
	if tz != nil {
		go tz.Update(a.logf)
	}
	for _, addr := range a.ProxyAddresses() {
		a.logf("info", "Local proxy active: %s", addr)
	}
	go a.RefreshExit()
	return nil
}

func (a *App) stopProxyMode() {
	a.mu.Lock()
	on, restore, tz := a.proxyOn, a.proxyRestore, a.proxyTZ
	a.proxyOn, a.proxyRestore, a.proxyTZ = false, nil, nil
	a.mu.Unlock()
	if !on {
		return
	}
	if restore != nil {
		restoreSystemProxy(*restore)
	}
	if tz != nil {
		tz.Restore(a.logf)
	}
	clearState()
	exitViaProxy.Store("")
	a.core.Stop() // free the port: the proxy is off
	a.logf("info", "Local proxy stopped")
}

// ProxyAddresses lists where the local proxy can be reached (SOCKS5 and HTTP
// share the port): localhost, plus, if LAN access is on, every address other
// devices can use: the Wi-Fi / Ethernet network this computer is on and its
// own hotspot.
func (a *App) ProxyAddresses() []string {
	s := a.GetSettings()
	port := strconv.Itoa(s.ProxyPort)
	out := []string{"127.0.0.1:" + port}
	if s.ProxyLAN {
		for _, ip := range lanAddresses() {
			out = append(out, ip.String()+":"+port)
		}
	}
	return out
}

// lanAddresses: private IPv4 addresses of connected adapters, except the
// tunnel and virtual machine / container switches.
func lanAddresses() []netip.Addr {
	as, _ := listAdapters()
	var out []netip.Addr
	for _, x := range as {
		d := strings.ToLower(x.Name + " " + x.Description)
		if !x.Up || x.Name == tunName || strings.Contains(d, "vethernet") || strings.Contains(d, "vmware") ||
			strings.Contains(d, "virtualbox") || strings.Contains(d, "docker") || strings.Contains(d, "loopback") ||
			strings.HasPrefix(x.Name, "veth") || strings.HasPrefix(x.Name, "br-") || x.Name == "lo" {
			continue
		}
		for _, ip := range x.V4 {
			if ip.IsPrivate() && !slices.Contains(out, ip) {
				out = append(out, ip)
			}
		}
	}
	return out
}

func (a *App) RefreshExit() ExitInfo {
	info, err := exitInfo()
	if err != nil {
		a.logf("warn", "Exit IP: %v", err)
	} else {
		a.logf("info", "Exit IP: %s (%s, %s)", info.IP, info.Country, info.Org)
	}
	a.mu.Lock()
	a.exit = info
	a.mu.Unlock()
	return info
}

func (a *App) LeakTest() LeakResult {
	r := leakCheck()
	if r.Unknown {
		a.logf("warn", "Leak test: the web IP could not be read (%s)", r.HTTPErr)
	} else if r.Pass {
		a.logf("info", "Leak test: pass ✓ (web %s, WebRTC %s)", r.HTTPIP, or(r.STUNIP, "—"))
	} else {
		a.logf("error", "Leak test: fail ✗ (web %s, WebRTC %s)", r.HTTPIP, or(r.STUNIP, r.STUNErr))
	}
	return r
}

// ListAdapters returns the real connections that have internet access.
func (a *App) ListAdapters() []NetAdapter {
	as, err := internetAdapters()
	if err != nil {
		a.logf("error", "Connection list: %v", err)
	}
	return as
}

// SetAdapterDNS applies a DNS preset (cloudflare, google, dhcp, restore) to a connection.
func (a *App) SetAdapterDNS(index uint32, preset string) ([]NetAdapter, error) {
	if err := applyAdapterDNS(index, preset); err != nil {
		a.logf("error", "DNS: %v", err)
		return nil, err
	}
	as, _ := internetAdapters()
	for _, x := range as {
		if x.Index == index {
			a.logf("info", "DNS of \"%s\" → %s", x.Name, strings.Join(append(x.DNS4, x.DNS6...), ", "))
		}
	}
	return as, nil
}

type AppEntry struct {
	Name string `json:"name"`
	Path string `json:"path"`
}

// RunningApps lists the programs currently running (to pick for split tunnel).
func (a *App) RunningApps() []AppEntry {
	seen := map[string]bool{}
	var out []AppEntry
	sys := strings.ToLower(filepath.Join(os.Getenv("SystemRoot"), "System32"))
	for _, p := range allProcessPaths() {
		lp := strings.ToLower(p)
		if seen[lp] || strings.HasPrefix(lp, sys) || isCorePath(p) {
			continue
		}
		seen[lp] = true
		out = append(out, AppEntry{Name: filepath.Base(p), Path: p})
	}
	sort.Slice(out, func(i, j int) bool { return strings.ToLower(out[i].Name) < strings.ToLower(out[j].Name) })
	return out
}

func (a *App) BrowseApp() string {
	p, err := runtime.OpenFileDialog(a.ctx, runtime.OpenDialogOptions{
		Title:   T("Choose an app"),
		Filters: []runtime.FileFilter{{DisplayName: T("Programs"), Pattern: exeFilter}},
	})
	if err != nil {
		return ""
	}
	return p
}

func (a *App) Status() Status {
	info := a.engine.Info()
	a.mu.Lock()
	defer a.mu.Unlock()
	st := Status{EngineInfo: info, Exit: a.exit, Busy: a.busy, V2rayNFound: a.settings.V2rayNDir != ""}
	if info.Running {
		a.upRate, a.downRate = max(info.UpTotal-a.lastUp, 0), max(info.DownTotal-a.lastDown, 0)
		a.lastUp, a.lastDown = info.UpTotal, info.DownTotal
		st.SocksOK = true
	} else {
		a.lastUp, a.lastDown, a.upRate, a.downRate = 0, 0, 0, 0
	}
	st.UpRate, st.DownRate = a.upRate, a.downRate
	st.Hotspot = a.hotspot.Info()
	if a.proxyOn { // "proxy only": no tunnel statistics, but it is connected
		st.Running, st.Since, st.ProxyMode = true, a.proxySince.Unix(), true
	}
	return st
}

// Checks shown before connecting (cheap enough to call every few seconds).
func (a *App) Preflight() Status {
	st := a.Status()
	st.Conflict = conflict()
	if a.builtin() {
		st.SocksOK = true // the embedded core is started on connect
	} else {
		st.SocksOK = socksAlive(a.GetSettings().SocksAddr)
	}
	return st
}

func or(a, b string) string {
	if a != "" {
		return a
	}
	return b
}
