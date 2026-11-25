// Package netcheck asks which public IP (and country) the internet sees.
// Shared by the desktop and the mobile app.
package netcheck

import (
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"

	"v2tun/internal/profile"
)

type ExitInfo struct {
	IP       string `json:"ip"`
	City     string `json:"city"`
	Country  string `json:"country"`
	Org      string `json:"org"`
	Timezone string `json:"timezone"`
}

var str, sub = profile.Str, profile.Sub

// IP lookup services, tried in order: any of them may be blocked or rate
// limited. Each returns JSON; field names are mapped to ExitInfo.
var services = []struct {
	url   string
	parse func(map[string]any) ExitInfo
}{
	{"https://ipinfo.io/json", func(m map[string]any) ExitInfo {
		return ExitInfo{IP: str(m["ip"]), City: str(m["city"]), Country: str(m["country"]), Org: str(m["org"]), Timezone: str(m["timezone"])}
	}},
	{"https://ipwho.is/", func(m map[string]any) ExitInfo {
		c, tz := sub(m["connection"]), sub(m["timezone"])
		org := str(c["org"])
		if asn := str(c["asn"]); asn != "" {
			org = "AS" + asn + " " + org
		}
		return ExitInfo{IP: str(m["ip"]), City: str(m["city"]), Country: str(m["country_code"]), Org: org, Timezone: str(tz["id"])}
	}},
	{"https://api.ip.sb/geoip", func(m map[string]any) ExitInfo {
		org := str(m["organization"])
		if asn := str(m["asn"]); asn != "" {
			org = "AS" + asn + " " + org
		}
		return ExitInfo{IP: str(m["ip"]), City: str(m["city"]), Country: str(m["country_code"]), Org: org, Timezone: str(m["timezone"])}
	}},
	{"https://api.ipify.org?format=json", func(m map[string]any) ExitInfo { return ExitInfo{IP: str(m["ip"])} }},
}

// Exit asks the IP services in turn with client c.
func Exit(c *http.Client) (ExitInfo, error) {
	var lastErr error
	for _, s := range services {
		req, _ := http.NewRequest("GET", s.url, nil)
		req.Header.Set("Accept", "application/json")
		req.Header.Set("User-Agent", "v2tun")
		resp, err := c.Do(req)
		if err != nil {
			lastErr = err
			continue
		}
		var m map[string]any
		err = json.NewDecoder(io.LimitReader(resp.Body, 64<<10)).Decode(&m)
		resp.Body.Close()
		if err != nil {
			// An HTML page instead of JSON: typically the ISP's filtering page.
			lastErr = fmt.Errorf("%s: HTTP %d, %s", s.url, resp.StatusCode, "not JSON (blocked or filtered)")
			continue
		}
		if info := s.parse(m); net.ParseIP(info.IP) != nil {
			return info, nil
		}
		lastErr = fmt.Errorf("%s: no IP in answer", s.url)
	}
	if lastErr == nil {
		lastErr = errors.New("no IP service answered")
	}
	return ExitInfo{}, lastErr
}
