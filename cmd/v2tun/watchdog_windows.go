//go:build windows

package main

import (
	"os"
	"os/exec"
	"strconv"
	"syscall"

	"golang.org/x/sys/windows"
)

func processAlive(pid int) bool {
	h, err := windows.OpenProcess(windows.SYNCHRONIZE, false, uint32(pid))
	if err != nil {
		return false
	}
	defer windows.CloseHandle(h)
	ev, _ := windows.WaitForSingleObject(h, 0)
	return ev == uint32(windows.WAIT_TIMEOUT)
}

// startWatchdog launches a tiny hidden copy of v2tun that waits for this
// process to end and, if it ended without disconnecting, restores the system.
func startWatchdog() {
	exe, err := os.Executable()
	if err != nil {
		return
	}
	cmd := exec.Command(exe, "--watchdog", strconv.Itoa(os.Getpid()))
	cmd.SysProcAttr = &syscall.SysProcAttr{HideWindow: true,
		CreationFlags: 0x08000000 | 0x00000200} // CREATE_NO_WINDOW | CREATE_NEW_PROCESS_GROUP
	cmd.Start()
}

// watchdogMain is the entry point of the watchdog process.
func watchdogMain(parent int) {
	h, err := windows.OpenProcess(windows.SYNCHRONIZE, false, uint32(parent))
	if err != nil {
		cleanupStale()
		return
	}
	windows.WaitForSingleObject(h, windows.INFINITE)
	windows.CloseHandle(h)
	if s, ok := loadState(); ok && s.PID == parent {
		cleanupStale()
	}
}

// cleanupNetwork: the WFP session and the Wintun adapter go away with the
// process; only the hotspot's firewall rule may be left.
func cleanupNetwork() { removeRelayRule() }
