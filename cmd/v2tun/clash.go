package main

import (
	"encoding/json"
	"fmt"
	"strings"

	"gopkg.in/yaml.v3"
)

// clashProxies converts the "proxies:" of a Clash / Clash.Meta (mihomo)
// subscription into sing-box outbound JSON entries.
func clashProxies(body string) []string {
	var cfg struct {
		Proxies []map[string]any `yaml:"proxies"`
	}
	if yaml.Unmarshal([]byte(body), &cfg) != nil {
		return nil
	}
	var out []string
	for _, p := range cfg.Proxies {
		ob, err := clashToOutbound(p)
		if err != nil {
			continue
		}
		ob["tag"] = str(p["name"])
		if b, err := json.Marshal(ob); err == nil {
			out = append(out, string(b))
		}
	}
	return out
}

func str(v any) string {
	switch x := v.(type) {
	case nil:
		return ""
	case string:
		return x
	default:
		return fmt.Sprint(x)
	}
}

func num(v any) int { return atoiDef(str(v), 0) }

func boolean(v any) bool {
	b, ok := v.(bool)
	return ok && b || truthy(str(v))
}

func sub(v any) map[string]any {
	m, _ := v.(map[string]any)
	if m == nil {
		m = map[string]any{}
	}
	return m
}

func strList(v any) []string {
	switch x := v.(type) {
	case []any:
		var out []string
		for _, e := range x {
			out = append(out, str(e))
		}
		return out
	case string:
		if x == "" {
			return nil
		}
		return strings.Split(x, ",")
	}
	return nil
}

func clashTLS(p map[string]any, force bool) map[string]any {
	if !force && !boolean(p["tls"]) && p["reality-opts"] == nil {
		return nil
	}
	t := map[string]any{"enabled": true, "server_name": firstNonEmpty(str(p["servername"]), str(p["sni"]), str(p["server"]))}
	if boolean(p["skip-cert-verify"]) {
		t["insecure"] = true
	}
	if a := strList(p["alpn"]); len(a) > 0 {
		t["alpn"] = a
	}
	fp := str(p["client-fingerprint"])
	if r := p["reality-opts"]; r != nil {
		rm := sub(r)
		t["reality"] = map[string]any{"enabled": true, "public_key": str(rm["public-key"]), "short_id": str(rm["short-id"])}
		fp = firstNonEmpty(fp, "chrome")
	}
	if fp != "" {
		t["utls"] = map[string]any{"enabled": true, "fingerprint": fp}
	}
	return t
}

func clashTransport(p map[string]any) (map[string]any, error) {
	switch str(p["network"]) {
	case "", "tcp":
		return nil, nil
	case "ws":
		o := sub(p["ws-opts"])
		t := map[string]any{"type": "ws", "path": firstNonEmpty(str(o["path"]), "/")}
		if h := sub(o["headers"]); len(h) > 0 {
			t["headers"] = h
		}
		if n := num(o["max-early-data"]); n > 0 {
			t["max_early_data"] = n
			t["early_data_header_name"] = firstNonEmpty(str(o["early-data-header-name"]), "Sec-WebSocket-Protocol")
		}
		return t, nil
	case "grpc":
		return map[string]any{"type": "grpc", "service_name": str(sub(p["grpc-opts"])["grpc-service-name"])}, nil
	case "h2":
		o := sub(p["h2-opts"])
		return map[string]any{"type": "http", "host": strList(o["host"]), "path": firstNonEmpty(str(o["path"]), "/")}, nil
	case "http":
		o := sub(p["http-opts"])
		t := map[string]any{"type": "http", "method": firstNonEmpty(str(o["method"]), "GET")}
		if ps := strList(o["path"]); len(ps) > 0 {
			t["path"] = ps[0]
		}
		return t, nil
	case "httpupgrade":
		o := sub(p["http-upgrade-opts"])
		return map[string]any{"type": "httpupgrade", "path": firstNonEmpty(str(o["path"]), "/"), "host": str(o["host"])}, nil
	}
	return nil, fmt.Errorf("network %v", p["network"])
}

