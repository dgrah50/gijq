package clipboard

import (
	"os"

	"github.com/atotto/clipboard"
	"github.com/aymanbagabas/go-osc52/v2"
)

// Service handles clipboard operations with SSH fallback
type Service struct {
	useOSC52 bool
}

// NewService creates a clipboard service, auto-detecting SSH
func NewService() *Service {
	useOSC52 := os.Getenv("SSH_CLIENT") != "" || os.Getenv("SSH_TTY") != ""
	return &Service{useOSC52: useOSC52}
}

// Copy copies text to clipboard
func (s *Service) Copy(text string) error {
	if s.useOSC52 {
		seq := osc52.New(text)
		_, err := seq.WriteTo(os.Stderr)
		return err
	}
	return clipboard.WriteAll(text)
}

// IsSSH returns whether we're in an SSH session
func (s *Service) IsSSH() bool {
	return s.useOSC52
}
