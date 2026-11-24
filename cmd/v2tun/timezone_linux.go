//go:build linux

package main

import (
	"errors"
	"os"
	"os/exec"
	"strings"
)

// getTZ returns the system time zone (IANA name, e.g. "Europe/Berlin").
func getTZ() (string, error) {
	if out, err := exec.Command("timedatectl", "show", "-p", "Timezone", "--value").Output(); err == nil {
		if tz := strings.TrimSpace(string(out)); tz != "" {
			return tz, nil
		}
	}
	l, err := os.Readlink("/etc/localtime")
	if err != nil {
		return "", err
	}
	if _, tz, ok := strings.Cut(l, "zoneinfo/"); ok {
		return tz, nil
	}
	return "", errors.New("unknown time zone")
}

// setTZ changes the system time zone through systemd-timedated (the desktop
// may ask for the password once).
func setTZ(id string) error {
	out, err := exec.Command("timedatectl", "set-timezone", id).CombinedOutput()
	if err != nil {
		return errors.New(strings.TrimSpace(string(out)))
	}
	return nil
}

// tzTarget: Linux uses the same IANA names as the exit IP services.
func tzTarget(iana string) (string, bool) { return iana, iana != "" }
