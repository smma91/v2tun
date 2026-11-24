package main

import (
	"crypto/sha1"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"
)

type Server struct {
	ID     string  `json:"id"`
	Sub    string  `json:"sub"` // subscription id, "" = added manually
	Name   string  `json:"name"`
	Link   string  `json:"link"`
	Type   string  `json:"type"`
	Addr   string  `json:"addr"`
	Delay  int     `json:"delay"` // real delay through the server, ms; 0 untested, -1 failed
	Ping   int     `json:"ping"`  // network ping to the server, ms; 0 untested, -1 failed
	Speed  float64 `json:"speed"` // download Mbit/s; 0 untested, -1 failed
	Error  string  `json:"error"` // why it cannot be used
	Tested int64   `json:"tested"`
}

func (s *Server) outbound() (map[string]any, error) {
	ob, _, err := parseLink(s.Link)
	return ob, err
}

type Subscription struct {
	ID      string `json:"id"`
	Name    string `json:"name"`
	URL     string `json:"url"`
	Updated int64  `json:"updated"`
	Count   int    `json:"count"`
	Error   string `json:"error"`
}

type ServerStore struct {
	Subs      []*Subscription `json:"subs"`
	Servers   []*Server       `json:"servers"`
	Selected  string          `json:"selected"`  // server id or "auto"
	AutoHours int             `json:"autoHours"` // subscription auto update interval, 0 = off
	Failover  bool            `json:"failover"`  // switch away from a dead selected server
}

type serverDB struct {
	mu sync.Mutex
	st ServerStore
}

func storePath() string { return filepath.Join(dataDir(), "servers.json") }

func loadServerDB() *serverDB {
	db := &serverDB{st: ServerStore{Selected: "auto", AutoHours: 12, Failover: true}}
	if b, err := os.ReadFile(storePath()); err == nil {
		json.Unmarshal(b, &db.st)
	}
	if db.st.Selected == "" {
		db.st.Selected = "auto"
	}
	return db
}

func (db *serverDB) saveLocked() {
	b, _ := json.MarshalIndent(db.st, "", "  ")
	writeSecret(storePath(), b)
}

// Snapshot returns a deep-enough copy for the UI / core.
func (db *serverDB) Snapshot() ServerStore {
	db.mu.Lock()
	defer db.mu.Unlock()
	st := db.st
	st.Subs = make([]*Subscription, len(db.st.Subs))
	for i, s := range db.st.Subs {
		c := *s
		st.Subs[i] = &c
	}
	st.Servers = make([]*Server, len(db.st.Servers))
	for i, s := range db.st.Servers {
		c := *s
		st.Servers[i] = &c
	}
	return st
}

func linkID(link string) string {
	h := sha1.Sum([]byte(strings.TrimSpace(link)))
	return "s" + hex.EncodeToString(h[:6])
}

func newServer(link, sub string) *Server {
	s := &Server{ID: linkID(link), Sub: sub, Link: strings.TrimSpace(link)}
	if needsXray(s.Link) {
		if u, err := url.Parse(s.Link); err == nil {
			s.Type, s.Name, s.Addr = "vless-xhttp", u.Fragment, u.Host
		}
		if s.Name == "" {
			s.Name = s.Addr
		}
		return s
	}
	ob, name, err := parseLink(link)
	if err != nil {
		s.Error = err.Error()
	}
	if ob != nil {
		s.Type, _ = ob["type"].(string)
		if h, ok := ob["server"].(string); ok && ob["server_port"] == nil {
			s.Addr = strings.TrimPrefix(h, "https://")
		} else if ok {
			s.Addr = fmt.Sprintf("%s:%v", h, ob["server_port"])
		} else if rs, ok := ob["servers"].([]any); ok && len(rs) > 0 {
			r, _ := rs[0].(map[string]any)
			s.Addr = fmt.Sprintf("%v:%v", r["server"], r["server_port"])
		} else if peers, ok := ob["peers"].([]any); ok && len(peers) > 0 {
			p := peers[0].(map[string]any)
			s.Addr = fmt.Sprintf("%v:%v", p["address"], p["port"])
		}
	}
	if name == "" {
		name = s.Addr
	}
	if s.Type == "" {
		if i := strings.Index(link, "://"); i > 0 {
			s.Type = link[:i]
		}
	}
	s.Name = name
	return s
}

