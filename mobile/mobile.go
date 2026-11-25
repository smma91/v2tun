// Package mobile is the Go side of the v2tun Android app (bound with
// gomobile). The app's VpnService runs the tunnel with sing-box's libbox; this
// package keeps the server list, subscriptions and settings, translates the
// UI texts and builds the sing-box configuration from them. Values cross the
// boundary as strings (JSON) so the Kotlin side stays simple.
package mobile

import (
	"encoding/json"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"v2tun/internal/i18n"
	"v2tun/internal/profile"
)

var (
	mu       sync.Mutex
	dataDir  string
	db       *profile.DB
	settings Settings
)

// Init must be called first: dir is the app's private files folder, dict the
// content of i18n.json (shipped as an app asset).
func Init(dir string, dict []byte) {
	mu.Lock()
	defer mu.Unlock()
	dataDir = dir
	os.MkdirAll(dir, 0o700)
	d := append([]byte(nil), dict...)
	i18n.SetSource(func() []byte { return d })
	profile.StorePath = func() string { return filepath.Join(dataDir, "servers.json") }
	db = profile.Load()
	settings = loadSettings()
	i18n.SetLang(settings.Lang)
}

// T translates an English UI text into the current language.
func T(s string) string { return i18n.T(s) }

// Servers returns the server list and subscriptions as JSON.
func Servers() string {
	b, _ := json.Marshal(db.Snapshot())
	return string(b)
}

// AddText adds whatever was pasted or scanned: share links, configs, and
// subscription URLs (downloaded right away). Returns how many servers were added.
func AddText(text string) (int, error) {
	subs, rest := profile.SplitSubURLs(text)
	n := 0
	var firstErr error
	if strings.TrimSpace(rest) != "" {
		added, err := db.AddLinks(rest)
		n += added
		firstErr = err
	}
	for _, u := range subs {
		sub, err := db.AddSub("", u)
		if err != nil {
			if firstErr == nil {
				firstErr = err
			}
			continue
		}
		before := len(db.Snapshot().Servers)
		if err := UpdateSubscription(sub.ID); err != nil && firstErr == nil {
			firstErr = err
		}
		n += len(db.Snapshot().Servers) - before
	}
	if n > 0 {
		return n, nil
	}
	return 0, firstErr
}

var httpClient = &http.Client{Timeout: 30 * time.Second}

// UpdateSubscription downloads one subscription again. (While the VPN is on,
// the download goes through it, so the provider does not see the real IP.)
func UpdateSubscription(id string) error {
	var url string
	for _, s := range db.Snapshot().Subs {
		if s.ID == id {
			url = s.URL
		}
	}
	if url == "" {
		return nil
	}
	links, err := profile.FetchSub(httpClient, url)
	db.ApplySub(id, links, err)
	return err
}

// UpdateSubscriptions refreshes all subscriptions; returns the number that failed.
func UpdateSubscriptions() int {
	failed := 0
	for _, s := range db.Snapshot().Subs {
		if UpdateSubscription(s.ID) != nil {
			failed++
		}
	}
	return failed
}

func Select(id string)             { db.SetSelected(id) }
func RemoveServer(id string)       { db.RemoveServer(id) }
func RemoveSubscription(id string) { db.RemoveSub(id) }

// SetDelays stores real-delay results ({"server id": ms, ...}, -1 = failed).
func SetDelays(results string) {
	var d map[string]int
	if json.Unmarshal([]byte(results), &d) == nil {
		db.SetDelays(d)
	}
}
