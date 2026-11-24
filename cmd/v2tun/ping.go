//go:build windows

package main

import (
	"context"
	"encoding/binary"
	"errors"
	"net"
	"net/netip"
	"strconv"
	"strings"
	"sync"
	"time"
	"unsafe"

	"golang.org/x/sys/windows"
)

var (
	procIcmpCreate      = iphlpapi.NewProc("IcmpCreateFile")
	procIcmpClose       = iphlpapi.NewProc("IcmpCloseHandle")
	procIcmpSendEcho2Ex = iphlpapi.NewProc("IcmpSendEcho2Ex")
)

type icmpEchoReply struct {
	Address       uint32
	Status        uint32
	RoundTripTime uint32
	DataSize      uint16
	Reserved      uint16
	Data          uintptr
	Options       [8]byte
	_             [8]byte
}

// udpProtocols have no TCP handshake to time; they are pinged with ICMP.
var udpProtocols = map[string]bool{"hysteria": true, "hysteria2": true, "tuic": true, "wireguard": true}

// icmpPing sends one echo from the real interface's address. Windows uses the
// strong host model, so the packet leaves through that interface, not the tunnel.
func icmpPing(src, dst netip.Addr, timeout time.Duration) (int, error) {
	if !src.Is4() || !dst.Is4() {
		return 0, errors.New("icmp: IPv4 only")
	}
	h, _, err := procIcmpCreate.Call()
	if h == uintptr(windows.InvalidHandle) {
		return 0, err
	}
	defer procIcmpClose.Call(h)
	s, d := src.As4(), dst.As4()
	data := []byte("v2tun-ping-v2tun-ping-v2tun-ping")
	reply := make([]byte, int(unsafe.Sizeof(icmpEchoReply{}))+len(data)+64)
	n, _, _ := procIcmpSendEcho2Ex.Call(h, 0, 0, 0,
		uintptr(binary.LittleEndian.Uint32(s[:])), uintptr(binary.LittleEndian.Uint32(d[:])),
		uintptr(unsafe.Pointer(&data[0])), uintptr(len(data)), 0,
		uintptr(unsafe.Pointer(&reply[0])), uintptr(len(reply)), uintptr(timeout.Milliseconds()))
	r := (*icmpEchoReply)(unsafe.Pointer(&reply[0]))
	if n == 0 || r.Status != 0 {
		return 0, errors.New("no reply")
	}
	return max(int(r.RoundTripTime), 1), nil
}

// tcpPing times the TCP handshake to the server over the real interface.
func tcpPing(up uplink, ip netip.Addr, port int, timeout time.Duration) (int, error) {
	d := directDialer(up)
	d.Timeout = timeout
	start := time.Now()
	c, err := d.DialContext(context.Background(), "tcp4", net.JoinHostPort(ip.String(), strconv.Itoa(port)))
	if err != nil {
		return 0, err
	}
	ms := int(time.Since(start).Milliseconds())
	c.Close()
	return max(ms, 1), nil
}

func splitAddr(addr string) (string, int) {
	addr = strings.TrimPrefix(addr, "https://")
	if i := strings.IndexByte(addr, '/'); i >= 0 {
		addr = addr[:i]
	}
	if h, p, err := net.SplitHostPort(addr); err == nil {
		return strings.Trim(h, "[]"), atoiDef(p, 443)
	}
	return addr, 443
}

// pingServers measures every server's ping (ms; -1 = no answer).
func pingServers(up uplink, servers []*Server) map[string]int {
	out := map[string]int{}
	var mu sync.Mutex
	var wg sync.WaitGroup
	sem := make(chan struct{}, 16)
	dns := map[string][]netip.Addr{}
	var dnsMu sync.Mutex
	resolve := func(h string) []netip.Addr {
		dnsMu.Lock()
		defer dnsMu.Unlock()
		if ips, ok := dns[h]; ok {
			return ips
		}
		ips, _ := resolveDirect(up, h)
		dns[h] = ips
		return ips
	}
	for _, s := range servers {
		if s.Addr == "" {
			continue
		}
		wg.Add(1)
		go func(s *Server) {
			defer wg.Done()
			sem <- struct{}{}
			defer func() { <-sem }()
			host, port := splitAddr(s.Addr)
			ms := -1
			if ips := resolve(host); len(ips) > 0 {
				var err error
				if udpProtocols[s.Type] {
					ms, err = icmpPing(up.Addr, ips[0], 3*time.Second)
				} else {
					ms, err = tcpPing(up, ips[0], port, 3*time.Second)
				}
				if err != nil {
					ms = -1
				}
			}
			mu.Lock()
			out[s.ID] = ms
			mu.Unlock()
		}(s)
	}
	wg.Wait()
	return out
}
