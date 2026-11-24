package main

import (
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"net"
	"net/url"
	"strconv"
	"strings"
)

// parseLink converts a share link (vless://, vmess://, trojan://, ss://,
// hysteria2://, hy2://, hysteria://, tuic://, wireguard://, wg://, ssh://,
// socks://, http(s)://, anytls://) or a sing-box outbound JSON object into a
// sing-box outbound. It also returns a display name.
func parseLink(link string) (map[string]any, string, error) {
	link = strings.TrimSpace(link)
	if strings.HasPrefix(link, "{") {
		var ob map[string]any
		if err := json.Unmarshal([]byte(link), &ob); err != nil {
			return nil, "", err
		}
		if _, ok := ob["type"].(string); !ok {
			return nil, "", errors.New("JSON without type")
		}
		name, _ := ob["tag"].(string)
		delete(ob, "tag")
		return ob, name, nil
	}
	i := strings.Index(link, "://")
	if i < 0 {
		return nil, "", errors.New("not a link")
	}
	scheme := strings.ToLower(link[:i])
	switch scheme {
	case "vmess":
		return parseVMess(link[i+3:])
	case "ss":
		return parseSS(link)
	}
	u, err := url.Parse(link)
	if err != nil {
		return nil, "", err
	}
	name := u.Fragment
	q := u.Query()
	host := u.Hostname()
	port, _ := strconv.Atoi(u.Port())
	if host == "" {
		return nil, "", errors.New("no server address")
	}
	base := func(typ string, defPort int) map[string]any {
		if port == 0 {
			port = defPort
		}
		return map[string]any{"type": typ, "server": host, "server_port": port}
	}
	user := u.User.Username()
	pass, _ := u.User.Password()

	switch scheme {
	case "vless":
		ob := base("vless", 443)
		ob["uuid"] = user
		if f := q.Get("flow"); f != "" {
			ob["flow"] = f
		}
		ob["packet_encoding"] = "xudp"
		if err := v2Transport(ob, q.Get("type"), q.Get("headerType"), q.Get("path"), q.Get("host"), q.Get("serviceName"), q.Get("mode")); err != nil {
			return nil, name, err
		}
		v2TLS(ob, q.Get("security"), firstNonEmpty(q.Get("sni"), q.Get("peer")), q.Get("fp"), q.Get("alpn"),
			truthy(q.Get("allowInsecure"), q.Get("insecure")), q.Get("pbk"), q.Get("sid"), host)
		return ob, name, nil

	case "trojan":
		ob := base("trojan", 443)
		ob["password"] = user
		if err := v2Transport(ob, q.Get("type"), q.Get("headerType"), q.Get("path"), q.Get("host"), q.Get("serviceName"), q.Get("mode")); err != nil {
			return nil, name, err
		}
		sec := q.Get("security")
		if sec == "" {
			sec = "tls"
		}
		v2TLS(ob, sec, firstNonEmpty(q.Get("sni"), q.Get("peer")), q.Get("fp"), q.Get("alpn"),
			truthy(q.Get("allowInsecure"), q.Get("insecure")), q.Get("pbk"), q.Get("sid"), host)
		return ob, name, nil

	case "hysteria2", "hy2":
		ob := base("hysteria2", 443)
		ob["password"] = firstNonEmpty(pass, user, q.Get("auth"))
		if pass != "" && user != "" {
			ob["password"] = user + ":" + pass
		}
		if ports := firstNonEmpty(q.Get("mport"), q.Get("ports")); ports != "" {
			var list []string
			for _, p := range strings.Split(ports, ",") {
				list = append(list, strings.ReplaceAll(strings.TrimSpace(p), "-", ":"))
			}
			ob["server_ports"] = list
		}
		if o := q.Get("obfs"); o != "" && o != "none" {
			ob["obfs"] = map[string]any{"type": o, "password": q.Get("obfs-password")}
		}
		ob["tls"] = quicTLS(firstNonEmpty(q.Get("sni"), q.Get("peer"), host), truthy(q.Get("insecure"), q.Get("allowInsecure")), q.Get("alpn"), "h3")
		return ob, name, nil

	case "hysteria":
		ob := base("hysteria", 443)
		if v := firstNonEmpty(q.Get("auth"), q.Get("auth_str")); v != "" {
			ob["auth_str"] = v
		}
		ob["up_mbps"] = atoiDef(firstNonEmpty(q.Get("upmbps"), q.Get("up")), 20)
		ob["down_mbps"] = atoiDef(firstNonEmpty(q.Get("downmbps"), q.Get("down")), 100)
		if o := firstNonEmpty(q.Get("obfsParam"), q.Get("obfs")); o != "" && o != "xplus" {
			ob["obfs"] = o
		}
		ob["tls"] = quicTLS(firstNonEmpty(q.Get("peer"), q.Get("sni"), host), truthy(q.Get("insecure")), q.Get("alpn"), "hysteria")
		return ob, name, nil

	case "tuic":
		ob := base("tuic", 443)
		ob["uuid"] = user
		ob["password"] = pass
		ob["congestion_control"] = firstNonEmpty(q.Get("congestion_control"), "bbr")
		if m := q.Get("udp_relay_mode"); m != "" {
			ob["udp_relay_mode"] = m
		}
		ob["tls"] = quicTLS(firstNonEmpty(q.Get("sni"), host), truthy(q.Get("allow_insecure"), q.Get("insecure")), q.Get("alpn"), "h3")
		return ob, name, nil

	case "anytls":
		ob := base("anytls", 443)
		ob["password"] = firstNonEmpty(user, pass)
		ob["tls"] = map[string]any{"enabled": true, "server_name": firstNonEmpty(q.Get("sni"), host),
			"insecure": truthy(q.Get("insecure"), q.Get("allowInsecure"))}
		return ob, name, nil

	case "wireguard", "wg":
		ob := map[string]any{"type": "wireguard", "private_key": firstNonEmpty(user, q.Get("privatekey"), q.Get("private_key"))}
		if port == 0 {
			port = 51820
		}
		addr := firstNonEmpty(q.Get("address"), q.Get("ip"), "172.16.0.2/32")
		var addrs []string
		for _, a := range strings.Split(addr, ",") {
			a = strings.TrimSpace(a)
			if !strings.Contains(a, "/") {
				if strings.Contains(a, ":") {
					a += "/128"
				} else {
					a += "/32"
				}
			}
			addrs = append(addrs, a)
		}
		ob["address"] = addrs
		if m := atoiDef(q.Get("mtu"), 0); m > 0 {
			ob["mtu"] = m
		}
		peer := map[string]any{"address": host, "port": port,
			"public_key":  firstNonEmpty(q.Get("publickey"), q.Get("public_key"), q.Get("peer")),
			"allowed_ips": []string{"0.0.0.0/0", "::/0"}}
		if psk := firstNonEmpty(q.Get("presharedkey"), q.Get("psk")); psk != "" {
			peer["pre_shared_key"] = psk
		}
		if r := q.Get("reserved"); r != "" {
			var res []int
			for _, x := range strings.Split(r, ",") {
				res = append(res, atoiDef(strings.TrimSpace(x), 0))
			}
			peer["reserved"] = res
		}
		ob["peers"] = []any{peer}
		return ob, name, nil

	case "ssh":
		ob := base("ssh", 22)
		ob["user"] = firstNonEmpty(user, "root")
		if pass != "" {
			ob["password"] = pass
		}
		if pk := firstNonEmpty(q.Get("pk"), q.Get("private_key")); pk != "" {
			ob["private_key"] = pk
		}
		if hk := q.Get("host_key"); hk != "" {
			ob["host_key"] = []string{hk}
		}
		return ob, name, nil

	case "socks", "socks5", "socks4", "socks4a":
		ob := base("socks", 1080)
		if user != "" && pass == "" { // v2rayN style: base64(user:pass)
			if d, err := b64(user); err == nil && strings.Contains(string(d), ":") {
				user, pass, _ = strings.Cut(string(d), ":")
			}
		}
		if user != "" {
			ob["username"], ob["password"] = user, pass
		}
		if strings.HasPrefix(scheme, "socks4") {
			ob["version"] = "4a"
		}
		return ob, name, nil

	case "naive+https", "naive+quic", "naive":
		ob := base("naive", 443)
		if user != "" {
			ob["username"], ob["password"] = user, pass
		}
		if scheme == "naive+quic" {
			ob["quic"] = true
		}
		ob["tls"] = map[string]any{"enabled": true, "server_name": firstNonEmpty(q.Get("sni"), host)}
		return ob, name, nil

	// OpenConnect family (Cisco AnyConnect / ocserv, Fortinet, GlobalProtect, Pulse, F5):
	// openconnect://user:pass@vpn.example.com:443?flavor=anyconnect&group=GROUP#name
	case "openconnect", "anyconnect", "ocserv", "fortinet", "globalprotect", "gp", "pulse", "f5":
		flavor := map[string]string{"globalprotect": "gp", "ocserv": "anyconnect", "openconnect": ""}[scheme]
		if _, known := map[string]bool{"anyconnect": true, "fortinet": true, "gp": true, "pulse": true, "f5": true}[scheme]; known {
			flavor = scheme
		}
		flavor = firstNonEmpty(q.Get("flavor"), q.Get("protocol"), flavor, "anyconnect")
		srv := "https://" + host
		if port != 0 && port != 443 {
			srv += ":" + strconv.Itoa(port)
		}
		if u.Path != "" && u.Path != "/" {
			srv += u.Path
		}
		ob := map[string]any{"type": "openconnect", "server": srv, "flavor": flavor}
		if user != "" {
			ob["username"], ob["password"] = user, pass
		}
		if g := firstNonEmpty(q.Get("group"), q.Get("authgroup")); g != "" {
			ob["auth_group"] = g
		}
		if truthy(q.Get("insecure"), q.Get("allowInsecure")) {
			ob["tls"] = map[string]any{"insecure": true}
		}
		if fp := firstNonEmpty(q.Get("servercert"), q.Get("pin")); fp != "" {
			ob["tls"] = map[string]any{"peer_fingerprint": []string{fp}}
		}
		return ob, name, nil

	case "http", "https":
		ob := base("http", map[bool]int{true: 443, false: 80}[scheme == "https"])
		if user != "" {
			ob["username"], ob["password"] = user, pass
		}
		if scheme == "https" {
			ob["tls"] = map[string]any{"enabled": true, "server_name": firstNonEmpty(q.Get("sni"), host)}
		}
		return ob, name, nil
	}
	return nil, name, trErr("Protocol %s is not supported", scheme)
}

