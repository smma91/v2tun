//go:build windows

package main

import "testing"

func TestExitInfoAndLeak(t *testing.T) {
	info, err := exitInfo()
	t.Logf("exit: %+v err=%v", info, err)
	t.Logf("leak: %+v", leakCheck())
}
