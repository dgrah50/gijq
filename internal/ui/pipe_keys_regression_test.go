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

func TestKeysPaneUsesPipedValue(t *testing.T) {
	jqSvc, err := jq.NewService([]byte(`{"users":[{"name":"alice","role":"admin"}]}`))
	if err != nil {
		t.Fatal(err)
	}
	hist, err := history.NewStore(filepath.Join(t.TempDir(), "history.json"))
	if err != nil {
		t.Fatal(err)
	}
	m := NewModel(jqSvc, autocomplete.NewService(jqSvc), hist, clipboard.NewService(), Config{})
	m.setFilter(".users[] | .")

	got := m.fetchKeys(m.currentPath())().(keysMsg).keys
	if want := []string{"name", "role"}; !reflect.DeepEqual(got, want) {
		t.Fatalf("keys pane = %v, want %v", got, want)
	}
}
