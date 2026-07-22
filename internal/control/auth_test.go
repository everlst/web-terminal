package control

import (
	"net/http/httptest"
	"testing"
	"time"
)

func TestAgentHMACAndReplayProtection(t *testing.T) {
	auth := newAuthenticator([]byte("0123456789abcdef0123456789abcdef"))
	now := time.Now()
	header, err := auth.Headers("GET", "/v1/targets", now)
	if err != nil {
		t.Fatal(err)
	}
	req := httptest.NewRequest("GET", "http://unix/v1/targets", nil)
	req.Header = header
	if err := auth.Verify(req, now); err != nil {
		t.Fatalf("expected valid signature: %v", err)
	}
	if err := auth.Verify(req, now); err == nil {
		t.Fatal("expected replay to fail")
	}
}
