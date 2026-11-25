//go:build windows || linux

package main

import (
	"encoding/json"
	"net/http"
	"path/filepath"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/wailsapp/wails/v2/pkg/runtime"
)

// ServersView is what the UI shows in the "servers" tab.
type ServersView struct {
	ServerStore
	CoreRunning bool   `json:"coreRunning"`
	CoreAddr    string `json:"coreAddr"`
	Testing     bool   `json:"testing"`
	Updating    bool   `json:"updating"`
	Pinging     bool   `json:"pinging"`
	Speeding    string `json:"speeding"` // server id in the speed test
}

type serverOps struct {
	mu       sync.Mutex
	testing  bool
	updating bool
	pending  bool   // the running core has an outdated server list
	pinging  bool   // ping of all servers in progress
	speeding string // speed test: server id being measured, "" = idle
}

func (a *App) builtin() bool { return a.GetSettings().ProxySource == "builtin" }

func (a *App) emitServers() {
	if a.ctx != nil {
		runtime.EventsEmit(a.ctx, "servers", a.GetServers())
	}
}

func (a *App) GetServers() ServersView {
	a.ops.mu.Lock()
	v := ServersView{ServerStore: a.db.Snapshot(), Testing: a.ops.testing, Updating: a.ops.updating,
		Pinging: a.ops.pinging, Speeding: a.ops.speeding}
	a.ops.mu.Unlock()
	// Stored messages are shown in the current language.
	for _, s := range v.Servers {
		if s.Error != "" {
			s.Error = newServer(s.Link, s.Sub).Error
		}
	}
	for _, s := range v.Subs {
		s.Error = retranslate(s.Error)
	}
	v.CoreRunning = a.core.Running()
	if v.CoreRunning {
		v.CoreAddr = a.core.Addr()
	}
	return v
}

// coreUplink is the real interface the core binds its connections to.
func (a *App) coreUplink() (uplink, error) {
	exclude := map[uint64]bool{}
	if all, err := listAdapters(); err == nil {
		for _, x := range all {
			if x.Name == tunName {
				exclude[x.LUID] = true
			}
		}
	}
	return findUplink(exclude)
}

// startCore (re)starts the embedded core with the current server list.
func (a *App) startCore() error {
	up, err := a.coreUplink()
	if err != nil {
		return err
	}
	st := a.db.Snapshot()
	for _, s := range st.Usable() {
		if s.Type == "naive" {
			if err := ensureCronet(); err != nil {
				a.logf("warn", "libcronet.dll: %v", err)
			}
			break
		}
	}
	s := a.GetSettings()
	a.core.SetListen(s.ProxyPort, s.ProxyLAN)
	allowProxyLAN(s.ProxyPort, s.ProxyLAN) // other devices may reach the proxy through the firewall
	if err := a.core.Start(up, st.Usable(), st.Selected); err != nil {
		return err
	}
	a.ops.mu.Lock()
	a.ops.pending = false
	a.ops.mu.Unlock()
	a.logf("info", "Built-in core running on %s (%d servers, via %s)", a.core.Addr(), len(st.Usable()), up.Name)
	return nil
}

// restartCoreIfRunning applies a changed server list. Restarting the core
// drops every open connection, so while the tunnel is in use the change is
// only applied right away if the server in use is gone; otherwise it waits
// for the next connect and open connections keep working.
func (a *App) restartCoreIfRunning() {
	if !a.core.Running() {
		return
	}
	if a.connected() && a.builtin() && activeStillThere(a.db.Snapshot()) {
		a.ops.mu.Lock()
		already := a.ops.pending
		a.ops.pending = true
		a.ops.mu.Unlock()
		if !already {
			a.logf("info", "The server list changed; it will apply on the next connection so open connections are not cut")
		}
		return
	}
	if err := a.startCore(); err != nil {
		a.logf("error", "Core: %v", err)
	}
}

func activeStillThere(st ServerStore) bool {
	for _, s := range st.Usable() {
		if st.Selected == "auto" || s.ID == st.Selected {
			return true
		}
	}
	return false
}

