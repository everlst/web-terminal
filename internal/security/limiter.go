package security

import (
	"sync"
	"time"
)

type attemptWindow struct {
	failures []time.Time
}

type LoginLimiter struct {
	mu        sync.Mutex
	attempts  map[string]*attemptWindow
	max       int
	window    time.Duration
	baseDelay time.Duration
}

func NewLoginLimiter(max int, window, baseDelay time.Duration) *LoginLimiter {
	return &LoginLimiter{
		attempts:  make(map[string]*attemptWindow),
		max:       max,
		window:    window,
		baseDelay: baseDelay,
	}
}

func (l *LoginLimiter) Check(ip string, now time.Time) (bool, time.Duration) {
	l.mu.Lock()
	defer l.mu.Unlock()
	entry := l.prune(ip, now)
	if len(entry.failures) >= l.max {
		remaining := entry.failures[0].Add(l.window).Sub(now)
		if remaining < 0 {
			remaining = 0
		}
		return false, remaining
	}
	return true, 0
}

func (l *LoginLimiter) Failure(ip string, now time.Time) time.Duration {
	l.mu.Lock()
	defer l.mu.Unlock()
	entry := l.prune(ip, now)
	entry.failures = append(entry.failures, now)
	delay := l.baseDelay << max(0, len(entry.failures)-1)
	if delay > 4*time.Second {
		delay = 4 * time.Second
	}
	return delay
}

func (l *LoginLimiter) Success(ip string) {
	l.mu.Lock()
	delete(l.attempts, ip)
	l.mu.Unlock()
}

func (l *LoginLimiter) prune(ip string, now time.Time) *attemptWindow {
	entry := l.attempts[ip]
	if entry == nil {
		entry = &attemptWindow{}
		l.attempts[ip] = entry
	}
	cutoff := now.Add(-l.window)
	kept := entry.failures[:0]
	for _, failure := range entry.failures {
		if failure.After(cutoff) {
			kept = append(kept, failure)
		}
	}
	entry.failures = kept
	return entry
}
