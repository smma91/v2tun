//go:build windows

package main

import (
	"bytes"
	_ "embed"
	"fmt"
	"os"
	"path/filepath"
	"sync"

	"github.com/xjasonlyu/tun2socks/v2/core/device/iobased"
	"golang.zx2c4.com/wireguard/tun"
)

//go:embed wintun.dll
var wintunDLL []byte

// libcronet.dll is Chromium's network stack, used by the NaiveProxy protocol
// (sing-box loads it at runtime, so no C compiler is needed to build).
//
//go:embed libcronet.dll
var cronetDLL []byte

// placeDLL writes an embedded DLL next to the exe, where its loader looks.
func placeDLL(name string, data []byte) error {
	exe, err := os.Executable()
	if err != nil {
		return err
	}
	p := filepath.Join(filepath.Dir(exe), name)
	if fi, err := os.Stat(p); err == nil && fi.Size() == int64(len(data)) {
		if b, err := os.ReadFile(p); err == nil && bytes.Equal(b, data) {
			return nil
		}
	}
	return os.WriteFile(p, data, 0o644)
}

func ensureWintun() error { return placeDLL("wintun.dll", wintunDLL) }

func ensureCronet() error { return placeDLL("libcronet.dll", cronetDLL) }

const tunName = "v2tun"

// tunDevice is a Wintun adapter exposed as a gVisor link endpoint.
type tunDevice struct {
	*iobased.Endpoint
	nt     *tun.NativeTun
	rSizes []int
	rBufs  [][]byte
	wBufs  [][]byte
	rMu    sync.Mutex
	wMu    sync.Mutex
}

func openTun(mtu int) (*tunDevice, error) {
	dev, err := tun.CreateTUN(tunName, mtu)
	if err != nil {
		return nil, fmt.Errorf("create wintun adapter: %w", err)
	}
	t := &tunDevice{nt: dev.(*tun.NativeTun), rSizes: make([]int, 1), rBufs: make([][]byte, 1), wBufs: make([][]byte, 1)}
	m, err := dev.MTU()
	if err != nil {
		dev.Close()
		return nil, err
	}
	ep, err := iobased.New(t, uint32(m), 0)
	if err != nil {
		dev.Close()
		return nil, err
	}
	t.Endpoint = ep
	return t, nil
}

func (t *tunDevice) LUID() uint64 { return t.nt.LUID() }

func (t *tunDevice) Read(p []byte) (int, error) {
	t.rMu.Lock()
	defer t.rMu.Unlock()
	t.rBufs[0] = p
	_, err := t.nt.Read(t.rBufs, t.rSizes, 0)
	return t.rSizes[0], err
}

func (t *tunDevice) Write(p []byte) (int, error) {
	t.wMu.Lock()
	defer t.wMu.Unlock()
	t.wBufs[0] = p
	return t.nt.Write(t.wBufs, 0)
}

func (t *tunDevice) Close() {
	t.nt.Close()
	t.Endpoint.Close()
}
