//go:build linux

package main

import (
	"errors"
	"os"
	"os/exec"
	"strconv"
	"syscall"
	"time"
)

func processAlive(pid int) bool {
	err := syscall.Kill(pid, 0)
	return err == nil || errors.Is(err, syscall.EPERM)
}

// startWatchdog launches a small copy of v2tun in its own session that waits
// for this process to end and, if it ended without disconnecting, restores
// the system (the TUN interface goes away with the process by itself; the
// firewall tables, the proxy setting and the time zone do not).
func startWatchdog() {
	exe, err := os.Executable()
	if err != nil {
		return
	}
	cmd := exec.Command(exe, "--watchdog", strconv.Itoa(os.Getpid()))
	cmd.SysProcAttr = &syscall.SysProcAttr{Setsid: true}
	if hasPrivileges() {
		cmd.SysProcAttr.AmbientCaps = netCaps
	}
	if cmd.Start() == nil {
		go cmd.Wait()
	}
}

// watchdogMain is the entry point of the watchdog process.
func watchdogMain(parent int) {
	for processAlive(parent) {
		time.Sleep(time.Second)
	}
	if s, ok := loadState(); ok && s.PID == parent {
		cleanupStale()
	}
}
