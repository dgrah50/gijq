package autocomplete

import "testing"

func TestParseIgnoresDotsInsideQuotedStringsAndNumbers(t *testing.T) {
	t.Run("nested key", func(t *testing.T) {
		ctx := Parse(".user.na")
		if ctx.Path != ".user" || ctx.Incomplete != "na" || ctx.StartPos != 6 {
			t.Fatalf("Parse() = %+v, want {Path:.user Incomplete:na StartPos:6}", ctx)
		}
	})

	t.Run("quoted decimal", func(t *testing.T) {
		ctx := Parse(`.foo == "1.2"`)
		if ctx.Incomplete == `2"` {
			t.Fatalf("Parse() split inside quoted decimal: %+v", ctx)
		}
	})

	t.Run("float literal", func(t *testing.T) {
		ctx := Parse(`.foo == 1.5`)
		if ctx.Incomplete == "5" {
			t.Fatalf("Parse() split inside float literal: %+v", ctx)
		}
	})
}
