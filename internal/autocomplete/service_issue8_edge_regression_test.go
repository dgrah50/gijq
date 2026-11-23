package autocomplete

import (
	"strings"
	"testing"

	"github.com/dayangraham/gijq/internal/jq"
)

func TestApplyQuotesObjectKeysAndPreservesArrayHints(t *testing.T) {
	tests := []struct {
		selected string
		want     string
	}{
		{"foo-bar", `."foo-bar"`},
		{"foo bar", `."foo bar"`},
		{"123abc", `."123abc"`},
		{"name", ".name"},
		{"[draft]", `."[draft]"`},
		{"[0", `."[0"`},
		{"a\"b", `."a\"b"`},
		{"a\\b", `."a\\b"`},
		{"a\\(b)", `."a\\(b)"`},
		{"a\tb", `."a\u0009b"`},
		{"café", `."café"`},
		{"日本", `."日本"`},
		{"", `.""`},
		{"[0]", ".[0]"},
	}
	svc := NewService(nil)
	for _, tt := range tests {
		t.Run(tt.selected, func(t *testing.T) {
			if got := svc.Apply(".key", Parse(".key"), tt.selected); got != tt.want {
				t.Errorf("Apply = %q, want %q", got, tt.want)
			}
		})
	}
}

func TestQuotedKeyCompletionRoundTrip(t *testing.T) {
	data := []byte(`{"a.b":{"child":1},"a|b":{"child":1},"a b":{"child":1},"a\"b":{"child":1},"[0":{"child":1}}`)
	jqSvc, err := jq.NewService(data)
	if err != nil {
		t.Fatal(err)
	}
	svc := NewService(jqSvc)
	for _, key := range []string{"a.b", "a|b", "a b", `a"b`, "[0"} {
		t.Run(key, func(t *testing.T) {
			filter := svc.Apply(".", Parse("."), key) + "."
			got, ctx := svc.Suggest(filter)
			if ctx.Path != filter[:len(filter)-1] || len(got) != 1 || got[0] != "child" {
				t.Fatalf("Suggest(%q) = (%v, %+v), want [child] under quoted key", filter, got, ctx)
			}
			if applied := svc.Apply(filter, ctx, got[0]); applied != filter+"child" {
				t.Errorf("Apply after quoted key = %q, want %q", applied, filter+"child")
			}
		})
	}
}

func TestApplyNonIdentifierAtKeyCompletionContexts(t *testing.T) {
	svc := NewService(nil)
	for _, filter := range []string{"", ".", ".a | .", ".users[]."} {
		ctx := Parse(filter)
		if ctx.StartPos > 0 && filter[ctx.StartPos-1] != '.' {
			t.Fatalf("Parse(%q) StartPos=%d is not after a dot", filter, ctx.StartPos)
		}
		want := filter[:ctx.StartPos] + `"foo-bar"`
		if !strings.HasSuffix(filter[:ctx.StartPos], ".") {
			want = filter[:ctx.StartPos] + `."foo-bar"`
		}
		if got := svc.Apply(filter, ctx, "foo-bar"); got != want {
			t.Errorf("Apply(%q) = %q", filter, got)
		}
	}
}
