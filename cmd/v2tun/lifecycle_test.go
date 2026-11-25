//go:build windows || linux

package main

import (
	"context"
	"errors"
	"fmt"
	"net"
	"net/netip"
	"sync/atomic"
	"testing"
	"time"

	M "github.com/xjasonlyu/tun2socks/v2/metadata"
)

func TestHandlerErrSeenPrune(t *testing.T) {
	h := &handler{logf: func(string, string, ...any) {}}
	old := time.Now().Add(-2 * time.Minute)
	h.errSeen = map[string]time.Time{}
	for i := range 300 {
		h.errSeen[fmt.Sprint("old", i)] = old
	}
	m := &M.Metadata{DstIP: netip.MustParseAddr("1.2.3.4"), DstPort: 443}
	h.dialFailed("TCP", m, false, errors.New("x"))
	if n := len(h.errSeen); n != 1 {
		t.Fatalf("after prune: %d entries, want 1", n)
	}
	logged := 0
	h.logf = func(string, string, ...any) { logged++ }
	h.dialFailed("TCP", m, false, errors.New("x"))
	if logged != 0 {
		t.Fatal("repeated failure logged again within a minute")
	}
}

type closeCount struct{ n *atomic.Int32 }

func (c closeCount) Close() error { c.n.Add(1); return nil }

func TestHandlerFlowCap(t *testing.T) {
	var logs atomic.Int32
	h := &handler{logf: func(string, string, ...any) { logs.Add(1) }}
	var closed atomic.Int32
	c := closeCount{&closed}
	for range maxFlows {
		if !h.acquireFlow(&h.tcpFlows, c) {
			t.Fatal("refused below the cap")
		}
	}
	for range 3 {
		if h.acquireFlow(&h.tcpFlows, c) {
			t.Fatal("accepted above the cap")
		}
	}
	if closed.Load() != 3 || logs.Load() != 1 || h.tcpFlows.Load() != maxFlows {
		t.Fatalf("closed=%d logs=%d flows=%d", closed.Load(), logs.Load(), h.tcpFlows.Load())
	}
	if !h.acquireFlow(&h.udpFlows, c) {
		t.Fatal("UDP refused because of TCP flows")
	}
	h.tcpFlows.Add(-1)
	if !h.acquireFlow(&h.tcpFlows, c) {
		t.Fatal("refused after a flow ended")
	}
}

func TestDNSTCPNoPoolAfterClose(t *testing.T) {
	c := newDNSTCP(&routerDialer{})
	c.close()
	a, b := net.Pipe()
	defer b.Close()
	c.put(netip.MustParseAddr("1.1.1.1"), a)
	if len(c.idle) != 0 {
		t.Fatal("connection pooled after close")
	}
	if _, err := a.Write([]byte{1}); err == nil {
		t.Fatal("connection not closed")
	}
}

// monitor must return (and signal done) as soon as its context is canceled,
// and pinServers must not change anything afterwards.
func TestMonitorStops(t *testing.T) {
	e := &Engine{logf: func(string, string, ...any) {}}
	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan struct{})
	go e.monitor(ctx, done, 0, &routerDialer{}, Settings{SocksAddr: "127.0.0.1:1"}, 0, nil)
	cancel()
	select {
	case <-done:
	case <-time.After(5 * time.Second):
		t.Fatal("monitor did not stop")
	}
	e.pinServers(ctx, uplink{}, []string{"example.com"})
	if e.servers != nil {
		t.Fatal("pinServers changed state after cancel")
	}
}