// v2rayN vmess link: base64 JSON.
func parseVMess(payload string) (map[string]any, string, error) {
	if i := strings.IndexByte(payload, '#'); i >= 0 {
		payload = payload[:i]
	}
	raw, err := b64(payload)
	if err != nil {
		return nil, "", fmt.Errorf("vmess: %w", err)
	}
	var v map[string]any
	if err := json.Unmarshal(raw, &v); err != nil {
		return nil, "", fmt.Errorf("vmess: %w", err)
	}
	s := func(k string) string {
		switch x := v[k].(type) {
		case string:
			return x
		case float64:
			return strconv.Itoa(int(x))
		}
		return ""
	}
	host := s("add")
	if host == "" {
		return nil, "", errors.New("vmess: no address")
	}
	ob := map[string]any{"type": "vmess", "server": host, "server_port": atoiDef(s("port"), 443),
		"uuid": s("id"), "security": firstNonEmpty(s("scy"), "auto"), "alter_id": atoiDef(s("aid"), 0),
		"packet_encoding": "xudp"}
	if err := v2Transport(ob, s("net"), s("type"), s("path"), s("host"), s("path"), s("mode")); err != nil {
		return nil, s("ps"), err
	}
	v2TLS(ob, s("tls"), s("sni"), s("fp"), s("alpn"), truthy(s("allowInsecure")), "", "", firstNonEmpty(s("host"), host))
	return ob, s("ps"), nil
}

