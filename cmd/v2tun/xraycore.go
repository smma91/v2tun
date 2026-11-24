//go:build windows || linux

package main

import (
	"bytes"
	"encoding/json"
	"fmt"
	"net/url"
	"strconv"
	"strings"
	"sync"

	xcore "github.com/xtls/xray-core/core"
	_ "github.com/xtls/xray-core/main/distro/all"
)

// Xray-core is embedded only for what sing-box cannot do (VLESS XHTTP /
// SplitHTTP). Each such server runs in its own small Xray instance with a
// local SOCKS port; sing-box reaches it through a "socks" outbound, so
// selection, auto-select, tests and failover work the same for all servers.
type xrayPool struct {
	mu    sync.Mutex
	insts map[string]*xrayInst // server id -> instance
}

type xrayInst struct {
	inst *xcore.Instance
	port int
	key  string // link + interface it was started with
}

var xrays = &xrayPool{insts: map[string]*xrayInst{}}

func needsXray(link string) bool {
	if !strings.HasPrefix(strings.ToLower(link), "vless://") {
		return false
	}
	u, err := url.Parse(link)
	if err != nil {
		return false
	}
	t := strings.ToLower(u.Query().Get("type"))
	return t == "xhttp" || t == "splithttp"
}

// xrayOutbound converts a VLESS XHTTP link to an Xray outbound. The server
// name is resolved here, on the real network: inside the tunnel names resolve
// to FakeIP addresses, which only mean something to the tunnel itself.
func xrayOutbound(link string, up uplink) (map[string]any, error) {
	u, err := url.Parse(link)
	if err != nil {
		return nil, err
	}
	q := u.Query()
	port, _ := strconv.Atoi(u.Port())
	if port == 0 {
		port = 443
	}
	user := map[string]any{"id": u.User.Username(), "encryption": firstNonEmpty(q.Get("encryption"), "none")}
	if f := q.Get("flow"); f != "" {
		user["flow"] = f
	}
	xh := map[string]any{"path": firstNonEmpty(q.Get("path"), "/")}
	if h := q.Get("host"); h != "" {
		xh["host"] = h
	}
	if m := q.Get("mode"); m != "" {
		xh["mode"] = m
	}
	if extra := q.Get("extra"); extra != "" {
		var e any
		if json.Unmarshal([]byte(extra), &e) == nil {
			xh["extra"] = e
		}
	}
	sockopt := map[string]any{"interface": up.Name}
	if directMark != 0 {
		sockopt["mark"] = directMark
	}
	addr := u.Hostname()
	if ips, err := resolveDirect(up, addr); err == nil && len(ips) > 0 && ips[0].String() != addr {
		addr = ips[0].String()
		if xh["host"] == nil {
			xh["host"] = u.Hostname() // keep the HTTP Host header
		}
	}
	stream := map[string]any{"network": "xhttp", "xhttpSettings": xh, "sockopt": sockopt}
	sec := strings.ToLower(q.Get("security"))
	switch sec {
	case "tls":
		t := map[string]any{"serverName": firstNonEmpty(q.Get("sni"), q.Get("host"), u.Hostname()),
			"fingerprint": firstNonEmpty(q.Get("fp"), "chrome"), "allowInsecure": truthy(q.Get("allowInsecure"), q.Get("insecure"))}
		if a := q.Get("alpn"); a != "" {
			t["alpn"] = strings.Split(a, ",")
		}
		stream["security"], stream["tlsSettings"] = "tls", t
	case "reality":
		stream["security"] = "reality"
		stream["realitySettings"] = map[string]any{"serverName": q.Get("sni"), "fingerprint": firstNonEmpty(q.Get("fp"), "chrome"),
			"publicKey": q.Get("pbk"), "shortId": q.Get("sid"), "spiderX": q.Get("spx")}
	}
	return map[string]any{
		"protocol":       "vless",
		"settings":       map[string]any{"vnext": []any{map[string]any{"address": addr, "port": port, "users": []any{user}}}},
		"streamSettings": stream,
	}, nil
}

// ensure starts (or reuses) the Xray instance for a server; returns its port.
func (p *xrayPool) ensure(id, link string, up uplink) (int, error) {
	p.mu.Lock()
	defer p.mu.Unlock()
	key := link + "|" + up.Name
	if x := p.insts[id]; x != nil {
		if x.key == key {
			return x.port, nil
		}
		x.inst.Close()
		delete(p.insts, id)
	}
	ob, err := xrayOutbound(link, up)
	if err != nil {
		return 0, err
	}
	port, err := freePortTCPUDP()
	if err != nil {
		return 0, err
	}
	cfg := map[string]any{
		"log": map[string]any{"loglevel": "warning"},
		"dns": map[string]any{"servers": []any{"1.1.1.1"}},
		"inbounds": []any{map[string]any{"listen": "127.0.0.1", "port": port, "protocol": "socks",
			"settings": map[string]any{"udp": true}}},
		"outbounds": []any{ob},
	}
	b, _ := json.Marshal(cfg)
	c, err := xcore.LoadConfig("json", bytes.NewReader(b))
	if err != nil {
		return 0, fmt.Errorf("xray config: %w", err)
	}
	inst, err := xcore.New(c)
	if err != nil {
		return 0, fmt.Errorf("xray: %w", err)
	}
	if err := inst.Start(); err != nil {
		inst.Close()
		return 0, fmt.Errorf("xray start: %w", err)
	}
	p.insts[id] = &xrayInst{inst: inst, port: port, key: key}
	return port, nil
}

// keep stops instances of servers that are no longer used.
func (p *xrayPool) keep(ids map[string]bool) {
	p.mu.Lock()
	defer p.mu.Unlock()
	for id, x := range p.insts {
		if !ids[id] {
			x.inst.Close()
			delete(p.insts, id)
		}
	}
}

func (p *xrayPool) stopAll() { p.keep(nil) }
