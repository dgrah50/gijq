package jq

import "testing"

func TestKeysAtIterOverObject(t *testing.T) {
	svc, err := NewService([]byte(`{"a":{"k":{"x":1,"y":2}}}`))
	if err != nil {
		t.Fatal(err)
	}
	for _, test := range []struct {
		path string
		want []string
	}{
		{path: ".[]", want: []string{"k"}},
		{path: ".a[]", want: []string{"x", "y"}},
	} {
		path := test.path
		got, err := svc.KeysAt(path)
		if err != nil {
			t.Fatalf("KeysAt(%q): %v", path, err)
		}
		if len(got) != len(test.want) {
			t.Errorf("KeysAt(%q) = %v, want %v", path, got, test.want)
			continue
		}
		for i := range test.want {
			if got[i] != test.want[i] {
				t.Errorf("KeysAt(%q) = %v, want %v", path, got, test.want)
				break
			}
		}
	}
}
