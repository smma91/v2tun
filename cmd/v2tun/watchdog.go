//go:build windows || linux

package main

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strconv"
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

// cleanupStale restores the system after a tunnel that did not shut down
// cleanly. Returns true if something had to be repaired.
func cleanupStale() bool {
	unpinHosts() // harmless when nothing is pinned
	cleanupNetwork()
	s, ok := loadState()
	if !ok || (s.PID != os.Getpid() && processAlive(s.PID)) {
		return false
	}
	if s.OrigTZ != "" {
		setTZ(s.OrigTZ)
	}
	if s.Proxy != nil { // otherwise every browser would point at a dead proxy
		restoreSystemProxy(*s.Proxy)
	}
	cleanupNetwork()
	flushDNSCache()
	clearState()
	writeLogFile(LogLine{Level: "warn", Msg: "network restored after an unclean exit (pid " + strconv.Itoa(s.PID) + ")"})
	return true
}
