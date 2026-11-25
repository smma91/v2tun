package profile

import (
	"encoding/json"
	"fmt"
	"strings"

	"gopkg.in/yaml.v3"
)

// ClashProxies converts the "proxies:" of a Clash / Clash.Meta (mihomo)
// subscription into sing-box outbound JSON entries.
func ClashProxies(body string) []string {
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
		ob["tag"] = Str(p["name"])
		if b, err := json.Marshal(ob); err == nil {
			out = append(out, string(b))
		}
	}
	return out
}

// Str turns a YAML / JSON scalar into a string.
func Str(v any) string {
	switch x := v.(type) {
	case nil:
		return ""
	case string:
		return x
	default:
		return fmt.Sprint(x)
	}
}

func num(v any) int { return AtoiDef(Str(v), 0) }

func boolean(v any) bool {
	b, ok := v.(bool)
	return ok && b || Truthy(Str(v))
}

// Sub returns a nested YAML / JSON object (or nil).
func Sub(v any) map[string]any {
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
			out = append(out, Str(e))
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
	t := map[string]any{"enabled": true, "server_name": FirstNonEmpty(Str(p["servername"]), Str(p["sni"]), Str(p["server"]))}
	if boolean(p["skip-cert-verify"]) {
		t["insecure"] = true
	}
	if a := strList(p["alpn"]); len(a) > 0 {
		t["alpn"] = a
	}
	fp := Str(p["client-fingerprint"])
	if r := p["reality-opts"]; r != nil {
		rm := Sub(r)
		t["reality"] = map[string]any{"enabled": true, "public_key": Str(rm["public-key"]), "short_id": Str(rm["short-id"])}
		fp = FirstNonEmpty(fp, "chrome")
	}
	if fp != "" {
		t["utls"] = map[string]any{"enabled": true, "fingerprint": fp}
	}
	return t
}

func clashTransport(p map[string]any) (map[string]any, error) {
	switch Str(p["network"]) {
	case "", "tcp":
		return nil, nil
	case "ws":
		o := Sub(p["ws-opts"])
		t := map[string]any{"type": "ws", "path": FirstNonEmpty(Str(o["path"]), "/")}
		if h := Sub(o["headers"]); len(h) > 0 {
			t["headers"] = h
		}
		if n := num(o["max-early-data"]); n > 0 {
			t["max_early_data"] = n
			t["early_data_header_name"] = FirstNonEmpty(Str(o["early-data-header-name"]), "Sec-WebSocket-Protocol")
		}
		return t, nil
	case "grpc":
		return map[string]any{"type": "grpc", "service_name": Str(Sub(p["grpc-opts"])["grpc-service-name"])}, nil
	case "h2":
		o := Sub(p["h2-opts"])
		return map[string]any{"type": "http", "host": strList(o["host"]), "path": FirstNonEmpty(Str(o["path"]), "/")}, nil
	case "http":
		o := Sub(p["http-opts"])
		t := map[string]any{"type": "http", "method": FirstNonEmpty(Str(o["method"]), "GET")}
		if ps := strList(o["path"]); len(ps) > 0 {
			t["path"] = ps[0]
		}
		return t, nil
	case "httpupgrade":
		o := Sub(p["http-upgrade-opts"])
		return map[string]any{"type": "httpupgrade", "path": FirstNonEmpty(Str(o["path"]), "/"), "host": Str(o["host"])}, nil
	}
	return nil, fmt.Errorf("network %v", p["network"])
}

