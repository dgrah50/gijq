package autocomplete

import "testing"

func TestParseTrailingWhitespaceAfterPipeApply(t *testing.T) {
	filter := ".a | .b   "
	ctx := Parse(filter)
	if ctx.StartPos != 6 {
		t.Fatalf("StartPos = %d, want 6", ctx.StartPos)
	}

	got := NewService(nil).Apply(filter, ctx, "bar")
	if want := ".a | .bar"; got != want {
		t.Fatalf("Apply() = %q, want %q", got, want)
	}
}
