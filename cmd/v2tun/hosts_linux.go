//go:build linux

package main

import "net/netip"

// On Linux the proxy servers' names need no pinning: a core resolving its
// server gets a FakeIP address, connects to it through the tunnel, and the
// tunnel resolves the real address outside and connects directly (cores are
// recognised by their program path). Nothing is written to /etc/hosts.
func pinHosts(pins map[string][]netip.Addr) error { return nil }

func unpinHosts() error { return nil }
