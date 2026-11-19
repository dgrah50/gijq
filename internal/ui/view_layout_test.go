package ui

import (
	"strings"
	"testing"

	"github.com/mattn/go-runewidth"
)

func TestHighlightWindowEllipsis(t *testing.T) {
	line := "abcdefghijklmnopqrstuvwxyz"
	got, w, left, right := highlightWindow(line, 5, 8, false, nil)
	if got != "…fghijk…" || w != 8 {
		t.Fatalf("got %q w=%d, want %q w=8", got, w, "…fghijk…")
	}
	if !left || !right {
		t.Fatalf("cut flags got left=%v right=%v, want true,true", left, right)
	}
	if runewidth.StringWidth(got) != w || !strings.HasPrefix(got, "…") || !strings.HasSuffix(got, "…") {
		t.Fatalf("bad markers/width: %q", got)
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
