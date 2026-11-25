//go:build windows || linux

package main

import (
	"io"
	"net"
	"net/netip"
	"time"
)

// dnsRelay answers DNS for hotspot devices on the hotspot address by passing
// each query to a public resolver. That address is routed into the tunnel, so
// the tunnel answers it (FakeIP / through the proxy) exactly as for this
// computer. (Windows' own DNS proxy on the shared adapter asks the real
// network's resolvers, which the leak guard rightly blocks.)
type dnsRelay struct {
	udp    *net.UDPConn
	tcp    net.Listener
	onFail func(error)
}

func startDNSRelay(listen netip.Addr, upstream string, onQuery func(netip.Addr)) (*dnsRelay, error) {
	u, err := net.ListenUDP("udp4", &net.UDPAddr{IP: listen.AsSlice(), Port: 53})
	if err != nil {
		return nil, err
	}
	r := &dnsRelay{udp: u}
	if l, err := net.Listen("tcp4", net.JoinHostPort(listen.String(), "53")); err == nil {
		r.tcp = l
		go r.serveTCP(upstream)
	}
	go r.serveUDP(upstream, onQuery)
	return r, nil
}

func (r *dnsRelay) serveUDP(upstream string, onQuery func(netip.Addr)) {
	defer recoverPanic("dns relay")
	buf := make([]byte, 65535)
	for {
		n, from, err := r.udp.ReadFromUDP(buf)
		if err != nil {
			return
		}
		q := append([]byte(nil), buf[:n]...)
		if onQuery != nil {
			if a, ok := netip.AddrFromSlice(from.IP.To4()); ok {
				onQuery(a)
			}
		}
		go func() {
			defer recoverPanic("dns relay")
			c, err := net.Dial("udp4", upstream)
			if err != nil {
				return
			}
			defer c.Close()
			c.SetDeadline(time.Now().Add(6 * time.Second))
			if _, err := c.Write(q); err != nil {
				return
			}
			resp := make([]byte, 65535)
			m, err := c.Read(resp)
			if err == nil {
				r.udp.WriteToUDP(resp[:m], from)
			} else if r.onFail != nil {
				r.onFail(err)
			}
		}()
	}
}

func (r *dnsRelay) serveTCP(upstream string) {
	defer recoverPanic("dns relay")
	for {
		c, err := r.tcp.Accept()
		if err != nil {
			return
		}
		go func() {
			defer recoverPanic("dns relay")
			defer c.Close()
			up, err := net.DialTimeout("tcp4", upstream, 6*time.Second)
			if err != nil {
				return
			}
			defer up.Close()
			c.SetDeadline(time.Now().Add(30 * time.Second))
			up.SetDeadline(time.Now().Add(30 * time.Second))
			go io.Copy(up, c)
			io.Copy(c, up)
		}()
	}
}

func (r *dnsRelay) Close() {
	r.udp.Close()
	if r.tcp != nil {
		r.tcp.Close()
	}
}