func clashToOutbound(p map[string]any) (map[string]any, error) {
	server, port := Str(p["server"]), num(p["port"])
	ob := map[string]any{"server": server, "server_port": port}
	setTransport := func() error {
		t, err := clashTransport(p)
		if t != nil {
			ob["transport"] = t
		}
		return err
	}
	switch Str(p["type"]) {
	case "ss":
		ob["type"], ob["method"], ob["password"] = "shadowsocks", Str(p["cipher"]), Str(p["password"])
		if pl := Str(p["plugin"]); pl != "" {
			o := Sub(p["plugin-opts"])
			switch pl {
			case "obfs":
				ob["plugin"] = "obfs-local"
				ob["plugin_opts"] = fmt.Sprintf("obfs=%s;obfs-host=%s", Str(o["mode"]), Str(o["host"]))
			case "v2ray-plugin":
				ob["plugin"] = "v2ray-plugin"
				opts := "mode=" + FirstNonEmpty(Str(o["mode"]), "websocket")
				if boolean(o["tls"]) {
					opts += ";tls"
				}
				if h := Str(o["host"]); h != "" {
					opts += ";host=" + h
				}
				if pa := Str(o["path"]); pa != "" {
					opts += ";path=" + pa
				}
				ob["plugin_opts"] = opts
			default:
				return nil, fmt.Errorf("ss plugin %s", pl)
			}
		}
	case "vmess":
		ob["type"], ob["uuid"], ob["alter_id"] = "vmess", Str(p["uuid"]), num(p["alterId"])
		ob["security"] = FirstNonEmpty(Str(p["cipher"]), "auto")
		ob["packet_encoding"] = "xudp"
		if err := setTransport(); err != nil {
			return nil, err
		}
		if t := clashTLS(p, false); t != nil {
			ob["tls"] = t
		}
	case "vless":
		ob["type"], ob["uuid"] = "vless", Str(p["uuid"])
		if f := Str(p["flow"]); f != "" {
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
		ob["type"], ob["password"] = "trojan", Str(p["password"])
		if err := setTransport(); err != nil {
			return nil, err
		}
		ob["tls"] = clashTLS(p, true)
	case "hysteria2":
		ob["type"], ob["password"] = "hysteria2", FirstNonEmpty(Str(p["password"]), Str(p["auth"]))
		if ports := Str(p["ports"]); ports != "" {
			var list []string
			for _, x := range strings.Split(ports, ",") {
				list = append(list, strings.ReplaceAll(strings.TrimSpace(x), "-", ":"))
			}
			ob["server_ports"] = list
		}
		if o := Str(p["obfs"]); o != "" {
			ob["obfs"] = map[string]any{"type": o, "password": Str(p["obfs-password"])}
		}
		t := clashTLS(p, true)
		if t["alpn"] == nil {
			t["alpn"] = []string{"h3"}
		}
		ob["tls"] = t
	case "hysteria":
		ob["type"] = "hysteria"
		ob["auth_str"] = FirstNonEmpty(Str(p["auth-str"]), Str(p["auth_str"]))
		ob["up_mbps"] = AtoiDef(strings.Fields(Str(p["up"]) + " 20")[0], 20)
		ob["down_mbps"] = AtoiDef(strings.Fields(Str(p["down"]) + " 100")[0], 100)
		if o := Str(p["obfs"]); o != "" {
			ob["obfs"] = o
		}
		ob["tls"] = clashTLS(p, true)
	case "tuic":
		ob["type"], ob["uuid"], ob["password"] = "tuic", Str(p["uuid"]), Str(p["password"])
		ob["congestion_control"] = FirstNonEmpty(Str(p["congestion-controller"]), "bbr")
		if m := Str(p["udp-relay-mode"]); m != "" {
			ob["udp_relay_mode"] = m
		}
		t := clashTLS(p, true)
		if t["alpn"] == nil {
			t["alpn"] = []string{"h3"}
		}
		ob["tls"] = t
	case "wireguard":
		addrs := []string{}
		if ip := Str(p["ip"]); ip != "" {
			if !strings.Contains(ip, "/") {
				ip += "/32"
			}
			addrs = append(addrs, ip)
		}
		if ip := Str(p["ipv6"]); ip != "" {
			if !strings.Contains(ip, "/") {
				ip += "/128"
			}
			addrs = append(addrs, ip)
		}
		peer := map[string]any{"address": server, "port": port, "public_key": Str(p["public-key"]),
			"allowed_ips": []string{"0.0.0.0/0", "::/0"}}
		if k := Str(p["pre-shared-key"]); k != "" {
			peer["pre_shared_key"] = k
		}
		if r, ok := p["reserved"].([]any); ok {
			var res []int
			for _, x := range r {
				res = append(res, num(x))
			}
			peer["reserved"] = res
		}
		w := map[string]any{"type": "wireguard", "private_key": Str(p["private-key"]), "address": addrs, "peers": []any{peer}}
		if m := num(p["mtu"]); m > 0 {
			w["mtu"] = m
		}
		return w, nil
	case "socks5":
		ob["type"] = "socks"
		if u := Str(p["username"]); u != "" {
			ob["username"], ob["password"] = u, Str(p["password"])
		}
	case "http":
		ob["type"] = "http"
		if u := Str(p["username"]); u != "" {
			ob["username"], ob["password"] = u, Str(p["password"])
		}
		if t := clashTLS(p, false); t != nil {
			ob["tls"] = t
		}
	case "ssh":
		ob["type"], ob["user"] = "ssh", FirstNonEmpty(Str(p["username"]), "root")
		if pw := Str(p["password"]); pw != "" {
			ob["password"] = pw
		}
		if k := Str(p["private-key"]); k != "" {
			ob["private_key"] = k
		}
	case "anytls":
		ob["type"], ob["password"] = "anytls", Str(p["password"])
		ob["tls"] = clashTLS(p, true)
	default:
		return nil, fmt.Errorf("clash type %v", p["type"])
	}
	return ob, nil
}
