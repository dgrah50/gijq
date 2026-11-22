package autocomplete

import "testing"

func TestApplySpecialKeysAndArrayHints(t *testing.T) {
	tests := []struct {
		filter   string
		selected string
		want     string
	}{
		{".foo", "foo-bar", `."foo-bar"`},
		{".foo", "foo bar", `."foo bar"`},
		{".foo", "123abc", `."123abc"`},
		{".foo", "café", `."café"`},
		{".foo", "名前", `."名前"`},
		{".foo", `a"b`, `."a\"b"`},
		{".foo", `a\\b`, `."a\\\\b"`},
		{".foo", "a&b", `."a&b"`},
		{".foo", "", `.""`},
		{".foo", string([]byte{'x', 0xff}), ".x\xff"},
		{".items.", "[0]", ".items[0]"},
		{".a | foo", "foo-bar", `.a | ."foo-bar"`},
		{".foo", "_x1", "._x1"},
		{".foo", "name", ".name"},
	}

	svc := NewService(nil)
	for _, tt := range tests {
		t.Run(tt.filter+"/"+tt.selected, func(t *testing.T) {
			if got := svc.Apply(tt.filter, Parse(tt.filter), tt.selected); got != tt.want {
				t.Errorf("Apply(%q, %q) = %q, want %q", tt.filter, tt.selected, got, tt.want)
			}
		})
	}
}
