//go:build linux

package main

import (
	"fmt"
	"net"
	"os"
	"path/filepath"

	"github.com/xjasonlyu/tun2socks/v2/core/device"
	"github.com/xjasonlyu/tun2socks/v2/core/device/tun"
)

const tunName = "v2tun"

// tunDevice is a /dev/net/tun interface exposed as a gVisor link endpoint.
// The kernel removes the interface and its routes when it is closed, also
// when the process is killed.
type tunDevice struct {
	device.Device
	index int
}

func openTun(mtu int) (*tunDevice, error) {
	d, err := tun.Open(tunName, uint32(mtu))
	if err != nil {
		return nil, fmt.Errorf("create tun interface: %w", err)
	}
	i, err := net.InterfaceByName(tunName)
	if err != nil {
		d.Close()
		return nil, err
	}
	return &tunDevice{Device: d, index: i.Index}, nil
}

// LUID: the interface index (the Windows code identifies adapters by LUID).
func (t *tunDevice) LUID() uint64 { return uint64(t.index) }

func (t *tunDevice) Close() { t.Device.Close() }

// ensureCronet: NaiveProxy needs libcronet.so next to the program (it is
// shipped in the packages).
func ensureCronet() error {
	exe, err := os.Executable()
	if err != nil {
		return err
	}
	if _, err := os.Stat(filepath.Join(filepath.Dir(exe), "libcronet.so")); err != nil {
		return fmt.Errorf("libcronet.so not found next to %s", exe)
	}
	return nil
}
