package autocomplete

import "testing"

func TestParseIgnoresDotsInsideStringsAndNumbers(t *testing.T) {
	tests := []struct {
		filter string
		want   Context
	}{
		{filter: ".user.na", want: Context{Path: ".user", Incomplete: "na", StartPos: 6}},
		{filter: `.a == "x.y`, want: Context{Path: ".", Incomplete: `a == "x.y`, StartPos: 1}},
		{filter: ".n == 1.5", want: Context{Path: ".", Incomplete: "n == 1.5", StartPos: 1}},
	}

	for _, tt := range tests {
		t.Run(tt.filter, func(t *testing.T) {
			if got := Parse(tt.filter); got != tt.want {
				t.Errorf("Parse(%q) = %+v, want %+v", tt.filter, got, tt.want)
			}
		})
	}
}
