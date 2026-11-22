package autocomplete

import "testing"

func TestParseIgnoresPipeInsideStringLiteral(t *testing.T) {
	ctx := Parse(`.a | select(.s == "x|y")`)
	if ctx.Path != "." || ctx.Incomplete != `select(.s == "x|y")` || ctx.StartPos != 5 {
		t.Fatalf("Parse() = %+v, want Path=%q Incomplete=%q StartPos=5", ctx, ".", `select(.s == "x|y")`)
	}
}
