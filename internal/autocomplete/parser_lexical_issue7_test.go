package autocomplete

import "testing"

func TestParseLexicalBoundaries(t *testing.T) {
	tests := []struct {
		name   string
		filter string
		want   Context
	}{
		{"nested key", ".user.na", Context{Path: ".user", Incomplete: "na", StartPos: 6}},
		{"quoted decimal in filter", `.items[] | select(.ver == "1.2")`, Context{Path: ".", Incomplete: `select(.ver == "1.2")`, StartPos: 11}},
		{"quoted nonnumeric dot", `.foo == "a.b"`, Context{Path: ".", Incomplete: `foo == "a.b"`, StartPos: 1}},
		{"escaped quote", `.foo == "a\"b.c"`, Context{Path: ".", Incomplete: `foo == "a\"b.c"`, StartPos: 1}},
		{"open quoted dot", `.a == "x.y`, Context{Path: ".", Incomplete: `a == "x.y`, StartPos: 1}},
		{"open float", `.a == 1.`, Context{Path: ".", Incomplete: `a == 1.`, StartPos: 1}},
		{"quoted pipe", `.a | select(.n == "x|y.z")`, Context{Path: ".", Incomplete: `select(.n == "x|y.z")`, StartPos: 5}},
		{"bracket in quoted path key", `.foo["x]y"].bar`, Context{Path: `.foo["x]y"]`, Incomplete: "bar", StartPos: 12}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := Parse(tt.filter); got != tt.want {
				t.Errorf("Parse(%q) = %+v, want %+v", tt.filter, got, tt.want)
			}
		})
	}
}