func (a *App) corePending() bool {
	a.ops.mu.Lock()
	defer a.ops.mu.Unlock()
	return a.ops.pending
}

func (a *App) AddLinks(text string) (int, error) {
	// Subscription addresses may be pasted together with (or instead of) links.
	subs, rest := SplitSubURLs(text)
	added := 0
	for _, u := range subs {
		before := len(a.db.Snapshot().Servers)
		if err := a.AddSubscription("", u); err != nil {
			a.logf("error", "Subscription: %v", err)
			if strings.TrimSpace(rest) == "" {
				return 0, err
			}
			continue
		}
		added += len(a.db.Snapshot().Servers) - before
	}
	if strings.TrimSpace(rest) == "" {
		if len(subs) > 0 {
			return added, nil
		}
		return 0, trErr("No valid link or config found")
	}
	n, err := a.db.AddLinks(rest)
	if err != nil {
		if len(subs) > 0 {
			return added, nil
		}
		return 0, err
	}
	n += added
	a.logf("info", "%d servers added", n)
	a.restartCoreIfRunning()
	a.emitServers()
	return n, nil
}

func (a *App) PasteLinks() (int, error) {
	text, err := runtime.ClipboardGetText(a.ctx)
	if err != nil || text == "" {
		return 0, trErr("The clipboard is empty")
	}
	return a.AddLinks(text)
}

// ImportOVPN picks an OpenVPN profile (.ovpn) and adds it as a server.
// user/pass are only needed when the profile asks for them (auth-user-pass).
func (a *App) ImportOVPN(user, pass string) (string, error) {
	p, err := runtime.OpenFileDialog(a.ctx, runtime.OpenDialogOptions{
		Title:   T("OpenVPN file"),
		Filters: []runtime.FileFilter{{DisplayName: "OpenVPN (*.ovpn, *.conf)", Pattern: "*.ovpn;*.conf"}},
	})
	if err != nil || p == "" {
		return "", err
	}
	link, err := ovpnFileToLink(p, user, pass)
	if err != nil {
		a.logf("error", "OpenVPN: %v", err)
		return "", err
	}
	if _, err := a.AddLinks(link); err != nil {
		return "", err
	}
	return filepath.Base(p), nil
}

// AddOpenConnect adds a Cisco AnyConnect / ocserv / Fortinet / GlobalProtect /
// Pulse / F5 server.
func (a *App) AddOpenConnect(server, flavor, user, pass, group, name string) error {
	server = strings.TrimSpace(server)
	if server == "" {
		return trErr("Enter the server address")
	}
	if !strings.Contains(server, "://") {
		server = "https://" + server
	}
	ob := map[string]any{"type": "openconnect", "server": server, "flavor": firstNonEmpty(flavor, "anyconnect"),
		"tag": firstNonEmpty(name, strings.TrimPrefix(server, "https://"))}
	if user != "" {
		ob["username"], ob["password"] = user, pass
	}
	if group != "" {
		ob["auth_group"] = group
	}
	b, _ := json.Marshal(ob)
	_, err := a.AddLinks(string(b))
	return err
}

func (a *App) AddSubscription(name, url string) error {
	sub, err := a.db.AddSub(name, url)
	if err != nil {
		return err
	}
	a.updateSubs([]string{sub.ID})
	return nil
}

func (a *App) UpdateSubscriptions() { a.updateSubs(nil) }

func (a *App) updateSubs(ids []string) {
	a.ops.mu.Lock()
	if a.ops.updating {
		a.ops.mu.Unlock()
		return
	}
	a.ops.updating = true
	a.ops.mu.Unlock()
	a.emitServers()
	defer func() {
		a.ops.mu.Lock()
		a.ops.updating = false
		a.ops.mu.Unlock()
		a.emitServers()
	}()

	want := map[string]bool{}
	for _, id := range ids {
		want[id] = true
	}
	changed := false
	for _, sub := range a.db.Snapshot().Subs {
		if len(want) > 0 && !want[sub.ID] {
			continue
		}
		links, err := a.fetchSubscription(sub.URL)
		a.db.ApplySub(sub.ID, links, err)
		if err != nil {
			a.logf("error", "Subscription «%s»: %v", sub.Name, err)
		} else {
			a.logf("info", "Subscription \"%s\": %d servers", sub.Name, len(links))
			changed = true
		}
	}
	if changed {
		a.restartCoreIfRunning()
	}
}