// isSubURL: an http(s) address with a path and no credentials is a
// subscription, not an HTTP proxy server (http://user:pass@host:port).
func isSubURL(l string) bool {
	u, err := url.Parse(strings.TrimSpace(l))
	if err != nil || (u.Scheme != "http" && u.Scheme != "https") || u.User != nil {
		return false
	}
	return (u.Path != "" && u.Path != "/") || u.RawQuery != ""
}

// SplitSubURLs separates subscription addresses from server links in pasted text.
func SplitSubURLs(text string) (subs []string, rest string) {
	var keep []string
	for _, l := range strings.Split(strings.ReplaceAll(text, "\r", ""), "\n") {
		if isSubURL(l) {
			subs = append(subs, strings.TrimSpace(l))
		} else {
			keep = append(keep, l)
		}
	}
	return subs, strings.Join(keep, "\n")
}

// migrateSubs fixes subscription URLs that were stored as HTTP proxy servers.
func (db *serverDB) migrateSubs() []string {
	db.mu.Lock()
	var urls []string
	for _, s := range db.st.Servers {
		if s.Sub == "" && isSubURL(s.Link) {
			urls = append(urls, s.Link)
		}
	}
	if len(urls) > 0 {
		db.removeWhereLocked(func(s *Server) bool { return s.Sub == "" && isSubURL(s.Link) })
		db.saveLocked()
	}
	db.mu.Unlock()
	return urls
}

// AddLinks adds manually pasted links; returns how many were new.
func (db *serverDB) AddLinks(text string) (int, error) {
	links := splitLinks(text)
	if len(links) == 0 {
		return 0, trErr("No valid link or config found")
	}
	db.mu.Lock()
	defer db.mu.Unlock()
	have := map[string]bool{}
	for _, s := range db.st.Servers {
		have[s.ID] = true
	}
	n := 0
	for _, l := range links {
		s := newServer(l, "")
		if have[s.ID] {
			continue
		}
		have[s.ID] = true
		db.st.Servers = append(db.st.Servers, s)
		n++
	}
	db.saveLocked()
	return n, nil
}

func (db *serverDB) AddSub(name, rawURL string) (*Subscription, error) {
	if !strings.HasPrefix(rawURL, "http://") && !strings.HasPrefix(rawURL, "https://") {
		return nil, trErr("The subscription URL must start with http:// or https://")
	}
	db.mu.Lock()
	defer db.mu.Unlock()
	for _, s := range db.st.Subs {
		if s.URL == rawURL {
			return s, nil
		}
	}
	if name == "" {
		name = rawURL
		if i := strings.Index(rawURL, "://"); i > 0 {
			name = strings.SplitN(rawURL[i+3:], "/", 2)[0]
		}
	}
	sub := &Subscription{ID: "sub" + linkID(rawURL)[1:], Name: name, URL: rawURL}
	db.st.Subs = append(db.st.Subs, sub)
	db.saveLocked()
	return sub, nil
}

func (db *serverDB) RemoveSub(id string) {
	db.mu.Lock()
	defer db.mu.Unlock()
	subs := db.st.Subs[:0]
	for _, s := range db.st.Subs {
		if s.ID != id {
			subs = append(subs, s)
		}
	}
	db.st.Subs = subs
	db.removeWhereLocked(func(s *Server) bool { return s.Sub == id })
	db.saveLocked()
}

func (db *serverDB) RemoveServer(id string) {
	db.mu.Lock()
	defer db.mu.Unlock()
	db.removeWhereLocked(func(s *Server) bool { return s.ID == id })
	db.saveLocked()
}

func (db *serverDB) removeWhereLocked(drop func(*Server) bool) {
	keep := db.st.Servers[:0]
	for _, s := range db.st.Servers {
		if !drop(s) {
			keep = append(keep, s)
		}
	}
	db.st.Servers = keep
	if db.st.Selected != "auto" && db.findLocked(db.st.Selected) == nil {
		db.st.Selected = "auto"
	}
}

