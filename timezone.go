//go:build windows

package main

import (
	"os/exec"
	"strings"
	"syscall"
)

// IANA (reported by ipinfo / browsers) -> Windows time zone id.
var ianaToWindows = map[string]string{
	"Europe/Berlin": "W. Europe Standard Time", "Europe/Amsterdam": "W. Europe Standard Time",
	"Europe/Stockholm": "W. Europe Standard Time", "Europe/Zurich": "W. Europe Standard Time",
	"Europe/Vienna": "W. Europe Standard Time", "Europe/Rome": "W. Europe Standard Time",
	"Europe/Oslo": "W. Europe Standard Time", "Europe/Luxembourg": "W. Europe Standard Time",
	"Europe/Paris": "Romance Standard Time", "Europe/Brussels": "Romance Standard Time",
	"Europe/Madrid": "Romance Standard Time", "Europe/Copenhagen": "Romance Standard Time",
	"Europe/Warsaw": "Central European Standard Time", "Europe/Prague": "Central Europe Standard Time",
	"Europe/Budapest": "Central Europe Standard Time", "Europe/Helsinki": "FLE Standard Time",
	"Europe/Kyiv": "FLE Standard Time", "Europe/Kiev": "FLE Standard Time", "Europe/Riga": "FLE Standard Time",
	"Europe/Tallinn": "FLE Standard Time", "Europe/Vilnius": "FLE Standard Time", "Europe/Sofia": "FLE Standard Time",
	"Europe/Bucharest": "GTB Standard Time", "Europe/Athens": "GTB Standard Time",
	"Europe/London": "GMT Standard Time", "Europe/Dublin": "GMT Standard Time", "Europe/Lisbon": "GMT Standard Time",
	"Europe/Istanbul": "Turkey Standard Time", "Europe/Moscow": "Russian Standard Time",
	"Asia/Dubai": "Arabian Standard Time", "Asia/Tokyo": "Tokyo Standard Time",
	"Asia/Singapore": "Singapore Standard Time", "Asia/Hong_Kong": "China Standard Time",
	"America/New_York": "Eastern Standard Time", "America/Toronto": "Eastern Standard Time",
	"America/Chicago": "Central Standard Time", "America/Denver": "Mountain Standard Time",
	"America/Los_Angeles": "Pacific Standard Time", "America/Vancouver": "Pacific Standard Time",
	"Asia/Tehran": "Iran Standard Time",
}

func tzutil(args ...string) (string, error) {
	cmd := exec.Command("tzutil", args...)
	cmd.SysProcAttr = &syscall.SysProcAttr{HideWindow: true, CreationFlags: 0x08000000}
	out, err := cmd.Output()
	return strings.TrimSpace(string(out)), err
}

// tzManager keeps the Windows time zone in line with the proxy exit, so the
// browser time zone does not contradict the IP location. Restored on stop.
type tzManager struct{ orig, current string }

func newTZManager() *tzManager {
	orig, err := tzutil("/g")
	if err != nil {
		return nil
	}
	return &tzManager{orig: orig, current: orig}
}

func (m *tzManager) Update(logf func(string, string, ...any)) {
	info, err := exitInfo()
	if err != nil || info.Timezone == "" {
		return
	}
	target, ok := ianaToWindows[info.Timezone]
	if !ok || target == m.current {
		return
	}
	if _, err := tzutil("/s", target); err != nil {
		logf("warn", "time zone: %v", err)
		return
	}
	m.current = target
	logf("info", "Time zone → %s (reopen your browser)", info.Timezone)
}

func (m *tzManager) Restore(logf func(string, string, ...any)) {
	if m.current == m.orig {
		return
	}
	if _, err := tzutil("/s", m.orig); err == nil {
		logf("info", "Time zone restored to %s", m.orig)
	}
}