// ss://base64(method:password)@host:port#name  or  ss://base64(method:password@host:port)#name
func parseSS(link string) (map[string]any, string, error) {
	rest := strings.TrimPrefix(link, "ss://")
	name := ""
	if i := strings.IndexByte(rest, '#'); i >= 0 {
		name, _ = url.PathUnescape(rest[i+1:])
		rest = rest[:i]
	}
	query := ""
	if i := strings.IndexByte(rest, '?'); i >= 0 {
		query, rest = rest[i+1:], rest[:i]
	}
	rest = strings.TrimSuffix(rest, "/")
	var userinfo, hostport string
	if i := strings.LastIndexByte(rest, '@'); i >= 0 {
		userinfo, hostport = rest[:i], rest[i+1:]
		if d, err := b64(userinfo); err == nil && strings.Contains(string(d), ":") {
			userinfo = string(d)
		} else if u, err := url.PathUnescape(userinfo); err == nil {
			userinfo = u
		}
	} else {
		d, err := b64(rest)
		if err != nil {
			return nil, name, fmt.Errorf("ss: %w", err)
		}
		j := strings.LastIndexByte(string(d), '@')
		if j < 0 {
			return nil, name, errors.New("ss: bad link")
		}
		userinfo, hostport = string(d[:j]), string(d[j+1:])
	}
	method, password, ok := strings.Cut(userinfo, ":")
	if !ok {
		return nil, name, errors.New("ss: no password")
	}
	host, p, err := net.SplitHostPort(hostport)
	if err != nil {
		return nil, name, err
	}
	ob := map[string]any{"type": "shadowsocks", "server": host, "server_port": atoiDef(p, 8388),
		"method": method, "password": password}
	if query != "" {
		q, _ := url.ParseQuery(query)
		if pl := q.Get("plugin"); pl != "" {
			parts := strings.SplitN(pl, ";", 2)
			name := parts[0]
			if name == "simple-obfs" {
				name = "obfs-local"
			}
			ob["plugin"] = name
			if len(parts) > 1 {
				ob["plugin_opts"] = parts[1]
			}
		}
	}
	return ob, name, nil
}

