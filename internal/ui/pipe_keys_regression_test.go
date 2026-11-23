package ui

import (
	"context"
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
	m.keys = keysState{}
	for _, tt := range []struct {
		filter string
		want   []string
		err    bool
	}{
		{"", []string{"users"}, false},
		{".", []string{"users"}, false},
		{".u", []string{"users"}, false},
		{".users[] | .", []string{"name", "role"}, false},
		{".users[] | .na", []string{"name"}, false},
		{".users[0].name | .", nil, false},
		{".[broken | .", nil, true},
	} {
		m.setFilter(tt.filter)
		cmd := m.fetchKeysIfNeeded()
		if cmd == nil {
			if tt.filter != "." && tt.filter != ".u" && tt.filter != ".users[] | .na" {
				t.Fatalf("fetchKeysIfNeeded(%q) returned nil", tt.filter)
			}
			continue
		}
		gotMsg := cmd().(keysMsg)
		updated, _ := m.Update(gotMsg)
		m = updated.(Model)
		if (gotMsg.err != nil) != tt.err {
			t.Fatalf("fetchKeys(%q) error = %v, want error %v", tt.filter, gotMsg.err, tt.err)
		}
		if tt.want == nil {
			if !tt.err && gotMsg.keys != nil {
				t.Fatalf("scalar pipe keys = %v, want none", gotMsg.keys)
			}
			continue
		}
		if gotMsg.err != nil || !reflect.DeepEqual(gotMsg.keys, tt.want) {
			t.Fatalf("keys pane for %q = %v, %v; want %v", tt.filter, gotMsg.keys, gotMsg.err, tt.want)
		}
	}

	filter := ".users[] | ."
	got, err := m.autocomplete.KeysFor(context.Background(), filter)
	if err != nil || !reflect.DeepEqual(got, []string{"name", "role"}) {
		t.Fatalf("KeysFor = %v, %v", got, err)
	}
	if got, _ := m.autocomplete.Suggest(filter); !reflect.DeepEqual(got, []string{"name", "role"}) {
		t.Fatalf("Suggest = %v, want [name role]", got)
	}
}
