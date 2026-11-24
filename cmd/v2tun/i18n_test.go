//go:build windows || linux

package main

import "testing"

func TestT(t *testing.T) {
	const key = "Tunnel disconnected and network settings restored"
	for _, l := range []string{"en", "zh", "ar", "fa"} {
		setLang(l)
		got := T(key)
		t.Logf("%s: %q | %q", l, got, T("Quit (disconnect tunnel)"))
		if (l == "en") != (got == key) {
			t.Errorf("%s: got %q", l, got)
		}
	}
	setLang("en")
}
