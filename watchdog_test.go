//go:build windows

package main

import (
	"os"
	"os/exec"
	"strconv"
	"testing"
	"time"
)

// A killed process must be detected and its state cleaned by the watchdog.
func TestWatchdogAfterKill(t *testing.T) {
	t.Setenv("V2TUN_DATADIR", t.TempDir())
	victim := exec.Command("ping", "-n", "30", "127.0.0.1")
	if err := victim.Start(); err != nil {
		t.Fatal(err)
	}
	saveState(runState{PID: victim.Process.Pid})
	if !processAlive(victim.Process.Pid) {
		t.Fatal("victim should be alive")
	}
	done := make(chan struct{})
	go func() { watchdogMain(victim.Process.Pid); close(done) }()
	time.Sleep(300 * time.Millisecond)
	victim.Process.Kill() // like "End task" in Task Manager
	select {
	case <-done:
	case <-time.After(5 * time.Second):
		t.Fatal("watchdog did not notice the kill")
	}
	if _, ok := loadState(); ok {
		t.Fatal("state not cleaned")
	}
	t.Logf("pid %s killed -> watchdog cleaned up", strconv.Itoa(victim.Process.Pid))
	_ = os.Getpid
}
