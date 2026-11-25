package profile

import (
	"bufio"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"
)

// ovpnToOutbound converts an OpenVPN client profile (.ovpn) into a sing-box
// "openvpn-client" endpoint. Inline blocks (<ca>, <cert>, <key>, <tls-auth>,
// <tls-crypt>, <tls-crypt-v2>, <secret>) and files next to the profile are
// supported. user/pass are used when the profile has auth-user-pass.
func ovpnToOutbound(text, dir, user, pass string) (map[string]any, bool, error) {
	inline := map[string][]string{}
	var lines [][]string
	sc := bufio.NewScanner(strings.NewReader(strings.ReplaceAll(text, "\r", "")))
	sc.Buffer(make([]byte, 1<<20), 1<<20)
	var block string
	for sc.Scan() {
		l := strings.TrimSpace(sc.Text())
		if block != "" {
			if l == "</"+block+">" {
				block = ""
				continue
			}
			inline[block] = append(inline[block], l)
			continue
		}
		if strings.HasPrefix(l, "<") && strings.HasSuffix(l, ">") && !strings.HasPrefix(l, "</") {
			block = strings.Trim(l, "<>")
			continue
		}
		if l == "" || l[0] == '#' || l[0] == ';' {
			continue
		}
		lines = append(lines, strings.Fields(l))
	}

	// material returns inline content, or the referenced file's lines.
	material := func(name string, args []string) []string {
		if len(args) > 0 && args[0] != "[inline]" {
			p := args[0]
			if !filepath.IsAbs(p) {
				p = filepath.Join(dir, p)
			}
			if b, err := os.ReadFile(p); err == nil {
				return strings.Split(strings.TrimSpace(strings.ReplaceAll(string(b), "\r", "")), "\n")
			}
		}
		return inline[name]
	}

	ob := map[string]any{"type": "openvpn-client"}
	tls := map[string]any{}
	var servers []any
	defPort, defProto := 1194, "udp"
	needAuth := false
	keyDir := ""
	var wrap map[string]any
	for _, f := range lines {
		args := f[1:]
		arg := func(i int) string {
			if i < len(args) {
				return args[i]
			}
			return ""
		}
		switch strings.ToLower(f[0]) {
		case "remote":
			r := map[string]any{"server": arg(0)}
			if p, err := strconv.Atoi(arg(1)); err == nil {
				r["server_port"] = p
			}
			if pr := arg(2); pr != "" {
				r["network"] = ovpnNet(pr)
			}
			servers = append(servers, r)
		case "port", "rport":
			defPort = AtoiDef(arg(0), defPort)
		case "proto":
			defProto = ovpnNet(arg(0))
		case "remote-random":
			ob["remote_random"] = true
		case "cipher":
			ob["cipher"] = arg(0)
		case "data-ciphers", "ncp-ciphers":
			ob["data_ciphers"] = strings.Split(arg(0), ":")
		case "data-ciphers-fallback":
			ob["data_ciphers_fallback"] = arg(0)
		case "auth":
			ob["auth"] = arg(0)
		case "auth-user-pass":
			needAuth = true
			if a := arg(0); a != "" {
				if b, err := os.ReadFile(filepath.Join(dir, a)); err == nil {
					cred := strings.Split(strings.ReplaceAll(string(b), "\r", ""), "\n")
					if len(cred) >= 2 && user == "" {
						user, pass = strings.TrimSpace(cred[0]), strings.TrimSpace(cred[1])
					}
				}
			}
		case "key-direction":
			keyDir = arg(0)
		case "ca":
			tls["certificate"] = material("ca", args)
		case "cert":
			tls["client_certificate"] = material("cert", args)
		case "key":
			tls["client_key"] = material("key", args)
		case "tls-auth":
			wrap = map[string]any{"type": "tls_auth", "key": material("tls-auth", args)}
			if d := arg(1); d != "" {
				keyDir = d
			}
		case "tls-crypt":
			wrap = map[string]any{"type": "tls_crypt", "key": material("tls-crypt", args)}
		case "tls-crypt-v2":
			wrap = map[string]any{"type": "tls_crypt_v2", "key": material("tls-crypt-v2", args)}
		case "secret":
			ob["mode"] = "static_key"
			ob["static_key"] = material("secret", args)
			if d := arg(1); d != "" {
				keyDir = d
			}
		case "verify-x509-name":
			tls["server_name"] = strings.Trim(arg(0), `"'`)
			if t := arg(1); t != "" {
				tls["server_name_type"] = t
			}
		case "remote-cert-tls":
			tls["remote_certificate_tls"] = arg(0)
		case "tls-version-min":
			tls["version_min"] = arg(0)
		case "tls-version-max":
			tls["version_max"] = arg(0)
		case "compress":
			ob["compression"] = FirstNonEmpty(arg(0), "stub")
		case "comp-lzo":
			ob["compression_lzo"] = FirstNonEmpty(arg(0), "adaptive")
		case "fragment":
			ob["fragment"] = AtoiDef(arg(0), 0)
		case "mssfix":
			if n := AtoiDef(arg(0), 0); n > 0 {
				ob["mss_fix"] = n
			}
		}
	}
	// Inline blocks are valid on their own, without a "ca" / "cert" ... line.
	for block, key := range map[string]string{"ca": "certificate", "cert": "client_certificate", "key": "client_key"} {
		if tls[key] == nil && len(inline[block]) > 0 {
			tls[key] = inline[block]
		}
	}
	if wrap == nil {
		for block, typ := range map[string]string{"tls-auth": "tls_auth", "tls-crypt": "tls_crypt", "tls-crypt-v2": "tls_crypt_v2"} {
			if len(inline[block]) > 0 {
				wrap = map[string]any{"type": typ, "key": inline[block]}
			}
		}
	}
	if ob["mode"] == nil && len(inline["secret"]) > 0 && tls["certificate"] == nil {
		ob["mode"], ob["static_key"] = "static_key", inline["secret"]
	}
	if len(servers) == 0 {
		return nil, needAuth, trErr("The .ovpn file has no remote")
	}
	for _, r := range servers {
		m := r.(map[string]any)
		if m["server_port"] == nil {
			m["server_port"] = defPort
		}
		if m["network"] == nil {
			m["network"] = defProto
		}
	}
	ob["servers"] = servers
	if wrap != nil {
		if d := map[string]string{"1": "client", "0": "server"}[keyDir]; d != "" && wrap["type"] == "tls_auth" {
			wrap["direction"] = d
		}
		tls["control_wrap"] = wrap
	}
	if keyDir != "" && ob["mode"] == "static_key" {
		ob["key_direction"] = map[string]string{"1": "client", "0": "server"}[keyDir]
	}
	if ob["mode"] != "static_key" {
		// OpenVPN 2.5+: in TLS mode "cipher" is the fallback for servers without
		// cipher negotiation and is accepted in addition to the defaults.
		if c, ok := ob["cipher"].(string); ok {
			delete(ob, "cipher")
			if ob["data_ciphers"] == nil {
				list := []string{"AES-256-GCM", "AES-128-GCM", "CHACHA20-POLY1305"}
				found := false
				for _, x := range list {
					found = found || strings.EqualFold(x, c)
				}
				if !found {
					list = append(list, c)
				}
				ob["data_ciphers"] = list
			}
			if ob["data_ciphers_fallback"] == nil {
				ob["data_ciphers_fallback"] = c
			}
		}
		if tls["certificate"] == nil {
			return nil, needAuth, trErr("The .ovpn file has no CA certificate (<ca>)")
		}
		ob["tls"] = tls
	}
	if needAuth {
		if user == "" {
			return nil, true, trErr("This profile needs a username and password")
		}
		ob["username"], ob["password"] = user, pass
	}
	return ob, needAuth, nil
}

func ovpnNet(p string) string {
	p = strings.ToLower(p)
	switch {
	case strings.HasPrefix(p, "tcp"):
		if strings.HasSuffix(p, "6") {
			return "tcp6"
		}
		return "tcp"
	case strings.HasSuffix(p, "6"):
		return "udp6"
	}
	return "udp"
}

// OvpnFileToLink reads a profile and returns the server entry (JSON) to store.
func OvpnFileToLink(path, user, pass string) (string, error) {
	b, err := os.ReadFile(path)
	if err != nil {
		return "", err
	}
	ob, _, err := ovpnToOutbound(string(b), filepath.Dir(path), user, pass)
	if err != nil {
		return "", err
	}
	ob["tag"] = strings.TrimSuffix(filepath.Base(path), filepath.Ext(path))
	j, err := json.Marshal(ob)
	if err != nil {
		return "", fmt.Errorf("ovpn: %w", err)
	}
	return string(j), nil
}