// v2Transport maps v2ray transport settings to sing-box.
func v2Transport(ob map[string]any, network, headerType, path, host, service, mode string) error {
	switch strings.ToLower(network) {
	case "", "tcp", "raw":
		if headerType == "http" {
			t := map[string]any{"type": "http", "method": "GET"}
			if path != "" {
				t["path"] = path
			}
			if host != "" {
				t["host"] = strings.Split(host, ",")
			}
			ob["transport"] = t
		}
	case "ws", "websocket":
		t := map[string]any{"type": "ws", "path": firstNonEmpty(path, "/")}
		if u, err := url.Parse(t["path"].(string)); err == nil {
			if ed := u.Query().Get("ed"); ed != "" {
				t["max_early_data"] = atoiDef(ed, 0)
				t["early_data_header_name"] = "Sec-WebSocket-Protocol"
				q := u.Query()
				q.Del("ed")
				u.RawQuery = q.Encode()
				t["path"] = u.String()
			}
		}
		if host != "" {
			t["headers"] = map[string]any{"Host": host}
		}
		ob["transport"] = t
	case "grpc":
		ob["transport"] = map[string]any{"type": "grpc", "service_name": firstNonEmpty(service, path)}
	case "httpupgrade":
		t := map[string]any{"type": "httpupgrade", "path": firstNonEmpty(path, "/")}
		if host != "" {
			t["host"] = host
		}
		ob["transport"] = t
	case "h2", "http":
		t := map[string]any{"type": "http", "path": firstNonEmpty(path, "/")}
		if host != "" {
			t["host"] = strings.Split(host, ",")
		}
		ob["transport"] = t
	case "quic":
		ob["transport"] = map[string]any{"type": "quic"}
	case "xhttp", "splithttp":
		return trErr("XHTTP is only available in the Xray core and is not supported yet")
	default:
		return trErr("transport %q is not supported", network)
	}
	return nil
}

