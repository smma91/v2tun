//go:build windows || linux

package main

import "v2tun/internal/i18n"

// The translations live in frontend/i18n.json (shared with the web UI); the
// logic is in internal/i18n, shared with the mobile apps.
func init() {
	i18n.SetSource(func() []byte {
		b, _ := assets.ReadFile("frontend/i18n.json")
		return b
	})
}

func validLang(l string) string { return i18n.ValidLang(l) }

func setLang(l string) { i18n.SetLang(l) }

func getLang() string { return i18n.Lang() }

// T returns the current-language version of an English string.
func T(s string) string { return i18n.T(s) }

// trErr creates a translatable error from an English format string.
func trErr(format string, args ...any) error { return i18n.Err(format, args...) }

func retranslate(s string) string { return i18n.Retranslate(s) }

func retranslateArgs(args []any) []any { return i18n.RetranslateArgs(args) }

// pick returns the wording for this operating system; both are i18n keys.
func pick(windows, linux string) string {
	if platform == "linux" {
		return linux
	}
	return windows
}
