package ui

import (
	"strings"
	"testing"

	"github.com/mattn/go-runewidth"
)

func TestClipRawLine(t *testing.T) {
	line := "abcdefghijklmnopqrstuvwxyz"
	got, left, right := clipRawLine(line, 5, 8)
	if got != "fghijklm" {
		t.Fatalf("clipRawLine() got %q, want %q", got, "fghijklm")
	}
	if !left || !right {
		t.Fatalf("clip flags got left=%v right=%v, want true,true", left, right)
	}
}

func TestWithEllipsisWidth(t *testing.T) {
	got := withEllipsis("abcdefgh", 6, true, true)
	if runewidth.StringWidth(got) != 6 {
		t.Fatalf("withEllipsis width = %d, want 6 (%q)", runewidth.StringWidth(got), got)
	}
	if !strings.HasPrefix(got, "…") || !strings.HasSuffix(got, "…") {
		t.Fatalf("withEllipsis missing markers: %q", got)
	}
}

func TestShowSuggestionPane(t *testing.T) {
	m := Model{width: 50}
	if m.showSuggestionPane() {
		t.Fatal("showSuggestionPane() = true, want false for narrow width")
	}

	m.width = 120
	if !m.showSuggestionPane() {
		t.Fatal("showSuggestionPane() = false, want true for wide width")
	}
}
