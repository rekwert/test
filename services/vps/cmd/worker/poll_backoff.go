package main

import (
	"sync"
	"time"
)

type pollBackoffTracker struct {
	mu   sync.Mutex
	until map[string]time.Time
	strikes map[string]int
}

func newPollBackoffTracker() *pollBackoffTracker {
	return &pollBackoffTracker{
		until:   make(map[string]time.Time),
		strikes: make(map[string]int),
	}
}

func (t *pollBackoffTracker) blocked(id string) bool {
	if t == nil || id == "" {
		return false
	}
	now := time.Now()
	t.mu.Lock()
	defer t.mu.Unlock()
	until, ok := t.until[id]
	return ok && until.After(now)
}

func (t *pollBackoffTracker) recordFailure(id string) {
	if t == nil || id == "" {
		return
	}
	const (
		base = 30 * time.Second
		max  = 5 * time.Minute
	)
	t.mu.Lock()
	defer t.mu.Unlock()
	n := t.strikes[id] + 1
	t.strikes[id] = n
	delay := base * time.Duration(n)
	if delay > max {
		delay = max
	}
	t.until[id] = time.Now().Add(delay)
}

func (t *pollBackoffTracker) clear(id string) {
	if t == nil || id == "" {
		return
	}
	t.mu.Lock()
	defer t.mu.Unlock()
	delete(t.until, id)
	delete(t.strikes, id)
}
