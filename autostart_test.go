//go:build windows

package main

import (
	"os"
	"testing"
)

// Creates and removes the real logon task; run manually with V2TUN_TASK_TEST=1.
func TestAutostartTask(t *testing.T) {
	if os.Getenv("V2TUN_TASK_TEST") == "" {
		t.Skip("set V2TUN_TASK_TEST=1")
	}
	if err := setAutostart(true); err != nil {
		t.Fatal(err)
	}
	if !autostartEnabled() {
		t.Fatal("task not found after create")
	}
	if err := setAutostart(false); err != nil {
		t.Fatal(err)
	}
	if autostartEnabled() {
		t.Fatal("task still there")
	}
}
