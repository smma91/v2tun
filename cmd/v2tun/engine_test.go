//go:build windows

package main

import (
	"net"
	"net/netip"
	"os"
	"testing"
)

func TestDiscovery(t *testing.T) {
	dir := findV2rayNDir()
	t.Logf("v2rayN dir: %q", dir)
	t.Logf("cores: %v", coreExecutables(dir))
	t.Logf("servers: %v", proxyServers(dir))
	t.Logf("socks alive: %v", socksAlive("127.0.0.1:10808"))
	t.Logf("conflict: %q", conflict())
	up, err := findUplink(nil)
	if err != nil {
		t.Fatal(err)
	}
	t.Logf("uplink: %s dns=%v idx=%d", up, up.DNS, up.Index)
	for _, h := range proxyServers(dir) {
		ips, err := resolveDirect(up, h)
		t.Logf("resolve %s -> %v %v", h, ips, err)
	}
}

func TestOwnerLookup(t *testing.T) {
	ln, _ := net.Listen("tcp4", "127.0.0.1:0")
	defer ln.Close()
	go ln.Accept()
	c, err := net.Dial("tcp4", ln.Addr().String())
	if err != nil {
		t.Fatal(err)
	}
	defer c.Close()
	la := c.LocalAddr().(*net.TCPAddr)
	pid := tcpOwner(netip.MustParseAddr("127.0.0.1"), uint16(la.Port))
	if int(pid) != os.Getpid() {
		t.Fatalf("tcp owner = %d, want %d", pid, os.Getpid())
	}
	t.Logf("tcp owner ok: %s", pidPath(pid))

	u, _ := net.ListenUDP("udp4", &net.UDPAddr{IP: net.IPv4zero})
	defer u.Close()
	if p := udpOwner(uint16(u.LocalAddr().(*net.UDPAddr).Port)); int(p) != os.Getpid() {
		t.Fatalf("udp owner = %d", p)
	}
}

func TestInternetAdapters(t *testing.T) {
	as, err := internetAdapters()
	if err != nil {
		t.Fatal(err)
	}
	for _, a := range as {
		t.Logf("%+v", a)
	}
}

func TestHostsStrip(t *testing.T) {
	in := "a\r\n" + hostsBegin + "\r\n1.2.3.4 x\r\n" + hostsEnd + "\r\nb\r\n"
	if got := stripPins(in); got != "a\r\nb\r\n" {
		t.Fatalf("%q", got)
	}
}
