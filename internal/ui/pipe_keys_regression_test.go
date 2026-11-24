package ui

import (
	"path/filepath"
	"reflect"
	"testing"

	"github.com/dayangraham/gijq/internal/autocomplete"
	"github.com/dayangraham/gijq/internal/clipboard"
	"github.com/dayangraham/gijq/internal/history"
	"github.com/dayangraham/gijq/internal/jq"
)

func TestKeysPaneUsesFilterContext(t *testing.T) {
	jqSvc, err := jq.NewService([]byte(`{"users":[{"name":"alice","role":"admin"}]}`))
	if err != nil {
		t.Fatal(err)
	}
	hist, err := history.NewStore(filepath.Join(t.TempDir(), "history.json"))
	if err != nil {
		t.Fatal(err)
	}
	m := NewModel(jqSvc, autocomplete.NewService(jqSvc), hist, clipboard.NewService(), Config{})
	// Supersede a pipe request with a root request, then deliver the pipe
	// result late. The pane must stay attached to the newer path.
	m.setFilter(".users[] | .")
	staleCmd := m.fetchKeysIfNeeded()
	m.setFilter(".")
	rootCmd := m.fetchKeysIfNeeded()
	staleUpdate, _ := m.Update(staleCmd().(keysMsg))
	m = staleUpdate.(Model)
	if m.keys.status != keysLoading || m.keys.path != m.currentPath() {
		t.Fatalf("stale pipe result changed pending root request: %+v", m.keys)
	}
	rootUpdate, _ := m.Update(rootCmd().(keysMsg))
	m = rootUpdate.(Model)
	for _, tt := range []struct {
		filter string
		want   []string
		shown  []string
	}{
		{".users[] | .", []string{"name", "role"}, []string{"name", "role"}},
		{".users[] | .na", []string{"name", "role"}, []string{"name"}},
		{".users[0].name | .", []string{"users"}, []string{"users"}},
		{"| .", []string{"users"}, []string{"users"}},
		{".[broken | .", []string{"users"}, []string{"users"}},
		{".", []string{"users"}, []string{"users"}},
		{".u", []string{"users"}, []string{"users"}},
	} {
		m.setFilter(tt.filter)
		cmd := m.fetchKeysIfNeeded()
		if cmd != nil {
			gotMsg := cmd().(keysMsg)
			updated, _ := m.Update(gotMsg)
			m = updated.(Model)
		}
		if m.keys.status != keysLoaded || !reflect.DeepEqual(m.keys.keys, tt.want) || !reflect.DeepEqual(m.keys.filtered(m.acContext().Incomplete), tt.shown) {
			t.Fatalf("keys pane for %q = status %v, keys %v, shown %v; want loaded, %v, %v", tt.filter, m.keys.status, m.keys.keys, m.keys.filtered(m.acContext().Incomplete), tt.want, tt.shown)
		}
		if got, _ := m.autocomplete.Suggest(tt.filter); !reflect.DeepEqual(got, tt.shown) {
			t.Fatalf("Suggest(%q) = %v, want pane keys %v", tt.filter, got, tt.shown)
		}
	}
}
