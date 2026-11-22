package autocomplete

import (
	"testing"

	"github.com/dayangraham/gijq/internal/jq"
)

func TestParseWhitespaceWithoutPipe(t *testing.T) {
	ctx := Parse("  .a.b")
	if ctx.Path != ".a" || ctx.Incomplete != "b" || ctx.StartPos != 5 {
		t.Fatalf("Parse(leading ws) = %+v, want {.a b 5}", ctx)
	}
	if got := NewService(nil).Apply("  .a.b", ctx, "bar"); got != "  .a.bar" {
		t.Fatalf("Apply = %q, want %q", got, "  .a.bar")
	}

	ctx = Parse(".a.b  ")
	if ctx.Path != ".a" || ctx.Incomplete != "b" || ctx.StartPos != 3 {
		t.Fatalf("Parse(trailing ws) = %+v, want {.a b 3}", ctx)
	}
}

func TestSuggestTrailingWhitespaceWithoutPipe(t *testing.T) {
	svc, err := jq.NewService([]byte(`{"a":{"bar":1,"baz":2}}`))
	if err != nil {
		t.Fatal(err)
	}
	got, _ := NewService(svc).Suggest(".a.b  ")
	if len(got) != 2 {
		t.Fatalf("Suggest = %v, want [bar baz]", got)
	}
}
