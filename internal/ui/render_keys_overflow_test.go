package ui

import (
	"fmt"
	"strings"
	"testing"
)

func TestRenderSuggestionsKeysPaneFitsAndShowsOverflow(t *testing.T) {
	m := newTestModel(t, 120, 13) // Five content rows.
	var keys []string
	for i := 0; i < 20; i++ {
		keys = append(keys, fmt.Sprintf("key%02d", i))
	}
	m.keys = keysState{status: keysLoaded, path: m.currentPath(), keys: keys, lower: keys}
	lines := m.renderSuggestions()
	if len(lines) > m.contentHeight() {
		t.Fatalf("renderSuggestions returned %d lines for %d rows; overflow marker is cut off:\n%s",
			len(lines), m.contentHeight(), strings.Join(lines, "\n"))
	}
	if !strings.Contains(lines[len(lines)-1], "more") {
		t.Fatalf("last visible row should be the overflow marker, got %q", lines[len(lines)-1])
	}

	keys = keys[:4]
	m.keys = keysState{status: keysLoaded, path: m.currentPath(), keys: keys, lower: keys}
	lines = m.renderSuggestions()
	if len(lines) != 5 {
		t.Fatalf("renderSuggestions returned %d lines for exactly fitting keys, want 5:\n%s", len(lines), strings.Join(lines, "\n"))
	}
	if strings.Contains(strings.Join(lines, "\n"), "+0 more") {
		t.Fatalf("renderSuggestions emitted a zero-count overflow marker:\n%s", strings.Join(lines, "\n"))
	}
}
