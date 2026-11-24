package main

import (
	"fmt"
	"os"
	"path/filepath"
	"runtime/debug"
	"sync"
	"time"
)

// Everything the UI log shows is also kept in %ProgramData%\v2tun\v2tun.log,
// together with crash traces, so problems can be diagnosed afterwards.
var (
	logMu   sync.Mutex
	logFile *os.File
)

func logPath() string { return filepath.Join(dataDir(), "v2tun.log") }

func openLogFile() {
	p := logPath()
	if fi, err := os.Stat(p); err == nil && fi.Size() > 2<<20 {
		os.Rename(p, p+".old")
	}
	f, err := os.OpenFile(p, os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0o644)
	if err != nil {
		return
	}
	logFile = f
	fmt.Fprintf(f, "\n===== v2tun started %s args=%v =====\n", time.Now().Format(time.DateTime), os.Args[1:])
	debug.SetCrashOutput(f, debug.CrashOptions{})
}

// recoverPanic is deferred at the top of long-running goroutines: a bug in
// one connection must not take the whole app (and the tunnel) down. The
// panic and its stack go to the log file for diagnosis.
func recoverPanic(where string) {
	if r := recover(); r != nil {
		writeLogFile(LogLine{Time: time.Now().Format("15:04:05"), Level: "error",
			Msg: fmt.Sprintf("panic in %s: %v\n%s", where, r, debug.Stack())})
	}
}

func writeLogFile(l LogLine) {
	logMu.Lock()
	defer logMu.Unlock()
	if logFile != nil {
		fmt.Fprintf(logFile, "%s %s [%s] %s\n", time.Now().Format("2006-01-02"), l.Time, l.Level, l.Msg)
	}
}
