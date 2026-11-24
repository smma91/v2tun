//go:build linux

package main

import (
	"errors"
	"path/filepath"
)

// NetAdapter is a real network connection (the DNS card in Settings). On
// Linux the connection's DNS belongs to NetworkManager / systemd-resolved;
// v2tun does not change it (DNS is captured into the tunnel instead), so the
// list is empty and the card is hidden.
type NetAdapter struct {
	Name        string   `json:"name"`
	Description string   `json:"description"`
	Index       uint32   `json:"index"`
	GUID        string   `json:"guid"`
	DNS4        []string `json:"dns4"`
	DNS6        []string `json:"dns6"`
	Static      bool     `json:"static"`
	Preset      string   `json:"preset"`
	Uplink      bool     `json:"uplink"`
	HasBackup   bool     `json:"hasBackup"`
}

func backupPath() string { return filepath.Join(dataDir(), "dns-backup.json") }

func internetAdapters() ([]NetAdapter, error) { return nil, nil }

func applyAdapterDNS(index uint32, preset string) error {
	return errors.New("not supported on Linux")
}