func (db *serverDB) findLocked(id string) *Server {
	for _, s := range db.st.Servers {
		if s.ID == id {
			return s
		}
	}
	return nil
}

func (db *serverDB) SetSelected(id string) {
	db.mu.Lock()
	defer db.mu.Unlock()
	db.st.Selected = id
	db.saveLocked()
}

func (db *serverDB) SetOptions(autoHours int, failover bool) {
	db.mu.Lock()
	defer db.mu.Unlock()
	db.st.AutoHours, db.st.Failover = autoHours, failover
	db.saveLocked()
}

func (db *serverDB) SetPings(d map[string]int) {
	db.mu.Lock()
	defer db.mu.Unlock()
	for _, s := range db.st.Servers {
		if v, ok := d[s.ID]; ok {
			s.Ping = v
		}
	}
	db.saveLocked()
}

func (db *serverDB) SetSpeed(id string, mbps float64) {
	db.mu.Lock()
	defer db.mu.Unlock()
	if s := db.findLocked(id); s != nil {
		s.Speed = mbps
	}
	db.saveLocked()
}

func (db *serverDB) SetDelays(d map[string]int) {
	db.mu.Lock()
	defer db.mu.Unlock()
	now := time.Now().Unix()
	for _, s := range db.st.Servers {
		if v, ok := d[s.ID]; ok {
			s.Delay, s.Tested = v, now
		}
	}
	db.saveLocked()
}

// fetchSub downloads a subscription. get is the HTTP client to use (through
// the tunnel when it is up, so the provider does not see the real IP).
func fetchSub(client *http.Client, rawURL string) ([]string, error) {
	req, err := http.NewRequest("GET", rawURL, nil)
	if err != nil {
		return nil, err
	}
	// Providers return the plain link list to v2rayN-compatible clients.
	req.Header.Set("User-Agent", "v2rayN/7.10.0")
	resp, err := client.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	if resp.StatusCode != 200 {
		return nil, fmt.Errorf("HTTP %d", resp.StatusCode)
	}
	b, err := io.ReadAll(io.LimitReader(resp.Body, 8<<20))
	if err != nil {
		return nil, err
	}
	links := splitLinks(string(b))
	if len(links) == 0 {
		return nil, trErr("The subscription did not contain any link")
	}
	return links, nil
}

// ApplySub replaces the servers of a subscription, keeping delays of
// servers that did not change.
func (db *serverDB) ApplySub(id string, links []string, fetchErr error) {
	db.mu.Lock()
	defer db.mu.Unlock()
	var sub *Subscription
	for _, s := range db.st.Subs {
		if s.ID == id {
			sub = s
		}
	}
	if sub == nil {
		return
	}
	if fetchErr != nil {
		sub.Error = fetchErr.Error()
		db.saveLocked()
		return
	}
	old := map[string]*Server{}
	for _, s := range db.st.Servers {
		if s.Sub == id {
			old[s.ID] = s
		}
	}
	db.removeWhereLocked(func(s *Server) bool { return s.Sub == id })
	seen := map[string]bool{}
	for _, l := range links {
		s := newServer(l, id)
		if seen[s.ID] {
			continue
		}
		seen[s.ID] = true
		if o := old[s.ID]; o != nil {
			s.Delay, s.Tested, s.Ping, s.Speed = o.Delay, o.Tested, o.Ping, o.Speed
		}
		db.st.Servers = append(db.st.Servers, s)
	}
	sub.Updated, sub.Count, sub.Error = time.Now().Unix(), len(seen), ""
	if db.st.Selected != "auto" && db.findLocked(db.st.Selected) == nil {
		db.st.Selected = "auto"
	}
	db.saveLocked()
}

// Usable returns the servers the core can run.
func (st ServerStore) Usable() []*Server {
	var out []*Server
	for _, s := range st.Servers {
		if s.Error == "" {
			out = append(out, s)
		}
	}
	return out
}
