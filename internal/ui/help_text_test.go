package ui

import (
	"strings"
	"testing"
)

func TestCompactHelpText(t *testing.T) {
	narrow := Model{width: 30}
	help := narrow.compactHelpText()
	if help != "?: help" {
		t.Fatalf("narrow help = %q, want %q", help, "?: help")
	}

	wide := Model{width: 200}
	help = wide.compactHelpText()
	if !strings.Contains(help, "tab: autocomplete") {
		t.Fatalf("wide help missing expected token: %q", help)
	}
	if !strings.Contains(help, "shift+left/right: h-scroll") {
		t.Fatalf("wide help missing expected token: %q", help)
	}
}
