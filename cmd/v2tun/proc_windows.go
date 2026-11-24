//go:build windows

package main

import (
	"encoding/binary"
	"net/netip"
	"strings"
	"sync"
	"time"
	"unsafe"

	"golang.org/x/sys/windows"
)

var (
	iphlpapi                = windows.NewLazySystemDLL("iphlpapi.dll")
	procGetExtendedTcpTable = iphlpapi.NewProc("GetExtendedTcpTable")
	procGetExtendedUdpTable = iphlpapi.NewProc("GetExtendedUdpTable")
)

const (
	tcpTableOwnerPidAll = 5
	udpTableOwnerPid    = 1
)

func extendedTable(proc *windows.LazyProc, class uintptr) []byte {
	size := uint32(64 * 1024)
	for i := 0; i < 4; i++ {
		buf := make([]byte, size)
		r, _, _ := proc.Call(uintptr(unsafe.Pointer(&buf[0])), uintptr(unsafe.Pointer(&size)), 0, windows.AF_INET, class, 0)
		if r == 0 {
			return buf
		}
		if windows.Errno(r) != windows.ERROR_INSUFFICIENT_BUFFER {
			return nil
		}
		size += 16 * 1024
	}
	return nil
}

func netPort(v uint32) uint16 {
	return uint16(v>>8&0xff) | uint16(v&0xff)<<8
}

// tcpOwner returns the PID owning the local IPv4 TCP endpoint addr:port.
func tcpOwner(addr netip.Addr, port uint16) uint32 {
	buf := extendedTable(procGetExtendedTcpTable, tcpTableOwnerPidAll)
	if len(buf) < 4 {
		return 0
	}
	n := int(binary.LittleEndian.Uint32(buf))
	want := addr.As4()
	for i := 0; i < n; i++ {
		row := buf[4+i*24:]
		if len(row) < 24 {
			break
		}
		lp := netPort(binary.LittleEndian.Uint32(row[8:]))
		if lp != port {
			continue
		}
		la := [4]byte(row[4:8])
		if la == want || la == [4]byte{} {
			return binary.LittleEndian.Uint32(row[20:])
		}
	}
	return 0
}

// udpOwner returns the PID owning the local IPv4 UDP port.
func udpOwner(port uint16) uint32 {
	buf := extendedTable(procGetExtendedUdpTable, udpTableOwnerPid)
	if len(buf) < 4 {
		return 0
	}
	n := int(binary.LittleEndian.Uint32(buf))
	for i := 0; i < n; i++ {
		row := buf[4+i*12:]
		if len(row) < 12 {
			break
		}
		if netPort(binary.LittleEndian.Uint32(row[4:])) == port {
			return binary.LittleEndian.Uint32(row[8:])
		}
	}
	return 0
}

type pidEntry struct {
	path string
	at   time.Time
}

var (
	pidMu    sync.Mutex
	pidCache = map[uint32]pidEntry{}
)

func pidPath(pid uint32) string {
	if pid == 0 || pid == 4 {
		return ""
	}
	pidMu.Lock()
	e, ok := pidCache[pid]
	pidMu.Unlock()
	if ok && time.Since(e.at) < 30*time.Second {
		return e.path
	}
	path := queryPath(pid)
	pidMu.Lock()
	pidCache[pid] = pidEntry{path, time.Now()}
	pidMu.Unlock()
	return path
}

func queryPath(pid uint32) string {
	h, err := windows.OpenProcess(windows.PROCESS_QUERY_LIMITED_INFORMATION, false, pid)
	if err != nil {
		return ""
	}
	defer windows.CloseHandle(h)
	buf := make([]uint16, windows.MAX_LONG_PATH)
	n := uint32(len(buf))
	if windows.QueryFullProcessImageName(h, 0, &buf[0], &n) != nil {
		return ""
	}
	return windows.UTF16ToString(buf[:n])
}

func allProcessPaths() []string {
	snap, err := windows.CreateToolhelp32Snapshot(windows.TH32CS_SNAPPROCESS, 0)
	if err != nil {
		return nil
	}
	defer windows.CloseHandle(snap)
	var out []string
	var pe windows.ProcessEntry32
	pe.Size = uint32(unsafe.Sizeof(pe))
	for err := windows.Process32First(snap, &pe); err == nil; err = windows.Process32Next(snap, &pe) {
		if p := queryPath(pe.ProcessID); p != "" {
			out = append(out, p)
		}
	}
	return out
}

func processPathByName(name string) string {
	snap, err := windows.CreateToolhelp32Snapshot(windows.TH32CS_SNAPPROCESS, 0)
	if err != nil {
		return ""
	}
	defer windows.CloseHandle(snap)
	var pe windows.ProcessEntry32
	pe.Size = uint32(unsafe.Sizeof(pe))
	for err := windows.Process32First(snap, &pe); err == nil; err = windows.Process32Next(snap, &pe) {
		if strings.EqualFold(windows.UTF16ToString(pe.ExeFile[:]), name) {
			if p := queryPath(pe.ProcessID); p != "" {
				return p
			}
		}
	}
	return ""
}
