package ui

import (
	"fmt"
	"strings"
	"testing"

	"github.com/charmbracelet/x/ansi"
)

func TestOverlayHistoryBoundsSelectedItemAndHeight(t *testing.T) {
	items := make([]string, 15)
	for i := range items {
		items[i] = fmt.Sprintf(".q%02d", i)
	}
	t.Run("selection after tenth item remains visible", func(t *testing.T) {
		m := Model{width: 80, height: 24, historyItems: items, historyIdx: 12}
		got := ansi.Strip(m.overlayHistory(""))
		if !strings.Contains(got, "→ .q12") {
			t.Fatalf("selected history item missing from overlay:\n%s", got)
		}
	})

	t.Run("out of range selection is clamped", func(t *testing.T) {
		for _, idx := range []int{-1, len(items)} {
			m := Model{width: 80, height: 24, historyItems: items, historyIdx: idx}
			got := ansi.Strip(m.overlayHistory(""))
			want := "→ .q00"
			if idx >= len(items) {
				want = "→ .q14"
			}
			if !strings.Contains(got, want) {
				t.Errorf("historyIdx %d: missing %q from overlay:\n%s", idx, want, got)
			}
		}
	})

	t.Run("short terminal overlay fits its height", func(t *testing.T) {
		m := Model{width: 80, height: 12, historyItems: items, historyIdx: 12}
		got := m.overlayHistory(strings.Repeat("\n", 11))
		if lines := strings.Count(got, "\n") + 1; lines > m.height {
			t.Fatalf("overlay has %d lines, terminal height is %d", lines, m.height)
		}
		if !strings.Contains(ansi.Strip(got), "→ .q12") {
			t.Fatalf("selected item missing from short overlay:\n%s", ansi.Strip(got))
		}
	})
}
