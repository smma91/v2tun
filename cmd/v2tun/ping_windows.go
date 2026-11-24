//go:build windows

package main

import (
	"encoding/binary"
	"errors"
	"net/netip"
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
