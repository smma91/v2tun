package main

import (
	"crypto/rand"
	"encoding/binary"
	"errors"
	"net"
	"net/http"
	"net/netip"
	"net/url"
	"sync/atomic"
	"time"

	"v2tun/internal/netcheck"
)

type ExitInfo = netcheck.ExitInfo

// exitInfo asks which public IP the internet sees. A new connection is used
// every time: a kept-alive one could still belong to the network path from
// before the tunnel was connected and report the wrong (real) address.
// exitViaProxy is set in "proxy only" mode: there is no tunnel then, so the
// app asks for its exit IP through its own local proxy.
var exitViaProxy atomic.Value // string, "" = direct

func exitInfo() (ExitInfo, error) {
	tr := &http.Transport{DisableKeepAlives: true, Proxy: nil}
	if p, _ := exitViaProxy.Load().(string); p != "" {
		if u, err := url.Parse("http://" + p); err == nil {
			tr.Proxy = http.ProxyURL(u)
		}
	}
	return netcheck.Exit(&http.Client{Timeout: 8 * time.Second, Transport: tr})
}

type LeakResult struct {
	HTTPIP  string `json:"httpIp"`
	HTTPErr string `json:"httpErr"`
	STUNIP  string `json:"stunIp"`
	STUNErr string `json:"stunErr"`
	Pass    bool   `json:"pass"`
	Unknown bool   `json:"unknown"` // the web IP could not be read: no verdict
}

// leakCheck compares the IP websites see (TCP) with the one WebRTC would
// reveal (UDP/STUN), exactly like browser leak tests do.
func leakCheck() LeakResult {
	var r LeakResult
	if info, err := exitInfo(); err == nil {
		r.HTTPIP = info.IP
	} else {
		r.HTTPErr = err.Error()
	}
	var ip netip.Addr
	var err error
	for _, s := range []string{"stun.l.google.com:19302", "stun.cloudflare.com:3478"} {
		if ip, err = stunPublicIP(s); err == nil {
			break
		}
	}
	if err != nil {
		r.STUNErr = err.Error() // UDP does not get out: WebRTC cannot reveal anything
	} else {
		r.STUNIP = ip.String()
	}
	switch {
	case r.HTTPIP == "":
		r.Unknown = true
	case r.STUNIP == "":
		r.Pass = true
	default:
		r.Pass = r.STUNIP == r.HTTPIP
	}
	return r
}

const stunMagic = 0x2112A442

func stunPublicIP(server string) (netip.Addr, error) {
	conn, err := net.DialTimeout("udp", server, 5*time.Second)
	if err != nil {
		return netip.Addr{}, err
	}
	defer conn.Close()
	req := make([]byte, 20)
	binary.BigEndian.PutUint16(req[0:], 0x0001)
	binary.BigEndian.PutUint32(req[4:], stunMagic)
	rand.Read(req[8:20])
	buf := make([]byte, 1500)
	for try := 0; try < 3; try++ {
		if _, err := conn.Write(req); err != nil {
			return netip.Addr{}, err
		}
		conn.SetReadDeadline(time.Now().Add(2 * time.Second))
		n, err := conn.Read(buf)
		if err != nil {
			var ne net.Error
			if errors.As(err, &ne) && ne.Timeout() {
				continue
			}
			return netip.Addr{}, err
		}
		if n < 20 || string(buf[8:20]) != string(req[8:20]) {
			continue
		}
		b := buf[20:n]
		for len(b) >= 4 {
			typ := binary.BigEndian.Uint16(b[0:])
			l := int(binary.BigEndian.Uint16(b[2:]))
			if len(b) < 4+l {
				break
			}
			v := b[4 : 4+l]
			if typ == 0x0020 && l >= 8 && v[1] == 0x01 {
				ip := [4]byte(v[4:8])
				binary.BigEndian.PutUint32(ip[:], binary.BigEndian.Uint32(ip[:])^stunMagic)
				return netip.AddrFrom4(ip), nil
			}
			b = b[4+(l+3)&^3:]
		}
	}
	return netip.Addr{}, errors.New("no STUN response")
}
