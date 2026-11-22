package autocomplete

import "testing"

func TestApplyQuotesSpecialKeysOnlyForFieldAccess(t *testing.T) {
	svc := NewService(nil)
	tests := []struct {
		filter   string
		selected string
		want     string
	}{
		{".foo", "foo-bar", `."foo-bar"`},
		{".foo", "foo bar", `."foo bar"`},
		{".foo", "123abc", `."123abc"`},
		{".foo", `a"b`, `."a\"b"`},
		{".foo", string([]byte{'x', 0xff}), ".\"x\xff\""},
		{".foo", "name", ".name"},
		{".items.", "[0]", ".items.[0]"},
		{".a | ", "foo-bar", ".a | foo-bar"},
		{".a | ", "select()", ".a | select()"},
		{"foo", "foo-bar", "foo-bar"},
		{`."fo`, "foo-bar", `."foo-bar"`},
	}

	for _, tt := range tests {
		t.Run(tt.filter+"/"+tt.selected, func(t *testing.T) {
			if got := svc.Apply(tt.filter, Parse(tt.filter), tt.selected); got != tt.want {
				t.Errorf("Apply(%q, %q) = %q, want %q", tt.filter, tt.selected, got, tt.want)
			}
		})
	}
}
