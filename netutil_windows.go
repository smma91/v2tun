//go:build windows

package main

import (
	"context"
	"fmt"
	"math/bits"
	"net"
	"net/netip"
	"os/exec"
	"strings"
	"syscall"
	"time"
	"unsafe"

	"golang.org/x/net/dns/dnsmessage"
	"golang.org/x/sys/windows"
)

type adapterInfo struct {
	Name, Description string
	Index             uint32
	LUID              uint64
	Up                bool
	V4                []netip.Addr
	DNS               []netip.Addr
}

func listAdapters() ([]adapterInfo, error) {
	size := uint32(32 * 1024)
	var buf []byte
	for i := 0; ; i++ {
		buf = make([]byte, size)
		err := windows.GetAdaptersAddresses(windows.AF_UNSPEC, 0, 0, (*windows.IpAdapterAddresses)(unsafe.Pointer(&buf[0])), &size)
		if err == nil {
			break
		}
		if err != windows.ERROR_BUFFER_OVERFLOW || i > 4 {
			return nil, err
		}
	}
	var out []adapterInfo
	for p := (*windows.IpAdapterAddresses)(unsafe.Pointer(&buf[0])); p != nil; p = p.Next {
		a := adapterInfo{
			Name:        windows.UTF16PtrToString(p.FriendlyName),
			Description: windows.UTF16PtrToString(p.Description),
			Index:       p.IfIndex,
			LUID:        p.Luid,
			Up:          p.OperStatus == windows.IfOperStatusUp,
		}
		for u := p.FirstUnicastAddress; u != nil; u = u.Next {
			if ip, ok := netip.AddrFromSlice(u.Address.IP()); ok && ip.Unmap().Is4() {
				a.V4 = append(a.V4, ip.Unmap())
			}
		}
		for d := p.FirstDnsServerAddress; d != nil; d = d.Next {
			if ip, ok := netip.AddrFromSlice(d.Address.IP()); ok && ip.Unmap().Is4() {
				a.DNS = append(a.DNS, ip.Unmap())
			}
		}
		out = append(out, a)
	}
	return out, nil
}

// waitAddrReady waits until the adapter's IPv4 address can be used
// (duplicate-address detection finished: state "preferred").
func waitAddrReady(luid uint64, timeout time.Duration) bool {
	deadline := time.Now().Add(timeout)
	for time.Now().Before(deadline) {
		if addrPreferred(luid) {
			return true
		}
		time.Sleep(100 * time.Millisecond)
	}
	return false
}

func addrPreferred(luid uint64) bool {
	size := uint32(32 * 1024)
	buf := make([]byte, size)
	if windows.GetAdaptersAddresses(windows.AF_INET, 0, 0, (*windows.IpAdapterAddresses)(unsafe.Pointer(&buf[0])), &size) != nil {
		return false
	}
	for p := (*windows.IpAdapterAddresses)(unsafe.Pointer(&buf[0])); p != nil; p = p.Next {
		if p.Luid != luid || p.OperStatus != windows.IfOperStatusUp {
			continue
		}
		for u := p.FirstUnicastAddress; u != nil; u = u.Next {
			if u.DadState == windows.IpDadStatePreferred {
				return true
			}
		}
	}
	return false
}

// uplink is the real network path (Wi-Fi, Ethernet, phone hotspot...).
type uplink struct {
	Name    string
	Index   uint32
	LUID    uint64
	Addr    netip.Addr
	Gateway netip.Addr
	DNS     []netip.Addr
}

func (u uplink) String() string {
	return fmt.Sprintf("%s (%s via %s)", u.Name, u.Addr, u.Gateway)
}

func sockaddrV4(r *windows.RawSockaddrInet) netip.Addr {
	return netip.AddrFrom4(*(*[4]byte)(unsafe.Pointer(&r.Data[0])))
}

// findUplink picks the interface the system would use for the internet,
// ignoring our own tunnel (and other excluded interfaces).
func findUplink(exclude map[uint64]bool) (uplink, error) {
	var tbl *windows.MibIpForwardTable2
	if err := windows.GetIpForwardTable2(windows.AF_INET, &tbl); err != nil {
		return uplink{}, err
	}
	defer windows.FreeMibTable(unsafe.Pointer(tbl))

	adapters, err := listAdapters()
	if err != nil {
		return uplink{}, err
	}
	byLUID := map[uint64]adapterInfo{}
	for _, a := range adapters {
		byLUID[a.LUID] = a
	}

	target := netip.MustParseAddr("1.1.1.1")
	bestLen, bestMetric := -1, uint32(0)
	var best uplink
	for _, r := range tbl.Rows() {
		if exclude[r.InterfaceLuid] {
			continue
		}
		pfx := netip.PrefixFrom(sockaddrV4(&r.DestinationPrefix.Prefix), int(r.DestinationPrefix.PrefixLength))
		if !pfx.Contains(target) {
			continue
		}
		a, ok := byLUID[r.InterfaceLuid]
		if !ok || !a.Up || len(a.V4) == 0 {
			continue
		}
		l := pfx.Bits()
		if l > bestLen || l == bestLen && r.Metric < bestMetric {
			bestLen, bestMetric = l, r.Metric
			best = uplink{Name: a.Name, Index: a.Index, LUID: a.LUID, Addr: a.V4[0], Gateway: sockaddrV4(&r.NextHop), DNS: a.DNS}
		}
	}
	if bestLen < 0 {
		return uplink{}, fmt.Errorf("no internet connection found")
	}
	return best, nil
}

