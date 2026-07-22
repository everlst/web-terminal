package security

import (
	"net/http/httptest"
	"testing"
)

func TestValidOrigin(t *testing.T) {
	req := httptest.NewRequest("POST", "https://terminal.example.com/api/auth/login", nil)
	req.Header.Set("Origin", "https://terminal.example.com")
	if !ValidOrigin(req) {
		t.Fatal("expected matching origin")
	}
	req.Header.Set("Origin", "https://attacker.example")
	if ValidOrigin(req) {
		t.Fatal("expected mismatching origin to fail")
	}
}

func TestValidOriginBehindCloudflare(t *testing.T) {
	req := httptest.NewRequest("POST", "http://web-terminal:3000/api/auth/login", nil)
	req.Host = "web-terminal:3000"
	req.Header.Set("Origin", "https://terminal.example.com")
	req.Header.Set("X-Forwarded-Host", "terminal.example.com")
	req.Header.Set("X-Forwarded-Proto", "https")
	if !ValidOrigin(req) {
		t.Fatal("expected forwarded origin to match")
	}
	if !IsHTTPS(req) {
		t.Fatal("expected forwarded request to be considered HTTPS")
	}
}

func TestValidOriginOnLAN(t *testing.T) {
	req := httptest.NewRequest("POST", "http://192.168.1.20:3000/api/auth/login", nil)
	req.Header.Set("Origin", "http://192.168.1.20:3000")
	if !ValidOrigin(req) {
		t.Fatal("expected LAN origin to match without configuration")
	}
	if IsHTTPS(req) {
		t.Fatal("expected LAN HTTP request not to be considered HTTPS")
	}
}