// fetchSubscription goes through the tunnel when it is up (the provider then
// does not see the real IP); otherwise, or if that fails, directly.
func (a *App) fetchSubscription(url string) ([]string, error) {
	client := &http.Client{Timeout: 30 * time.Second}
	links, err := fetchSub(client, url)
	if err == nil || !a.connected() {
		return links, err
	}
	up, uerr := a.coreUplink()
	if uerr != nil {
		return nil, err
	}
	d := directDialer(up)
	direct := &http.Client{Timeout: 30 * time.Second, Transport: &http.Transport{DialContext: d.DialContext}}
	return fetchSub(direct, url)
}

func (a *App) RemoveSubscription(id string) {
	a.db.RemoveSub(id)
	a.restartCoreIfRunning()
	a.emitServers()
}

func (a *App) RemoveServer(id string) {
	a.db.RemoveServer(id)
	a.restartCoreIfRunning()
	a.emitServers()
}

func (a *App) SelectServer(id string) {
	a.db.SetSelected(id)
	if a.core.Running() && !a.core.Select(id) {
		a.restartCoreIfRunning()
	}
	name := T("Auto (fastest)")
	for _, s := range a.db.Snapshot().Servers {
		if s.ID == id {
			name = s.Name
		}
	}
	a.logf("info", "Server selected: %s", name)
	a.emitServers()
	go func() { time.Sleep(2 * time.Second); a.RefreshExit() }()
}

func (a *App) SetServerOptions(autoHours int, failover bool) {
	a.db.SetOptions(autoHours, failover)
	a.emitServers()
}

// testCore returns a core that knows every server in st. When the running
// core carries live traffic with an outdated list, a temporary one is used so
// open connections are not disturbed.
func (a *App) testCore(st ServerStore) (*coreManager, func(), error) {
	if a.core.Running() && a.corePending() {
		up, err := a.coreUplink()
		if err != nil {
			return nil, nil, err
		}
		tmp := &coreManager{logf: a.logf}
		if err := tmp.Start(up, st.Usable(), "auto"); err != nil {
			a.logf("error", "Core: %v", err)
			return nil, nil, err
		}
		return tmp, tmp.stopOnly, nil
	}
	if !a.core.Running() {
		if err := a.startCore(); err != nil {
			a.logf("error", "Core: %v", err)
			return nil, nil, err
		}
	}
	return a.core, func() {}, nil
}

// PingServers measures the network ping to every server (TCP handshake, or
// ICMP for UDP protocols), outside the tunnel.
func (a *App) PingServers() error {
	a.ops.mu.Lock()
	if a.ops.pinging {
		a.ops.mu.Unlock()
		return nil
	}
	a.ops.pinging = true
	a.ops.mu.Unlock()
	a.emitServers()
	defer func() {
		a.ops.mu.Lock()
		a.ops.pinging = false
		a.ops.mu.Unlock()
		a.emitServers()
	}()
	up, err := a.coreUplink()
	if err != nil {
		return err
	}
	st := a.db.Snapshot()
	res := pingServers(up, st.Usable())
	a.db.SetPings(res)
	ok := 0
	for _, v := range res {
		if v > 0 {
			ok++
		}
	}
	a.logf("info", "Server ping: %d of %d answered", ok, len(res))
	return nil
}

