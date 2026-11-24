//go:build windows

package main

import (
	"context"
	"encoding/binary"
	"io"
	"net/http"
	"net/netip"
	"net/url"
	"os"
	"sort"
	"testing"
	"time"

	M "github.com/xjasonlyu/tun2socks/v2/metadata"
	"github.com/xjasonlyu/tun2socks/v2/proxy/socks5"
)

// Full check of a subscription through the embedded core:
// fetch -> parse -> run all servers -> delay test -> HTTPS + DNS through the best one.
func TestLiveSubscription(t *testing.T) {
	u := os.Getenv("V2TUN_SUB")
	if u == "" {
		t.Skip("set V2TUN_SUB")
	}
	links, err := fetchSub(&http.Client{Timeout: 20 * time.Second}, u)
	if err != nil {
		t.Fatal(err)
	}
	var servers []*Server
	for _, l := range links {
		s := newServer(l, "sub")
		if s.Error != "" {
			t.Logf("skip %s: %s", s.Name, s.Error)
			continue
		}
		servers = append(servers, s)
	}
	t.Logf("subscription: %d links, %d usable", len(links), len(servers))

	up, err := findUplink(nil)
	if err != nil {
		t.Fatal(err)
	}
	var c coreManager
	if err := c.Start(up, servers, "auto"); err != nil {
		t.Fatal(err)
	}
	defer c.Stop()
	var ids []string
	for _, s := range servers {
		ids = append(ids, s.ID)
	}
	res := c.Test(ids)
	sort.Slice(servers, func(i, j int) bool {
		a, b := res[servers[i].ID], res[servers[j].ID]
		if (a > 0) != (b > 0) {
			return a > 0
		}
		return a < b
	})
	for _, s := range servers {
		t.Logf("%6dms  %-12s %-28s %s", res[s.ID], s.Type, s.Name, s.Addr)
	}
	best := servers[0]
	if res[best.ID] <= 0 {
		t.Fatal("no working server")
	}
	c.Select(best.ID)
	t.Logf("selected: %s", best.Name)

	// HTTPS through the core
	pu, _ := url.Parse("http://" + c.Addr())
	hc := http.Client{Timeout: 20 * time.Second, Transport: &http.Transport{Proxy: http.ProxyURL(pu)}}
	for _, target := range []string{"https://ipinfo.io/json", "https://www.youtube.com/generate_204"} {
		start := time.Now()
		resp, err := hc.Get(target)
		if err != nil {
			t.Errorf("%s: %v", target, err)
			continue
		}
		b, _ := io.ReadAll(io.LimitReader(resp.Body, 400))
		resp.Body.Close()
		t.Logf("%s -> %d in %v %s", target, resp.StatusCode, time.Since(start).Round(time.Millisecond), b)
	}

	// UDP through the core (DNS to 8.8.8.8 over SOCKS UDP)
	s5, _ := socks5.New(c.Addr(), "", "")
	m := &M.Metadata{Network: M.UDP, DstIP: netip.MustParseAddr("8.8.8.8"), DstPort: 53}
	pc, err := s5.DialUDP(m)
	if err == nil {
		q := dnsQuery(t, "www.google.com.")
		pc.WriteTo(q, m.UDPAddr())
		pc.SetReadDeadline(time.Now().Add(5 * time.Second))
		buf := make([]byte, 1500)
		n, _, err := pc.ReadFrom(buf)
		t.Logf("UDP via %s: %d bytes err=%v", best.Name, n, err)
		pc.Close()
	}
	// DNS over TCP through the core
	dt := newDNSTCP(&routerDialer{socks: s5})
	defer dt.close()
	q := dnsQuery(t, "www.youtube.com.")
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	_ = ctx
	resp, err := dt.query(netip.MustParseAddr("8.8.8.8"), q)
	if err != nil {
		t.Errorf("DNS over TCP: %v", err)
	} else {
		d := newDNSMap()
		d.record(resp)
		t.Logf("DNS over TCP: %d addresses (%d bytes, id %d)", len(d.m), len(resp), binary.BigEndian.Uint16(resp))
	}
}
