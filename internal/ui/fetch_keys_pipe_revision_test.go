package ui

import (
	"reflect"
	"testing"

	"github.com/dayangraham/gijq/internal/autocomplete"
	"github.com/dayangraham/gijq/internal/jq"
)

func TestFetchKeysPipeRevision(t *testing.T) {
	jqSvc, err := jq.NewService([]byte(`{"users":[{"name":"alice","role":"admin"}],"config":{"theme":"dark"}}`))
	if err != nil {
		t.Fatal(err)
	}
	m := Model{jq: jqSvc, autocomplete: autocomplete.NewService(jqSvc)}

	// Establish the already-loaded root state. Entering a pipe must change the
	// request identity, even though its right-hand path is still ".".
	if !m.keys.request(".") {
		t.Fatal("root keys request was not started")
	}
	m.keys.receive(".", []string{"config", "users"}, nil)
	m.setFilter(".")
	if cmd := m.fetchKeysIfNeeded(); cmd != nil {
		t.Fatal("unchanged root path unexpectedly requested keys")
	}
	m.setFilter(".users[] | .")
	cmd := m.fetchKeysIfNeeded()
	if cmd == nil {
		t.Fatal("changing the pipe prefix did not request keys")
	}
	msg := cmd().(keysMsg)
	if msg.path != ".users[] | ." {
		t.Fatalf("keys path = %q, want %q", msg.path, ".users[] | .")
	}
	if msg.err != nil {
		t.Fatal(msg.err)
	}
	if want := []string{"name", "role"}; !reflect.DeepEqual(msg.keys, want) {
		t.Fatalf("pipe keys = %v, want %v", msg.keys, want)
	}

	// The pane retains every sibling key while the user types a partial name.
	m.setFilter(".users[0].na")
	msg = m.fetchKeys(m.currentPath())().(keysMsg)
	if want := []string{"name", "role"}; !reflect.DeepEqual(msg.keys, want) {
		t.Fatalf("partial-path keys = %v, want %v", msg.keys, want)
	}
	m.setFilter(".users[] | .na")
	msg = m.fetchKeys(m.currentPath())().(keysMsg)
	if want := []string{"name", "role"}; !reflect.DeepEqual(msg.keys, want) {
		t.Fatalf("partial pipe keys = %v, want %v", msg.keys, want)
	}

	m.setFilter(".users[ | .")
	msg = m.fetchKeys(m.currentPath())().(keysMsg)
	if msg.err == nil {
		t.Fatal("invalid pipe prefix did not report an error")
	}
}
