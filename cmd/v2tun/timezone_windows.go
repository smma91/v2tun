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

func getTZ() (string, error) { return tzutil("/g") }

func setTZ(id string) error {
	_, err := tzutil("/s", id)
	return err
}

func tzTarget(iana string) (string, bool) {
	t, ok := ianaToWindows[iana]
	return t, ok
}
