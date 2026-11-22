package autocomplete

import "testing"

func TestParseQuotedExpressions(t *testing.T) {
	tests := []struct {
		name   string
		filter string
		path   string
		part   string
	}{
		{"pipe in string", `.a | select(.s == "x|y")`, ".", `select(.s == "x|y")`},
		{"escaped quote", `.a | "a\"|b"`, ".", `"a\"|b"`},
		{"unterminated string", `.a | "x|`, ".", `"x|`},
		{"pipe after string", `.a | "x|y" | .fo`, ".", "fo"},
		{"pipe in interpolation", `.x | "\(.foo | .ba`, ".", "ba"},
		{"dot in string", `.a | .foo == "a.b`, ".", `foo == "a.b`},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ctx := Parse(tt.filter)
			if ctx.Path != tt.path || ctx.Incomplete != tt.part {
				t.Errorf("Parse(%q) = %+v, want Path=%q Incomplete=%q", tt.filter, ctx, tt.path, tt.part)
			}
		})
	}
}
