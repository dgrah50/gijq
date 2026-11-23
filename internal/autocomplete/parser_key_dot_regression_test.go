package autocomplete

import "testing"

func TestFindLastKeyDotTracksQuotesDecimalsAndBrackets(t *testing.T) {
	tests := []struct {
		name string
		text string
		want int
	}{
		{name: "key access", text: ".user.na", want: 5},
		{name: "open double quote", text: `.a == "x.y`, want: 0},
		{name: "escaped quote", text: `.a == "x\".y`, want: 0},
		{name: "single quote", text: ".a == 'x.y", want: 0},
		{name: "decimal", text: ".n == 1.5", want: 0},
		{name: "closed quote then key", text: `.a == "x.y" and .user.na`, want: 21},
		{name: "unmatched close then key", text: ".a].b", want: 3},
		{name: "array index then key", text: ".b][0].na", want: 6},
		{name: "array key then key", text: ".b][.k].na", want: 7},
		{name: "unclosed bracket", text: ".items[.user.na", want: 0},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := findLastKeyDot(tt.text); got != tt.want {
				t.Errorf("findLastKeyDot(%q) = %d, want %d", tt.text, got, tt.want)
			}
		})
	}
}

func TestParseIgnoresDotsInsideStringsAndNumbers(t *testing.T) {
	tests := []struct {
		filter string
		want   Context
	}{
		{filter: ".user.na", want: Context{Path: ".user", Incomplete: "na", StartPos: 6}},
		{filter: `.a == "x.y`, want: Context{Path: ".", Incomplete: `a == "x.y`, StartPos: 1}},
		{filter: `.a == "x\".y`, want: Context{Path: ".", Incomplete: `a == "x\".y`, StartPos: 1}},
		{filter: ".a == 'x.y", want: Context{Path: ".", Incomplete: "a == 'x.y", StartPos: 1}},
		{filter: ".n == 1.5", want: Context{Path: ".", Incomplete: "n == 1.5", StartPos: 1}},
		{filter: `.a == "x.y" and .user.na`, want: Context{Path: `.a == "x.y" and .user`, Incomplete: "na", StartPos: 22}},
		{filter: `.x.a == 1.5`, want: Context{Path: ".x", Incomplete: "a == 1.5", StartPos: 3}},
		{filter: ".user ", want: Context{Path: ".", Incomplete: "user", StartPos: 1}},
		{filter: ".a == 1", want: Context{Path: ".", Incomplete: "a == 1", StartPos: 1}},
		{filter: ".items[.user.na", want: Context{Path: ".", Incomplete: "items[.user.na", StartPos: 1}},
	}

	for _, tt := range tests {
		t.Run(tt.filter, func(t *testing.T) {
			if got := Parse(tt.filter); got != tt.want {
				t.Errorf("Parse(%q) = %+v, want %+v", tt.filter, got, tt.want)
			}
		})
	}
}