// bindToInterface forces a socket out of a given interface (IP_UNICAST_IF),
// regardless of the routing table that points everything into the tunnel.
func bindToInterface(index uint32) func(network, address string, c syscall.RawConn) error {
	return func(network, address string, c syscall.RawConn) error {
		var serr error
		err := c.Control(func(fd uintptr) {
			const ipUnicastIf = 31
			serr = windows.SetsockoptInt(windows.Handle(fd), windows.IPPROTO_IP, ipUnicastIf, int(bits.ReverseBytes32(index)))
		})
		if err != nil {
			return err
		}
		return serr
	}
}

func directDialer(u uplink) *net.Dialer {
	return &net.Dialer{
		Timeout:   10 * time.Second,
		LocalAddr: &net.TCPAddr{IP: u.Addr.AsSlice()},
		Control:   bindToInterface(u.Index),
	}
}

func directListenUDP(u uplink) (net.PacketConn, error) {
	lc := net.ListenConfig{Control: bindToInterface(u.Index)}
	return lc.ListenPacket(context.Background(), "udp4", net.JoinHostPort(u.Addr.String(), "0"))
}

// resolveDirect resolves A records outside the tunnel (used to pin the proxy
// server name, which the core must be able to resolve without the tunnel).
func resolveDirect(u uplink, host string) ([]netip.Addr, error) {
	if a, err := netip.ParseAddr(host); err == nil {
		return []netip.Addr{a}, nil
	}
	name, err := dnsmessage.NewName(strings.TrimSuffix(host, ".") + ".")
	if err != nil {
		return nil, err
	}
	servers := append(append([]netip.Addr{}, u.DNS...), netip.MustParseAddr("1.1.1.1"), netip.MustParseAddr("8.8.8.8"))
	var lastErr error
	for _, srv := range servers {
		ips, err := queryA(u, srv, name)
		if err == nil && len(ips) > 0 {
			return ips, nil
		}
		lastErr = err
	}
	if lastErr == nil {
		lastErr = fmt.Errorf("no A record for %s", host)
	}
	return nil, lastErr
}

func queryA(u uplink, server netip.Addr, name dnsmessage.Name) ([]netip.Addr, error) {
	msg := dnsmessage.Message{
		Header:    dnsmessage.Header{ID: uint16(time.Now().UnixNano()), RecursionDesired: true},
		Questions: []dnsmessage.Question{{Name: name, Type: dnsmessage.TypeA, Class: dnsmessage.ClassINET}},
	}
	q, err := msg.Pack()
	if err != nil {
		return nil, err
	}
	pc, err := directListenUDP(u)
	if err != nil {
		return nil, err
	}
	defer pc.Close()
	pc.SetDeadline(time.Now().Add(3 * time.Second))
	if _, err := pc.WriteTo(q, net.UDPAddrFromAddrPort(netip.AddrPortFrom(server, 53))); err != nil {
		return nil, err
	}
	buf := make([]byte, 1500)
	n, _, err := pc.ReadFrom(buf)
	if err != nil {
		return nil, err
	}
	var resp dnsmessage.Message
	if err := resp.Unpack(buf[:n]); err != nil {
		return nil, err
	}
	var out []netip.Addr
	for _, a := range resp.Answers {
		if r, ok := a.Body.(*dnsmessage.AResource); ok {
			out = append(out, netip.AddrFrom4(r.A))
		}
	}
	return out, nil
}

func run(name string, args ...string) error {
	cmd := exec.Command(name, args...)
	cmd.SysProcAttr = &syscall.SysProcAttr{HideWindow: true, CreationFlags: 0x08000000}
	out, err := cmd.CombinedOutput()
	if err != nil {
		return fmt.Errorf("%s %s: %v %s", name, strings.Join(args, " "), err, strings.TrimSpace(string(out)))
	}
	return nil
}
