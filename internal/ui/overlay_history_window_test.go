package ui

import (
	"fmt"
	"strings"
	"testing"

	"github.com/charmbracelet/x/ansi"
)

func makeHistoryItems(n int) []string {
	items := make([]string, n)
	for i := range items {
		items[i] = fmt.Sprintf(".q%02d", i)
	}
	return items
}

func TestOverlayHistoryKeepsSelectedItemVisible(t *testing.T) {
	items := makeHistoryItems(15)

	t.Run("empty base with selection beyond index 9", func(t *testing.T) {
		m := Model{
			mode:         ModeHistory,
			width:        80,
			height:       24,
			historyItems: items,
			historyIdx:   12,
		}
		got := m.overlayHistory("")
		if !strings.Contains(got, "→ .q12") {
			t.Fatalf("overlayHistory(\"\") omitted selected item; got:\n%s", got)
		}
		if strings.Contains(got, ".q00") {
			t.Fatalf("overlayHistory(\"\") should have slid past .q00; got:\n%s", got)
		}
	})

	t.Run("full screen base slides window around selection", func(t *testing.T) {
		base := strings.Repeat(strings.Repeat(" ", 80)+"\n", 23) + strings.Repeat(" ", 80)

		cases := []struct {
			name        string
			idx         int
			wantSel     string
			wantPresent []string
			wantAbsent  []string
		}{
			{
				name:        "top of list",
				idx:         0,
				wantSel:     "→ .q00",
				wantPresent: []string{".q09", "...+5 more"},
				wantAbsent:  []string{".q10"},
			},
			{
				name:        "middle beyond index 9",
				idx:         12,
				wantSel:     "→ .q12",
				wantPresent: []string{".q03", "...+2 more"},
				wantAbsent:  []string{".q00", ".q02", ".q13"},
			},
			{
				name:        "last item",
				idx:         14,
				wantSel:     "→ .q14",
				wantPresent: []string{".q05"},
				wantAbsent:  []string{".q04", "more"},
			},
		}

		for _, tc := range cases {
			t.Run(tc.name, func(t *testing.T) {
				m := Model{
					mode:         ModeHistory,
					width:        80,
					height:       24,
					historyItems: items,
					historyIdx:   tc.idx,
				}
				got := m.overlayHistory(base)
				plain := ansi.Strip(got)
				if !strings.Contains(plain, tc.wantSel) {
					t.Fatalf("missing selected item %q; got:\n%s", tc.wantSel, plain)
				}
				for _, s := range tc.wantPresent {
					if !strings.Contains(plain, s) {
						t.Fatalf("missing expected %q; got:\n%s", s, plain)
					}
				}
				for _, s := range tc.wantAbsent {
					if strings.Contains(plain, s) {
						t.Fatalf("unexpected %q; got:\n%s", s, plain)
					}
				}
				lines := strings.Split(got, "\n")
				if len(lines) != m.height {
					t.Fatalf("overlay lines = %d, want %d", len(lines), m.height)
				}
			})
		}
	})

	t.Run("short terminal keeps selected item visible", func(t *testing.T) {
		m := Model{
			mode:         ModeHistory,
			width:        80,
			height:       12,
			historyItems: items,
			historyIdx:   12,
		}
		base := strings.Repeat("\n", 11)
		got := m.overlayHistory(base)
		if !strings.Contains(got, "→ .q12") {
			t.Fatalf("short terminal overlayHistory omitted selected item; got:\n%s", got)
		}
	})
}
