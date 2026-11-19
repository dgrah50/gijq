package ui

import (
	"strings"
	"testing"
)

func TestCompactHelpText(t *testing.T) {
	narrow := Model{width: 30}
	help := narrow.compactHelpText()
	if help != "f1: help" {
		t.Fatalf("narrow help = %q, want %q", help, "f1: help")
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
