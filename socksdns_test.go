//go:build windows

package main

import (
	"context"
	"encoding/binary"
	"net"
	"net/netip"
	"testing"
	"time"

	"golang.org/x/net/dns/dnsmessage"

	M "github.com/xjasonlyu/tun2socks/v2/metadata"
	"github.com/xjasonlyu/tun2socks/v2/proxy/socks5"
)

func dnsQuery(t *testing.T, name string) []byte {
	n, _ := dnsmessage.NewName(name)
	msg := dnsmessage.Message{Header: dnsmessage.Header{ID: 7, RecursionDesired: true},
		Questions: []dnsmessage.Question{{Name: n, Type: dnsmessage.TypeA, Class: dnsmessage.ClassINET}}}
	b, err := msg.Pack()
	if err != nil {
		t.Fatal(err)
	}
	return b
}

func TestSocksDNS(t *testing.T) {
	s, _ := socks5.New("127.0.0.1:10808", "", "")
	m := &M.Metadata{Network: M.UDP, DstIP: netip.MustParseAddr("8.8.8.8"), DstPort: 53}
	q := dnsQuery(t, "www.youtube.com.")

	// UDP via SOCKS
	pc, err := s.DialUDP(m)
	if err != nil {
		t.Logf("UDP associate: %v", err)
	} else {
		pc.WriteTo(q, m.UDPAddr())
		pc.SetReadDeadline(time.Now().Add(5 * time.Second))
		buf := make([]byte, 2048)
		n, _, err := pc.ReadFrom(buf)
		t.Logf("UDP via SOCKS: %d bytes, err=%v", n, err)
		pc.Close()
	}

	// TCP via SOCKS
	m.Network = M.TCP
	ctx, cancel := context.WithTimeout(context.Background(), 8*time.Second)
	defer cancel()
	c, err := s.DialContext(ctx, m)
	if err != nil {
		t.Fatalf("TCP via SOCKS: %v", err)
	}
	defer c.Close()
	c.SetDeadline(time.Now().Add(8 * time.Second))
	l := make([]byte, 2)
	binary.BigEndian.PutUint16(l, uint16(len(q)))
	c.Write(append(l, q...))
	if _, err := c.Read(l); err != nil {
		t.Fatalf("TCP DNS read: %v", err)
	}
	resp := make([]byte, binary.BigEndian.Uint16(l))
	_, err = ioReadFull(c, resp)
	d := newDNSMap()
	d.record(resp)
	t.Logf("TCP via SOCKS: %d bytes err=%v map=%d", len(resp), err, len(d.m))
}

func TestDNSTCPClient(t *testing.T) {
	s, _ := socks5.New("127.0.0.1:10808", "", "")
	c := newDNSTCP(&routerDialer{socks: s})
	defer c.close()
	srv := netip.MustParseAddr("8.8.8.8")
	for i, name := range []string{"www.youtube.com.", "ipinfo.io.", "www.youtube.com."} {
		start := time.Now()
		q := dnsQuery(t, name)
		q[0], q[1] = byte(i), 9
		resp, err := c.query(srv, q)
		if err != nil {
			t.Fatalf("%s: %v", name, err)
		}
		if resp[0] != byte(i) || resp[1] != 9 {
			t.Fatalf("id not rewritten")
		}
		d := newDNSMap()
		d.record(resp)
		t.Logf("%s -> %d addrs in %v", name, len(d.m), time.Since(start).Round(time.Millisecond))
	}
}

// Raw SOCKS5 UDP ASSOCIATE over IPv4, to tell a local problem from a server without UDP.
func TestSocksUDPRaw(t *testing.T) {
	c, err := net.Dial("tcp4", "127.0.0.1:10808")
	if err != nil {
		t.Fatal(err)
	}
	defer c.Close()
	c.SetDeadline(time.Now().Add(10 * time.Second))
	c.Write([]byte{5, 1, 0})
	b := make([]byte, 64)
	ioReadFull(c, b[:2])
	c.Write([]byte{5, 3, 0, 1, 0, 0, 0, 0, 0, 0})
	n, _ := c.Read(b)
	t.Logf("associate reply: % x", b[:n])
	bnd := netip.AddrPortFrom(netip.AddrFrom4([4]byte(b[4:8])), binary.BigEndian.Uint16(b[8:10]))
	if bnd.Addr().IsUnspecified() {
		bnd = netip.AddrPortFrom(netip.MustParseAddr("127.0.0.1"), bnd.Port())
	}
	u, err := net.ListenUDP("udp4", &net.UDPAddr{IP: net.IPv4(127, 0, 0, 1)})
	if err != nil {
		t.Fatal(err)
	}
	defer u.Close()
	for _, target := range []string{"8.8.8.8", "1.1.1.1"} {
		ip := netip.MustParseAddr(target).As4()
		pkt := append([]byte{0, 0, 0, 1}, ip[:]...)
		pkt = append(pkt, 0, 53)
		pkt = append(pkt, dnsQuery(t, "www.youtube.com.")...)
		u.WriteToUDPAddrPort(pkt, bnd)
		u.SetReadDeadline(time.Now().Add(4 * time.Second))
		buf := make([]byte, 2048)
		k, _, err := u.ReadFromUDPAddrPort(buf)
		t.Logf("UDP to %s via %s: %d bytes err=%v", target, bnd, k, err)
	}
}

func ioReadFull(c net.Conn, b []byte) (int, error) {
	n := 0
	for n < len(b) {
		k, err := c.Read(b[n:])
		n += k
		if err != nil {
			return n, err
		}
	}
	return n, nil
}
