//go:build windows

package main

import (
	"io"
	"net/http"
	"net/url"
	"testing"
	"time"
)

// What do the IP services answer through the selected server?
func TestExitInfoThroughCore(t *testing.T) {
	st := loadServerDB().Snapshot()
	servers := st.Usable()
	if len(servers) == 0 {
		t.Skip("no servers")
	}
	up, err := (&App{}).coreUplink()
	if err != nil {
		t.Fatal(err)
	}
	var c coreManager
	if err := c.Start(up, servers, st.Selected); err != nil {
		t.Fatal(err)
	}
	defer c.Stop()
	pu, _ := url.Parse("http://" + c.Addr())
	hc := http.Client{Timeout: 15 * time.Second, Transport: &http.Transport{Proxy: http.ProxyURL(pu)}}
	for _, u := range []string{"https://ipinfo.io/json", "https://ipwho.is/", "https://api.ip.sb/geoip", "http://ip-api.com/json", "https://ifconfig.co/json", "https://api.ipify.org?format=json"} {
		resp, err := hc.Get(u)
		if err != nil {
			t.Logf("%-32s ERR %v", u, err)
			continue
		}
		b, _ := io.ReadAll(io.LimitReader(resp.Body, 300))
		resp.Body.Close()
		t.Logf("%-32s %d %q", u, resp.StatusCode, b)
	}
	// and directly, without the proxy
	for _, u := range []string{"https://ipinfo.io/json"} {
		resp, err := http.Get(u)
		if err != nil {
			t.Logf("DIRECT %s ERR %v", u, err)
			continue
		}
		b, _ := io.ReadAll(io.LimitReader(resp.Body, 200))
		resp.Body.Close()
		t.Logf("DIRECT %s %d %q", u, resp.StatusCode, b)
	}
}