// SpeedTest downloads through each given server (all usable ones when ids is
// empty), one after another, and reports Mbit/s live.
func (a *App) SpeedTest(ids []string) error {
	a.ops.mu.Lock()
	if a.ops.speeding != "" {
		a.ops.mu.Unlock()
		return nil
	}
	a.ops.speeding = "*"
	a.ops.mu.Unlock()
	defer func() {
		a.ops.mu.Lock()
		a.ops.speeding = ""
		a.ops.mu.Unlock()
		a.emitServers()
	}()
	st := a.db.Snapshot()
	if len(ids) == 0 {
		// Fastest first, dead ones last: useful results arrive early.
		list := st.Usable()
		sort.SliceStable(list, func(i, j int) bool {
			di, dj := list[i].Delay, list[j].Delay
			if (di > 0) != (dj > 0) {
				return di > 0
			}
			return di < dj
		})
		for _, s := range list {
			ids = append(ids, s.ID)
		}
	}
	tester, done, err := a.testCore(st)
	if err != nil {
		return err
	}
	defer done()
	for _, id := range ids {
		a.ops.mu.Lock()
		a.ops.speeding = id
		a.ops.mu.Unlock()
		a.emitServers()
		mbps, err := tester.Speed(id, 6*time.Second)
		if err != nil {
			mbps = -1
		}
		a.db.SetSpeed(id, mbps)
		name := id
		for _, s := range st.Servers {
			if s.ID == id {
				name = s.Name
			}
		}
		if err != nil {
			a.logf("warn", "Speed test \"%s\": failed (%v)", name, err)
		} else {
			a.logf("info", "Speed test \"%s\": %.1f Mbps", name, mbps)
		}
	}
	return nil
}

// TestServers measures every server's real delay (starting the core if needed).
func (a *App) TestServers() error {
	a.ops.mu.Lock()
	if a.ops.testing {
		a.ops.mu.Unlock()
		return nil
	}
	a.ops.testing = true
	a.ops.mu.Unlock()
	a.emitServers()
	defer func() {
		a.ops.mu.Lock()
		a.ops.testing = false
		a.ops.mu.Unlock()
		a.emitServers()
	}()
	st := a.db.Snapshot()
	var ids []string
	for _, s := range st.Usable() {
		ids = append(ids, s.ID)
	}
	tester, done, err := a.testCore(st)
	if err != nil {
		return err
	}
	defer done()
	res := tester.Test(ids)
	a.db.SetDelays(res)
	ok := 0
	for _, d := range res {
		if d > 0 {
			ok++
		}
	}
	a.logf("info", "Server test: %d of %d working", ok, len(ids))
	return nil
}

// bestServer returns the fastest working server from the last test.
func bestServer(st ServerStore, except string) *Server {
	var cand []*Server
	for _, s := range st.Usable() {
		if s.Delay > 0 && s.ID != except {
			cand = append(cand, s)
		}
	}
	sort.Slice(cand, func(i, j int) bool { return cand[i].Delay < cand[j].Delay })
	if len(cand) == 0 {
		return nil
	}
	return cand[0]
}

// serverMaintenance: failover away from a dead selected server, and periodic
// subscription updates.
func (a *App) serverMaintenance() {
	defer recoverPanic("maintenance")
	fails := 0
	t := time.NewTicker(30 * time.Second)
	for range t.C {
		st := a.db.Snapshot()
		if len(st.Subs) > 0 && st.AutoHours > 0 {
			for _, s := range st.Subs {
				if time.Since(time.Unix(s.Updated, 0)) > time.Duration(st.AutoHours)*time.Hour {
					a.logf("info", "Auto-updating subscriptions…")
					a.UpdateSubscriptions()
					break
				}
			}
		}
		if !a.builtin() || !a.connected() || !a.core.Running() || !st.Failover || st.Selected == "auto" {
			fails = 0
			continue
		}
		res := a.core.Test([]string{st.Selected})
		if res[st.Selected] > 0 {
			fails = 0
			continue
		}
		if fails++; fails < 2 {
			continue
		}
		fails = 0
		a.logf("warn", "The selected server does not respond; looking for a working one…")
		a.TestServers()
		if b := bestServer(a.db.Snapshot(), st.Selected); b != nil {
			a.logf("info", "Auto-switched to \"%s\" (%dms)", b.Name, b.Delay)
			a.SelectServer(b.ID)
		} else {
			a.logf("error", "No working server found")
		}
	}
}
