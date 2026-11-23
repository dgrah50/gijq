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
		shown  []string
		err    bool
	}{
		{"", []string{"users"}, []string{"users"}, false},
		{".", []string{"users"}, []string{"users"}, false},
		{".u", []string{"users"}, []string{"users"}, false},
		{".users[] | .", []string{"name", "role"}, []string{"name", "role"}, false},
		{".users[] | .na", []string{"name", "role"}, []string{"name"}, false},
		{".users[0].name | .", nil, nil, false},
		{".[broken | .", nil, nil, true},
	} {
		m.keys = keysState{}
		m.setFilter(tt.filter)
		cmd := m.fetchKeysIfNeeded()
		if cmd == nil {
			t.Fatalf("fetchKeysIfNeeded(%q) returned nil", tt.filter)
		}
		gotMsg := cmd().(keysMsg)
		updated, _ := m.Update(gotMsg)
		m = updated.(Model)
		if (gotMsg.err != nil) != tt.err {
			t.Fatalf("fetchKeys(%q) error = %v, want error %v", tt.filter, gotMsg.err, tt.err)
		}
		if gotMsg.err != nil || !reflect.DeepEqual(gotMsg.keys, tt.want) {
		}
		if tt.err {
			if m.keys.status != keysFailed {
				t.Fatalf("keys pane status for %q = %v, want failed", tt.filter, m.keys.status)
			}
			continue
		}
		if m.keys.status != keysLoaded || !reflect.DeepEqual(m.keys.keys, tt.want) || !reflect.DeepEqual(m.keys.filtered(m.acContext().Incomplete), tt.shown) {
			t.Fatalf("keys pane for %q = status %v, keys %v, shown %v; want loaded, %v, %v", tt.filter, m.keys.status, m.keys.keys, m.keys.filtered(m.acContext().Incomplete), tt.want, tt.shown)
		}
	}

	if got, want := m.autocomplete.KeysContext(".users[] | .na"), m.autocomplete.KeysContext(".users[] | ."); got != want {
		t.Fatalf("pipe context = %q, want %q", got, want)
	}

	filter := ".users[] | ."
	if got, _ := m.autocomplete.Suggest(filter); !reflect.DeepEqual(got, []string{"name", "role"}) {
		t.Fatalf("Suggest = %v, want [name role]", got)
	}
	for _, filter := range []string{"| .", ".users[0].name | ."} {
		if got, _ := m.autocomplete.Suggest(filter); !reflect.DeepEqual(got, []string{"users"}) {
			t.Fatalf("Suggest(%q) = %v, want root fallback [users]", filter, got)
		}
	}
}

func TestKeysPaneIgnoresCancelledABAResult(t *testing.T) {
	var k keysState
	k.request("A")
	first := k.seq
	k.request("B")
	k.request("A")
	third := k.seq
	k.receive("A", nil, context.Canceled, first)
	if k.status != keysLoading || k.seq != third {
		t.Fatalf("stale A result changed current request: %+v", k)
	}
	k.receive("A", []string{"current"}, nil, third)
	if k.status != keysLoaded || !reflect.DeepEqual(k.keys, []string{"current"}) {
		t.Fatalf("current A result not applied: %+v", k)
	}
}
