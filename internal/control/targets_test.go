package control

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/everlst/web-terminal/internal/config"
)

func TestInternalLabel(t *testing.T) {
	if !hasInternalLabel("com.example=x,com.evlst.web-terminal.internal=true") {
		t.Fatal("expected internal label")
	}
	if hasInternalLabel("com.evlst.web-terminal.internal=false") {
		t.Fatal("did not expect internal label")
	}
}

func TestHostUserAutoDetection(t *testing.T) {
	root := t.TempDir()
	if err := os.MkdirAll(filepath.Join(root, "etc"), 0o755); err != nil {
		t.Fatal(err)
	}
	passwd := "root:x:0:0:root:/root:/bin/sh\n" +
		"service:x:999:999:service:/var/empty:/sbin/nologin\n" +
		"nasuser:x:1026:100:NAS User:/volume1/homes/nasuser:/bin/sh\n"
	if err := os.WriteFile(filepath.Join(root, "etc/passwd"), []byte(passwd), 0o644); err != nil {
		t.Fatal(err)
	}
	account, err := validateHostAccount(config.Agent{HostRoot: root})
	if err != nil {
		t.Fatal(err)
	}
	if account.Name != "nasuser" || account.UID != 1026 {
		t.Fatalf("unexpected detected account: %+v", account)
	}
}
