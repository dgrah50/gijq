package ui

import (
	"reflect"
	"testing"

	"github.com/dayangraham/gijq/internal/autocomplete"
	"github.com/dayangraham/gijq/internal/jq"
)

func TestFetchKeysResolvesPipe(t *testing.T) {
	jqSvc, err := jq.NewService([]byte(`{"users":[{"name":"alice","role":"admin"}]}`))
	if err != nil {
		t.Fatal(err)
	}
	m := Model{jq: jqSvc, autocomplete: autocomplete.NewService(jqSvc)}
	m.setFilter(".users[] | .")

	msg := m.fetchKeys(m.currentPath())().(keysMsg)
	if msg.err != nil {
		t.Fatal(msg.err)
	}
	if want := []string{"name", "role"}; !reflect.DeepEqual(msg.keys, want) {
		t.Fatalf("fetchKeys() = %v, want %v", msg.keys, want)
	}
}
