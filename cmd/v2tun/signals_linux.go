//go:build linux

package main

import (
	"os"
	"os/signal"
	"syscall"
)

// handleSignals: logout, shutdown or `kill` disconnect cleanly (the GUI
// toolkit would otherwise keep running and leave that to the watchdog).
func handleSignals(a *App) {
	c := make(chan os.Signal, 1)
	signal.Notify(c, syscall.SIGTERM, syscall.SIGINT, syscall.SIGHUP)
	go func() {
		defer recoverPanic("signals")
		<-c
		a.quit()
	}()
}
