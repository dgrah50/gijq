package autocomplete

import "testing"

func TestApplyQuotesNonIdentifierKeysAtKeyAccess(t *testing.T) {
	tests := []struct {
		filter, selected, want string
	}{
		{".foo", "foo-bar", `."foo-bar"`},
		{".foo", "foo bar", `."foo bar"`},
		{".foo", `a"b`, `."a\"b"`},
		{".foo", `a\b`, `."a\\b"`},
		{".foo", "name", ".name"},
		{".items", "[0]", ".[0]"},
		{".items", "[]", ".[]"},
		{".foo", "[draft] x", `."[draft] x"`},
		{".foo", "[foo]", `."[foo]"`},
		{".foo", "123", `."123"`},
		{".日本", "日本語キー", `."日本語キー"`},
		{`."日本" | .f`, "foo-bar", `."日本" | ."foo-bar"`},
		{".foo", "a.b|c", `."a.b|c"`},
		{"日本語", "日本語キー", `日本語キー`},
		{"", "foo-bar", `foo-bar`},
		{".foo | ", "foo-bar", `.foo | foo-bar`},
		{`."fo`, "foo-bar", `."foo-bar"`},
		{`."fo`, "foo", `.foo`},
		{`."`, "[]", `.[]`},
		{`.foo`, "line\nbreak", `."line\nbreak"`},
	}
	service := NewService(nil)
	for _, tt := range tests {
		t.Run(tt.selected, func(t *testing.T) {
			if got := service.Apply(tt.filter, Parse(tt.filter), tt.selected); got != tt.want {
				t.Errorf("Apply(%q, Parse(%q), %q) = %q, want %q", tt.filter, tt.filter, tt.selected, got, tt.want)
			}
		})
	}
}

func TestParseQuotedKeyForNextSuggestion(t *testing.T) {
	ctx := Parse(`."foo-bar".`)
	if ctx.Path != `."foo-bar"` || ctx.Incomplete != "" {
		t.Fatalf("Parse quoted key = %+v", ctx)
	}
	ctx = Parse(`."fo`)
	if ctx.Incomplete != "fo" || ctx.StartPos != 1 {
		t.Fatalf("Parse partial quoted key = %+v", ctx)
	}
}
