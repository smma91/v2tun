//go:build windows

package main

import (
	"encoding/json"
	"net/http"
	"os"
	"testing"
	"time"
)

// Fetches the subscription the user added (stored as a server by mistake).
func TestUserSubscription(t *testing.T) {
	b, err := os.ReadFile(storePath())
	if err != nil {
		t.Skip(err)
	}
	var st ServerStore
	json.Unmarshal(b, &st)
	var u string
	for _, s := range st.Servers {
		if isSubURL(s.Link) {
			u = s.Link
		}
	}
	for _, s := range st.Subs {
		u = s.URL
	}
	if u == "" {
		t.Skip("no subscription")
	}
	subs, rest := SplitSubURLs(u)
	if len(subs) != 1 || rest != "" {
		t.Fatalf("not detected as subscription")
	}
	links, err := fetchSub(&http.Client{Timeout: 20 * time.Second}, u)
	if err != nil {
		t.Fatal(err)
	}
	for _, l := range links {
		s := newServer(l, "x")
		t.Logf("%-12s %-30s err=%q", s.Type, s.Name, s.Error)
		if s.Error == "" {
			validate(t, s)
		}
	}
}