func clashToOutbound(p map[string]any) (map[string]any, error) {
	server, port := str(p["server"]), num(p["port"])
	ob := map[string]any{"server": server, "server_port": port}
	setTransport := func() error {
		t, err := clashTransport(p)
		if t != nil {
			ob["transport"] = t
		}
		return err
	}
	switch str(p["type"]) {
	case "ss":
		ob["type"], ob["method"], ob["password"] = "shadowsocks", str(p["cipher"]), str(p["password"])
		if pl := str(p["plugin"]); pl != "" {
			o := sub(p["plugin-opts"])
			switch pl {
			case "obfs":
				ob["plugin"] = "obfs-local"
				ob["plugin_opts"] = fmt.Sprintf("obfs=%s;obfs-host=%s", str(o["mode"]), str(o["host"]))
			case "v2ray-plugin":
				ob["plugin"] = "v2ray-plugin"
				opts := "mode=" + firstNonEmpty(str(o["mode"]), "websocket")
				if boolean(o["tls"]) {
					opts += ";tls"
				}
				if h := str(o["host"]); h != "" {
					opts += ";host=" + h
				}
				if pa := str(o["path"]); pa != "" {
					opts += ";path=" + pa
				}
				ob["plugin_opts"] = opts
			default:
				return nil, fmt.Errorf("ss plugin %s", pl)
			}
		}
	case "vmess":
		ob["type"], ob["uuid"], ob["alter_id"] = "vmess", str(p["uuid"]), num(p["alterId"])
		ob["security"] = firstNonEmpty(str(p["cipher"]), "auto")
		ob["packet_encoding"] = "xudp"
		if err := setTransport(); err != nil {
			return nil, err
		}
		if t := clashTLS(p, false); t != nil {
			ob["tls"] = t
		}
	case "vless":
		ob["type"], ob["uuid"] = "vless", str(p["uuid"])
		if f := str(p["flow"]); f != "" {
			ob["flow"] = f
		}
		ob["packet_encoding"] = "xudp"
		if err := setTransport(); err != nil {
			return nil, err
		}
		if t := clashTLS(p, false); t != nil {
			ob["tls"] = t
		}
	case "trojan":
		ob["type"], ob["password"] = "trojan", str(p["password"])
		if err := setTransport(); err != nil {
			return nil, err
		}
		ob["tls"] = clashTLS(p, true)
	case "hysteria2":
		ob["type"], ob["password"] = "hysteria2", firstNonEmpty(str(p["password"]), str(p["auth"]))
		if ports := str(p["ports"]); ports != "" {
			var list []string
			for _, x := range strings.Split(ports, ",") {
				list = append(list, strings.ReplaceAll(strings.TrimSpace(x), "-", ":"))
			}
			ob["server_ports"] = list
		}
		if o := str(p["obfs"]); o != "" {
			ob["obfs"] = map[string]any{"type": o, "password": str(p["obfs-password"])}
		}
		t := clashTLS(p, true)
		if t["alpn"] == nil {
			t["alpn"] = []string{"h3"}
		}
		ob["tls"] = t
	case "hysteria":
		ob["type"] = "hysteria"
		ob["auth_str"] = firstNonEmpty(str(p["auth-str"]), str(p["auth_str"]))
		ob["up_mbps"] = atoiDef(strings.Fields(str(p["up"]) + " 20")[0], 20)
		ob["down_mbps"] = atoiDef(strings.Fields(str(p["down"]) + " 100")[0], 100)
		if o := str(p["obfs"]); o != "" {
			ob["obfs"] = o
		}
		ob["tls"] = clashTLS(p, true)
	case "tuic":
		ob["type"], ob["uuid"], ob["password"] = "tuic", str(p["uuid"]), str(p["password"])
		ob["congestion_control"] = firstNonEmpty(str(p["congestion-controller"]), "bbr")
		if m := str(p["udp-relay-mode"]); m != "" {
			ob["udp_relay_mode"] = m
		}
		t := clashTLS(p, true)
		if t["alpn"] == nil {
			t["alpn"] = []string{"h3"}
		}
		ob["tls"] = t
	case "wireguard":
		addrs := []string{}
		if ip := str(p["ip"]); ip != "" {
			if !strings.Contains(ip, "/") {
				ip += "/32"
			}
			addrs = append(addrs, ip)
		}
		if ip := str(p["ipv6"]); ip != "" {
			if !strings.Contains(ip, "/") {
				ip += "/128"
			}
			addrs = append(addrs, ip)
		}
		peer := map[string]any{"address": server, "port": port, "public_key": str(p["public-key"]),
			"allowed_ips": []string{"0.0.0.0/0", "::/0"}}
		if k := str(p["pre-shared-key"]); k != "" {
			peer["pre_shared_key"] = k
		}
		if r, ok := p["reserved"].([]any); ok {
			var res []int
			for _, x := range r {
				res = append(res, num(x))
			}
			peer["reserved"] = res
		}
		w := map[string]any{"type": "wireguard", "private_key": str(p["private-key"]), "address": addrs, "peers": []any{peer}}
		if m := num(p["mtu"]); m > 0 {
			w["mtu"] = m
		}
		return w, nil
	case "socks5":
		ob["type"] = "socks"
		if u := str(p["username"]); u != "" {
			ob["username"], ob["password"] = u, str(p["password"])
		}
	case "http":
		ob["type"] = "http"
		if u := str(p["username"]); u != "" {
			ob["username"], ob["password"] = u, str(p["password"])
		}
		if t := clashTLS(p, false); t != nil {
			ob["tls"] = t
		}
	case "ssh":
		ob["type"], ob["user"] = "ssh", firstNonEmpty(str(p["username"]), "root")
		if pw := str(p["password"]); pw != "" {
			ob["password"] = pw
		}
		if k := str(p["private-key"]); k != "" {
			ob["private_key"] = k
		}
	case "anytls":
		ob["type"], ob["password"] = "anytls", str(p["password"])
		ob["tls"] = clashTLS(p, true)
	default:
		return nil, fmt.Errorf("clash type %v", p["type"])
	}
	return ob, nil
}
