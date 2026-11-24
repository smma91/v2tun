//go:build linux

package main

import (
	"os"
	"path/filepath"
)

// Settings and server files hold credentials: only this user may read them.
func protectFile(path string) error { return os.Chmod(path, 0o600) }

// writeSecret writes a file with mode 0600, atomically (no half-written
// settings if the app is killed in the middle).
func writeSecret(path string, data []byte) error {
	tmp, err := os.CreateTemp(filepath.Dir(path), ".tmp-*")
	if err != nil {
		return err
	}
	defer os.Remove(tmp.Name())
	if _, err := tmp.Write(data); err != nil {
		tmp.Close()
		return err
	}
	if err := tmp.Chmod(0o600); err != nil {
		tmp.Close()
		return err
	}
	if err := tmp.Close(); err != nil {
		return err
	}
	return os.Rename(tmp.Name(), path)
}
