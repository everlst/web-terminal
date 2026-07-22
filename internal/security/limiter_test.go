package security

import (
	"testing"
	"time"
)

func TestLoginLimiter(t *testing.T) {
	limiter := NewLoginLimiter(5, 15*time.Minute, time.Millisecond)
	now := time.Now()
	for index := 0; index < 5; index++ {
		if allowed, _ := limiter.Check("127.0.0.1", now); !allowed {
			t.Fatalf("attempt %d unexpectedly blocked", index)
		}
		limiter.Failure("127.0.0.1", now)
	}
	if allowed, retry := limiter.Check("127.0.0.1", now); allowed || retry <= 0 {
		t.Fatalf("expected block, allowed=%v retry=%v", allowed, retry)
	}
	if allowed, _ := limiter.Check("127.0.0.1", now.Add(16*time.Minute)); !allowed {
		t.Fatal("expected expired failures to be removed")
	}
}
