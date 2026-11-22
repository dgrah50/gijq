package clipboard

import "testing"

func TestNewServiceDetectsSSHConnection(t *testing.T) {
	t.Setenv("SSH_CLIENT", "")
	t.Setenv("SSH_TTY", "")
	t.Setenv("SSH_CONNECTION", "10.0.0.1 12345 10.0.0.2 22")

	if !NewService().IsSSH() {
		t.Fatal("expected SSH_CONNECTION to enable SSH clipboard mode")
	}
}
