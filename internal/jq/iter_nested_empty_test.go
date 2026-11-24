package jq

import (
	"reflect"
	"testing"
)

func TestKeysAtNestedIterSkipsEmptyFirstElement(t *testing.T) {
	for _, tc := range []struct {
		input, path string
		want        []string
	}{
		{`[[], [{"x":1}]]`, ".[][]", []string{"x"}},
		{`{"a":{},"b":{"x":{"y":1}}}`, ".[][]", []string{"y"}},
		{`{"users":[{"tags":[]},{"tags":[{"name":"n"}]}]}`, ".users[].tags[]", []string{"name"}},
	} {
		svc, err := NewService([]byte(tc.input))
		if err != nil {
			t.Fatal(err)
		}
		got, err := svc.KeysAt(tc.path)
		if err != nil {
			t.Fatalf("KeysAt(%q): %v", tc.path, err)
		}
		if !reflect.DeepEqual(got, tc.want) {
			t.Errorf("input %s: KeysAt(%q) = %v, want %v", tc.input, tc.path, got, tc.want)
		}
	}
}
