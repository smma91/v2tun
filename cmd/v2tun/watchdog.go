//go:build windows

package main

import (
	"encoding/json"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"syscall"

	"golang.org/x/sys/windows"
)

// runState records what a connected tunnel changed on the system, so it can
// be undone even if v2tun is killed (Task Manager, crash, power loss).
//
// What a killed process leaves behind, and who cleans it:
//   - firewall (kill switch): WFP dynamic session -> Windows removes it
//   - TUN adapter + its routes/DNS: Wintun removes it with the process
//   - hosts file pins, Windows time zone: the watchdog below, or the next start
type runState struct {
	PID    int    `json:"pid"`
	Since  int64  `json:"since"`
	OrigTZ string `json:"origTz"`
	// Set while "proxy only" mode has changed the Windows proxy setting.
	Proxy *proxyBackup `json:"proxy,omitempty"`
}

func statePath() string { return filepath.Join(dataDir(), "state.json") }

func saveState(s runState) {
	b, _ := json.Marshal(s)
	writeSecret(statePath(), b)
}

func clearState() { os.Remove(statePath()) }

func loadState() (runState, bool) {
	var s runState
	b, err := os.ReadFile(statePath())
	if err != nil || json.Unmarshal(b, &s) != nil {
		return s, false
	}
	return s, true
}

func processAlive(pid int) bool {
	h, err := windows.OpenProcess(windows.SYNCHRONIZE, false, uint32(pid))
	if err != nil {
		return false
	}
	defer windows.CloseHandle(h)
	ev, _ := windows.WaitForSingleObject(h, 0)
	return ev == uint32(windows.WAIT_TIMEOUT)
}

// cleanupStale restores the system after a tunnel that did not shut down
// cleanly. Returns true if something had to be repaired.
func cleanupStale() bool {
	unpinHosts() // harmless when nothing is pinned
	s, ok := loadState()
	if !ok || (s.PID != os.Getpid() && processAlive(s.PID)) {
		return false
	}
	if s.OrigTZ != "" {
		tzutil("/s", s.OrigTZ)
	}
	if s.Proxy != nil { // otherwise every browser would point at a dead proxy
		restoreSystemProxy(*s.Proxy)
	}
	run("ipconfig", "/flushdns")
	clearState()
	writeLogFile(LogLine{Level: "warn", Msg: "network restored after an unclean exit (pid " + strconv.Itoa(s.PID) + ")"})
	return true
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
