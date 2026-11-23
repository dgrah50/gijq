package ui

import (
	"reflect"
	"testing"

	"github.com/dayangraham/gijq/internal/autocomplete"
	"github.com/dayangraham/gijq/internal/jq"
)

func TestCurrentPathUsesAutocompletePipeContextAtCursor(t *testing.T) {
	jqSvc, err := jq.NewService([]byte(`{"users":[{"name":"alice","role":"admin"}],"config":{"theme":"dark"}}`))
	if err != nil {
		t.Fatal(err)
	}
	m := Model{jq: jqSvc, autocomplete: autocomplete.NewService(jqSvc)}
	tests := []struct {
		filter string
		cursor int
		want   string
	}{
		{filter: ".users[] | .name", cursor: len([]rune(".users")), want: "."},
		{filter: ".users[] | .name | .role", cursor: len([]rune(".users[] | .na")), want: ".users[] | ."},
		{filter: `."é"[] | .`, cursor: len([]rune(`."é"[] | .`)), want: `."é"[] | .`},
		{filter: ".config |= .theme", cursor: len([]rune(".config |= .theme")), want: "."},
	}
	for _, tt := range tests {
		m.setFilter(tt.filter)
		m.filter.SetCursor(tt.cursor)
		if got := m.currentPath(); got != tt.want {
			t.Errorf("currentPath(%q at %d) = %q, want %q", tt.filter, tt.cursor, got, tt.want)
		}
	}

	m.setFilter(".users[] | .")
	msg := m.fetchKeys(m.currentPath())().(keysMsg)
	if msg.err != nil {
		t.Fatal(msg.err)
	}
	if want := []string{"name", "role"}; !reflect.DeepEqual(msg.keys, want) {
		t.Fatalf("fetchKeys() = %v, want %v", msg.keys, want)
	}
}
