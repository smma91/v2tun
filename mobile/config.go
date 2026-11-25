package mobile

import (
	"encoding/json"
	"net/netip"
	"strings"

	"v2tun/internal/profile"
)

// BuildConfig returns the sing-box configuration for the VpnService. Same
// behaviour as the desktop tunnel:
//
//   - every app through the selected server ("auto" = fastest), except the
//     split-tunnel lists; private networks stay direct
//   - remote DNS: names get FakeIP addresses and are resolved by the server
//   - IPv6 is captured and refused (the servers' DNS is IPv4), so it cannot leak
//   - QUIC is refused so apps use TCP, unless allowed
//   - a local SOCKS5 / HTTP proxy, optionally for other devices
func BuildConfig() (string, error) {
	mu.Lock()
	s := settings
	mu.Unlock()
	st := db.Snapshot()
	outbounds, endpoints, _, err := profile.Outbounds(st.Usable(), st.Selected, nil)
	if err != nil {
		return "", err
	}
	cfg := buildConfig(s, outbounds, endpoints)
	b, err := json.Marshal(cfg)
	return string(b), err
}

func buildConfig(s Settings, outbounds, endpoints []any) map[string]any {
	ips, suffixes := splitRules(s.Domains)

	tun := map[string]any{
		"type": "tun", "tag": "tun",
		"address":      []string{"172.19.0.1/30", "fdfe:dcba:9876::1/126"},
		"mtu":          9000,
		"auto_route":   true,
		"strict_route": true,
		"stack":        "mixed",
	}
	if len(s.Apps) > 0 { // Android routes apps itself (VpnService allowed / disallowed apps)
		if s.AppMode == "include" {
			tun["include_package"] = s.Apps
		} else {
			tun["exclude_package"] = s.Apps
		}
	}
	listen := "127.0.0.1"
	if s.ProxyLAN {
		listen = "0.0.0.0"
	}
	inbounds := []any{tun, map[string]any{"type": "mixed", "tag": "proxy", "listen": listen, "listen_port": s.ProxyPort}}

	// DNS
	var dnsServers []any
	var dnsRules []any
	upstream := "1.1.1.1"
	for _, d := range s.DNS {
		if a, err := netip.ParseAddr(d); err == nil && a.Is4() {
			upstream = d
			break
		}
	}
	dnsServers = append(dnsServers,
		map[string]any{"type": "tcp", "tag": "remote", "server": upstream, "detour": "select"},
		map[string]any{"type": "local", "tag": "local"},
	)
	listed := map[string]any{}
	if len(suffixes) > 0 {
		listed["domain_suffix"] = suffixes
	}
	// Names that go direct are resolved for real, outside the tunnel.
	if s.DomainMode == "exclude" && len(suffixes) > 0 {
		dnsRules = append(dnsRules, merge(map[string]any{"server": "local"}, listed))
	}
	if !s.LocalDNS {
		dnsServers = append(dnsServers, map[string]any{"type": "fakeip", "tag": "fake", "inet4_range": "198.18.0.0/15"})
		r := map[string]any{"query_type": []string{"A", "AAAA"}, "server": "fake"}
		if s.DomainMode == "include" && len(suffixes) > 0 {
			r = merge(r, listed) // only the listed names use the tunnel
		}
		dnsRules = append(dnsRules, r)
	}
	final := "remote"
	if s.DomainMode == "include" && len(suffixes) > 0 {
		final = "local"
	}
	dns := map[string]any{"servers": dnsServers, "rules": dnsRules, "final": final, "strategy": "ipv4_only"}

	// Routing
	rules := []any{
		map[string]any{"action": "sniff"},
		map[string]any{"protocol": "dns", "action": "hijack-dns"},
		map[string]any{"ip_is_private": true, "outbound": "direct"},
	}
	list := map[string]any{}
	if len(suffixes) > 0 {
		list["domain_suffix"] = suffixes
	}
	if len(ips) > 0 {
		list["ip_cidr"] = ips
	}
	routeFinal := "select"
	if len(list) > 0 {
		if s.DomainMode == "include" {
			rules = append(rules, merge(map[string]any{"outbound": "select"}, list))
			routeFinal = "direct"
		} else {
			rules = append(rules, merge(map[string]any{"outbound": "direct"}, list))
		}
	}
	// Everything still here goes through the tunnel.
	rules = append(rules, map[string]any{"ip_version": 6, "action": "reject"})
	if !s.AllowQUIC {
		rules = append(rules, map[string]any{"network": "udp", "port": []int{443, 80}, "action": "reject"})
	}

	cfg := map[string]any{
		"log":       map[string]any{"level": "warn"},
		"dns":       dns,
		"inbounds":  inbounds,
		"outbounds": outbounds,
		"route": map[string]any{
			"rules":                   rules,
			"final":                   routeFinal,
			"auto_detect_interface":   true,
			"default_domain_resolver": "local",
		},
	}
	if len(endpoints) > 0 {
		cfg["endpoints"] = endpoints
	}
	return cfg
}

// splitRules turns the domain list ("example.com", ".com", "*.example.org",
// "10.0.0.0/8", one per entry) into sing-box rule fields. A name also covers
// its subdomains, as on the desktop.
func splitRules(entries []string) (ips, suffixes []string) {
	for _, e := range entries {
		e = strings.ToLower(strings.TrimSpace(e))
		if e == "" || strings.HasPrefix(e, "#") {
			continue
		}
		if p, err := netip.ParsePrefix(e); err == nil {
			ips = append(ips, p.String())
			continue
		}
		if a, err := netip.ParseAddr(e); err == nil {
			ips = append(ips, netip.PrefixFrom(a, a.BitLen()).String())
			continue
		}
		e = strings.TrimSuffix(strings.TrimPrefix(strings.TrimPrefix(e, "*"), "."), ".")
		if e != "" {
			suffixes = append(suffixes, e)
		}
	}
	return ips, suffixes
}

func merge(a, b map[string]any) map[string]any {
	out := map[string]any{}
	for k, v := range b {
		out[k] = v
	}
	for k, v := range a {
		out[k] = v
	}
	return out
}