func v2TLS(ob map[string]any, security, sni, fp, alpn string, insecure bool, pbk, sid, fallbackSNI string) {
	switch strings.ToLower(security) {
	case "tls", "reality", "xtls":
	default:
		return
	}
	t := map[string]any{"enabled": true, "server_name": firstNonEmpty(sni, fallbackSNI)}
	if insecure {
		t["insecure"] = true
	}
	if alpn != "" {
		t["alpn"] = strings.Split(alpn, ",")
	}
	if fp == "" && strings.EqualFold(security, "reality") {
		fp = "chrome"
	}
	if fp != "" && fp != "none" {
		if fp == "random" || fp == "randomized" {
			fp = "randomized"
		}
		t["utls"] = map[string]any{"enabled": true, "fingerprint": fp}
	}
	if strings.EqualFold(security, "reality") {
		t["reality"] = map[string]any{"enabled": true, "public_key": pbk, "short_id": sid}
	}
	ob["tls"] = t
}

func quicTLS(sni string, insecure bool, alpn, defALPN string) map[string]any {
	t := map[string]any{"enabled": true, "server_name": sni}
	if insecure {
		t["insecure"] = true
	}
	if alpn == "" {
		alpn = defALPN
	}
	t["alpn"] = strings.Split(alpn, ",")
	return t
}

func b64(s string) ([]byte, error) {
	s = strings.TrimSpace(s)
	s = strings.TrimRight(s, "=")
	for _, enc := range []*base64.Encoding{base64.RawStdEncoding, base64.RawURLEncoding} {
		if d, err := enc.DecodeString(s); err == nil {
			return d, nil
		}
	}
	return nil, errors.New("invalid base64")
}

func firstNonEmpty(v ...string) string {
	for _, s := range v {
		if s != "" {
			return s
		}
	}
	return ""
}

func truthy(v ...string) bool {
	for _, s := range v {
		if s == "1" || strings.EqualFold(s, "true") {
			return true
		}
	}
	return false
}

func atoiDef(s string, def int) int {
	if n, err := strconv.Atoi(strings.TrimSpace(s)); err == nil {
		return n
	}
	return def
}

// splitLinks extracts links from a subscription body or pasted text:
// plain lines, base64 of lines, or a sing-box JSON with "outbounds".
func splitLinks(body string) []string {
	body = strings.TrimSpace(strings.TrimPrefix(body, "\ufeff"))
	if strings.HasPrefix(body, "{") || strings.HasPrefix(body, "[") {
		var cfg struct {
			Outbounds []json.RawMessage `json:"outbounds"`
			Endpoints []json.RawMessage `json:"endpoints"`
		}
		var list []json.RawMessage
		if json.Unmarshal([]byte(body), &cfg) == nil && len(cfg.Outbounds)+len(cfg.Endpoints) > 0 {
			list = append(cfg.Outbounds, cfg.Endpoints...)
		} else if json.Unmarshal([]byte(body), &list) != nil {
			list = []json.RawMessage{json.RawMessage(body)}
		}
		var out []string
		for _, o := range list {
			var m map[string]any
			if json.Unmarshal(o, &m) != nil {
				continue
			}
			switch m["type"] {
			case "direct", "block", "dns", "selector", "urltest", nil:
				continue
			}
			out = append(out, string(o))
		}
		return out
	}
	if strings.Contains(body, "proxies:") {
		if list := clashProxies(body); len(list) > 0 {
			return list
		}
	}
	if !strings.Contains(body, "://") {
		if d, err := b64(strings.Join(strings.Fields(body), "")); err == nil {
			body = string(d)
			if strings.Contains(body, "proxies:") {
				if list := clashProxies(body); len(list) > 0 {
					return list
				}
			}
		}
	}
	var out []string
	for _, l := range strings.Split(body, "\n") {
		l = strings.TrimSpace(l)
		if strings.Contains(l, "://") && !strings.HasPrefix(l, "#") {
			out = append(out, l)
		}
	}
	return out
}
