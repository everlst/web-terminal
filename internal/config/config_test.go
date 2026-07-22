package config

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/everlst/web-terminal/internal/security"
)

func TestLoadServerNeedsOnlyPassword(t *testing.T) {
	tokenPath := filepath.Join(t.TempDir(), "agent_token")
	if err := os.WriteFile(tokenPath, []byte("0123456789abcdef0123456789abcdef"), 0o600); err != nil {
		t.Fatal(err)
	}
	t.Setenv("WEB_TERMINAL_PASSWORD", "correct horse battery staple")
	t.Setenv("AGENT_TOKEN_FILE", tokenPath)

	cfg, err := LoadServer()
	if err != nil {
		t.Fatal(err)
	}
	valid, err := security.VerifyPassword(cfg.PasswordHash, "correct horse battery staple")
	if err != nil || !valid {
		t.Fatalf("expected configured password to verify, valid=%v err=%v", valid, err)
	}
	if cfg.Addr != ":3000" {
		t.Fatalf("unexpected default listen address: %s", cfg.Addr)
	}
}

func TestLoadServerRejectsMissingPassword(t *testing.T) {
	t.Setenv("WEB_TERMINAL_PASSWORD", "")
	if _, err := LoadServer(); err == nil {
		t.Fatal("expected missing password to fail")
	}
}
