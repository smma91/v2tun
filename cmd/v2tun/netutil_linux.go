//go:build linux

package main

import (
	"bufio"
	"context"
	"errors"
	"fmt"
	"net"
	"net/netip"
	"os"
	"os/exec"
	"strings"
	"syscall"
	"time"

	"github.com/vishvananda/netlink"
	"golang.org/x/sys/unix"
)

type adapterInfo struct {
	Name, Description string
	Index             uint32
	LUID              uint64 // the interface index on Linux
	Up                bool
	V4                []netip.Addr
	DNS               []netip.Addr
}

func listAdapters() ([]adapterInfo, error) {
	ifs, err := net.Interfaces()
	if err != nil {
		return nil, err
	}
	var out []adapterInfo
	for _, i := range ifs {
		a := adapterInfo{Name: i.Name, Description: i.Name, Index: uint32(i.Index), LUID: uint64(i.Index),
			Up: i.Flags&net.FlagUp != 0 && i.Flags&net.FlagRunning != 0}
		addrs, _ := i.Addrs()
		for _, x := range addrs {
			if n, ok := x.(*net.IPNet); ok {
				if ip, ok := netip.AddrFromSlice(n.IP); ok && ip.Unmap().Is4() {
					a.V4 = append(a.V4, ip.Unmap())
				}
			}
		}
		out = append(out, a)
	}
	return out, nil
}

// waitAddrReady: IPv4 addresses on Linux are usable immediately.
func waitAddrReady(luid uint64, timeout time.Duration) bool { return true }

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

// findUplink picks the interface of the default route, ignoring our tunnel
// (and other excluded interfaces).
func findUplink(exclude map[uint64]bool) (uplink, error) {
	routes, err := netlink.RouteListFiltered(netlink.FAMILY_V4, &netlink.Route{Table: unix.RT_TABLE_MAIN}, netlink.RT_FILTER_TABLE)
	if err != nil {
		return uplink{}, err
	}
	target := netip.MustParseAddr("1.1.1.1")
	bestLen, bestPrio := -1, 0
	var best uplink
	for _, r := range routes {
		if r.LinkIndex <= 0 || exclude[uint64(r.LinkIndex)] {
			continue
		}
		pfx := netip.MustParsePrefix("0.0.0.0/0")
		if r.Dst != nil {
			ones, _ := r.Dst.Mask.Size()
			a, _ := netip.AddrFromSlice(r.Dst.IP.To4())
			pfx = netip.PrefixFrom(a, ones)
		}
		if !pfx.Contains(target) {
			continue
		}
		link, err := netlink.LinkByIndex(r.LinkIndex)
		if err != nil || link.Attrs().Name == tunName || link.Attrs().OperState == netlink.OperDown {
			continue
		}
		addrs, _ := netlink.AddrList(link, netlink.FAMILY_V4)
		if len(addrs) == 0 {
			continue
		}
		l := pfx.Bits()
		if l > bestLen || l == bestLen && r.Priority < bestPrio {
			bestLen, bestPrio = l, r.Priority
			src, _ := netip.AddrFromSlice(addrs[0].IP.To4())
			if r.Src != nil {
				src, _ = netip.AddrFromSlice(r.Src.To4())
			}
			gw, _ := netip.AddrFromSlice(r.Gw.To4())
			best = uplink{Name: link.Attrs().Name, Index: uint32(r.LinkIndex), LUID: uint64(r.LinkIndex), Addr: src, Gateway: gw}
		}
	}
	if bestLen < 0 {
		return uplink{}, errors.New("no internet connection found")
	}
	best.DNS = upstreamDNS()
	return best, nil
}

// upstreamDNS returns the real DNS servers of the network (not a local stub
// such as systemd-resolved's 127.0.0.53, whose queries we would capture).
func upstreamDNS() []netip.Addr {
	for _, f := range []string{"/run/systemd/resolve/resolv.conf", "/run/NetworkManager/no-stub-resolv.conf", "/etc/resolv.conf"} {
		file, err := os.Open(f)
		if err != nil {
			continue
		}
		var out []netip.Addr
		sc := bufio.NewScanner(file)
		for sc.Scan() {
			fields := strings.Fields(sc.Text())
			if len(fields) >= 2 && fields[0] == "nameserver" {
				if a, err := netip.ParseAddr(fields[1]); err == nil && a.Is4() && !a.IsLoopback() {
					out = append(out, a)
				}
			}
		}
		file.Close()
		if len(out) > 0 {
			return out
		}
	}
	return nil
}

// bindToInterface sends a socket out of the real interface, past the routes
// into the tunnel (SO_BINDTODEVICE), and marks it so the kill switch lets it
// through (SO_MARK).
func bindToInterface(name string) func(network, address string, c syscall.RawConn) error {
	return func(network, address string, c syscall.RawConn) error {
		var serr error
		err := c.Control(func(fd uintptr) {
			if serr = unix.BindToDevice(int(fd), name); serr != nil {
				return
			}
			serr = unix.SetsockoptInt(int(fd), unix.SOL_SOCKET, unix.SO_MARK, directMark)
		})
		if err != nil {
			return err
		}
		return serr
	}
}

func directDialer(u uplink) *net.Dialer {
	return &net.Dialer{Timeout: 10 * time.Second, Control: bindToInterface(u.Name)}
}

func directListenUDP(u uplink) (net.PacketConn, error) {
	lc := net.ListenConfig{Control: bindToInterface(u.Name)}
	return lc.ListenPacket(context.Background(), "udp4", "0.0.0.0:0")
}

// run executes a helper program with the network capabilities passed on
// (file capabilities are not inherited by child processes otherwise).
func run(name string, args ...string) error {
	cmd := exec.Command(name, args...)
	if hasPrivileges() {
		cmd.SysProcAttr = &syscall.SysProcAttr{AmbientCaps: netCaps}
	}
	out, err := cmd.CombinedOutput()
	if err != nil {
		return fmt.Errorf("%s %s: %v %s", name, strings.Join(args, " "), err, strings.TrimSpace(string(out)))
	}
	return nil
}
