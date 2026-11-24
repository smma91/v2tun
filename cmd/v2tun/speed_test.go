//go:build windows

package main

import (
	"net/http"
	"os"
	"testing"
	"time"
)

func TestPingAndSpeed(t *testing.T) {
	var servers []*Server
	if u := os.Getenv("V2TUN_SUB"); u != "" {
		if links, err := fetchSub(&http.Client{Timeout: 20 * time.Second}, u); err == nil {
			for _, l := range links {
				if s := newServer(l, "sub"); s.Error == "" {
					servers = append(servers, s)
				}
			}
		}
	}
	if len(servers) == 0 { // subscription unreachable: use the servers saved by the app
		servers = loadServerDB().Snapshot().Usable()
	}
	if len(servers) == 0 {
		t.Skip("no servers")
	}
	up, err := (&App{}).coreUplink() // the real network, never the tunnel
	if err != nil {
		t.Fatal(err)
	}
	pings := pingServers(up, servers)
	var c coreManager
	if err := c.Start(up, servers, "auto"); err != nil {
		t.Fatal(err)
	}
	defer c.Stop()
	for _, s := range servers[:3] {
		mbps, err := c.Speed(s.ID, 5*time.Second)
		t.Logf("%-24s ping %4dms  speed %6.2f Mbps err=%v", s.Name, pings[s.ID], mbps, err)
	}
	for _, s := range servers[3:] {
		t.Logf("%-24s ping %4dms", s.Name, pings[s.ID])
	}
}
