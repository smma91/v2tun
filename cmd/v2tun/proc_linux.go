//go:build linux

package main

import (
	"bufio"
	"encoding/hex"
	"net/netip"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"time"
)

// socketInode finds the inode of the local socket addr:port in
// /proc/net/{tcp,udp}[6] (port only when addr is invalid).
func socketInode(proto string, addr netip.Addr, port uint16) string {
	for _, f := range []string{"/proc/net/" + proto, "/proc/net/" + proto + "6"} {
		file, err := os.Open(f)
		if err != nil {
			continue
		}
		sc := bufio.NewScanner(file)
		sc.Scan() // header
		for sc.Scan() {
			fields := strings.Fields(sc.Text())
			if len(fields) < 10 {
				continue
			}
			ipHex, portHex, ok := strings.Cut(fields[1], ":")
			if !ok {
				continue
			}
			if p, err := strconv.ParseUint(portHex, 16, 16); err != nil || uint16(p) != port {
				continue
			}
			if addr.IsValid() {
				if ip, ok := parseProcIP(ipHex); ok && !ip.IsUnspecified() && ip != addr {
					continue
				}
			}
			file.Close()
			return fields[9]
		}
		file.Close()
	}
	return ""
}

// parseProcIP decodes the address format of /proc/net/*: 32-bit words in host
// (little-endian) order; IPv4-mapped IPv6 addresses become IPv4.
func parseProcIP(h string) (netip.Addr, bool) {
	b, err := hex.DecodeString(h)
	if err != nil || (len(b) != 4 && len(b) != 16) {
		return netip.Addr{}, false
	}
	for i := 0; i < len(b); i += 4 {
		b[i], b[i+1], b[i+2], b[i+3] = b[i+3], b[i+2], b[i+1], b[i]
	}
	a, _ := netip.AddrFromSlice(b)
	return a.Unmap(), true
}

var (
	inodeMu   sync.Mutex
	inodePID  = map[string]uint32{}
	inodeScan time.Time
)

// pidOfInode finds the process holding a socket. A full scan of /proc is
// costly, so the map is reused and rebuilt at most every 200 ms. Only this
// user's processes can be inspected, which covers v2rayN's cores and apps.
func pidOfInode(inode string) uint32 {
	if inode == "" || inode == "0" {
		return 0
	}
	inodeMu.Lock()
	defer inodeMu.Unlock()
	if pid, ok := inodePID[inode]; ok {
		return pid
	}
	if time.Since(inodeScan) < 200*time.Millisecond {
		return 0
	}
	inodeScan = time.Now()
	m := map[string]uint32{}
	procs, _ := os.ReadDir("/proc")
	for _, p := range procs {
		pid, err := strconv.ParseUint(p.Name(), 10, 32)
		if err != nil {
			continue
		}
		dir := filepath.Join("/proc", p.Name(), "fd")
		fds, err := os.ReadDir(dir)
		if err != nil {
			continue
		}
		for _, fd := range fds {
			l, err := os.Readlink(filepath.Join(dir, fd.Name()))
			if err == nil && strings.HasPrefix(l, "socket:[") {
				m[strings.TrimSuffix(strings.TrimPrefix(l, "socket:["), "]")] = uint32(pid)
			}
		}
	}
	inodePID = m
	return m[inode]
}

// tcpOwner returns the PID owning the local IPv4 TCP endpoint addr:port.
func tcpOwner(addr netip.Addr, port uint16) uint32 {
	return pidOfInode(socketInode("tcp", addr, port))
}

// udpOwner returns the PID owning the local UDP port.
func udpOwner(port uint16) uint32 {
	return pidOfInode(socketInode("udp", netip.Addr{}, port))
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
	if pid == 0 {
		return ""
	}
	pidMu.Lock()
	e, ok := pidCache[pid]
	pidMu.Unlock()
	if ok && time.Since(e.at) < 30*time.Second {
		return e.path
	}
	path, _ := os.Readlink("/proc/" + strconv.FormatUint(uint64(pid), 10) + "/exe")
	path = strings.TrimSuffix(path, " (deleted)")
	pidMu.Lock()
	pidCache[pid] = pidEntry{path, time.Now()}
	pidMu.Unlock()
	return path
}

func allProcessPaths() []string {
	procs, _ := os.ReadDir("/proc")
	var out []string
	for _, p := range procs {
		if _, err := strconv.Atoi(p.Name()); err != nil {
			continue
		}
		if l, err := os.Readlink("/proc/" + p.Name() + "/exe"); err == nil {
			out = append(out, strings.TrimSuffix(l, " (deleted)"))
		}
	}
	return out
}

// processPathByName finds a running program by its name (v2rayN runs as a
// .NET app whose process is called "v2rayN").
func processPathByName(name string) string {
	procs, _ := os.ReadDir("/proc")
	for _, p := range procs {
		if _, err := strconv.Atoi(p.Name()); err != nil {
			continue
		}
		comm, err := os.ReadFile("/proc/" + p.Name() + "/comm")
		if err != nil || !strings.EqualFold(strings.TrimSpace(string(comm)), name) {
			continue
		}
		if l, err := os.Readlink("/proc/" + p.Name() + "/exe"); err == nil {
			return strings.TrimSuffix(l, " (deleted)")
		}
	}
	return ""
}
