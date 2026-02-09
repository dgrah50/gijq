package clipboard

import (
	"os"
	"testing"
)

func TestDetectSSH(t *testing.T) {
	// Save original env
	origClient := os.Getenv("SSH_CLIENT")
	origTTY := os.Getenv("SSH_TTY")
	defer func() {
		os.Setenv("SSH_CLIENT", origClient)
		os.Setenv("SSH_TTY", origTTY)
	}()

	// Test non-SSH
	os.Unsetenv("SSH_CLIENT")
	os.Unsetenv("SSH_TTY")
	svc := NewService()
	if svc.useOSC52 {
		t.Error("expected useOSC52=false without SSH env vars")
	}

	// Test with SSH_CLIENT
	os.Setenv("SSH_CLIENT", "192.168.1.1 12345 22")
	svc = NewService()
	if !svc.useOSC52 {
		t.Error("expected useOSC52=true with SSH_CLIENT")
	}

	// Test with SSH_TTY
	os.Unsetenv("SSH_CLIENT")
	os.Setenv("SSH_TTY", "/dev/pts/0")
	svc = NewService()
	if !svc.useOSC52 {
		t.Error("expected useOSC52=true with SSH_TTY")
	}
}
