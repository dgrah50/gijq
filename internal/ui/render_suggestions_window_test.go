package ui

import (
	"fmt"
	"strings"
	"testing"
)

func TestRenderSuggestionsKeepsSelectedItemVisible(t *testing.T) {
	m := Model{
		mode:        ModeAutocomplete,
		height:      13, // Five content rows.
		selectedIdx: 15,
	}
	for i := 0; i < 20; i++ {
		m.suggestions = append(m.suggestions, fmt.Sprintf("key%02d", i))
	}

	got := strings.Join(m.renderSuggestions(), "\n")
	if !strings.Contains(got, "→ key15") {
		t.Fatalf("renderSuggestions() omitted selected item; got:\n%s", got)
	}
}
