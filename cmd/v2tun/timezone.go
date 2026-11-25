//go:build windows || linux

package main

import "sync"

// tzManager keeps the system time zone in line with the proxy exit, so the
// browser time zone does not contradict the IP location. Restored on stop.
type tzManager struct {
	orig string

	mu       sync.Mutex
	current  string
	restored bool // no more changes after Restore
}

func newTZManager() *tzManager {
	orig, err := getTZ()
	if err != nil {
		return nil
	}
	return &tzManager{orig: orig, current: orig}
}

func (m *tzManager) Update(logf func(string, string, ...any)) {
	info, err := exitInfo()
	if err != nil || info.Timezone == "" {
		return
	}
	target, ok := tzTarget(info.Timezone)
	if !ok {
		return
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.restored || target == m.current {
		return
	}
	if err := setTZ(target); err != nil {
		logf("warn", "time zone: %v", err)
		return
	}
	m.current = target
	logf("info", "Time zone → %s (reopen your browser)", info.Timezone)
}

func (m *tzManager) Restore(logf func(string, string, ...any)) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.restored = true
	if m.current == m.orig {
		return
	}
	if err := setTZ(m.orig); err == nil {
		m.current = m.orig
		logf("info", "Time zone restored to %s", m.orig)
	}
}
