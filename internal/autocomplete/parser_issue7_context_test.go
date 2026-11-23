package autocomplete

import "testing"

func TestParseKeepsQuotedAndNumericDotsInContext(t *testing.T) {
	tests := []struct {
		name   string
		filter string
		want   Context
	}{
		{"nested key", ".user.na", Context{Path: ".user", Incomplete: "na", StartPos: 6}},
		{"quoted decimal after pipe", `.items[] | select(.ver == "1.2")`, Context{Path: ".", Incomplete: `select(.ver == "1.2")`, StartPos: 11}},
		{"quoted dot", `.foo == "a.b"`, Context{Path: ".", Incomplete: `foo == "a.b"`, StartPos: 1}},
		{"single digit float", `.foo == 1.5`, Context{Path: ".", Incomplete: "foo == 1.5", StartPos: 1}},
		{"multi digit float", `.foo == 10.5`, Context{Path: ".", Incomplete: "foo == 10.5", StartPos: 1}},
		{"long float with trailing dot", `.foo == 123.`, Context{Path: ".", Incomplete: "foo == 123.", StartPos: 1}},
		{"digit suffix key", `.a12.b`, Context{Path: ".a12", Incomplete: "b", StartPos: 4}},
		{"digit suffix nested key", `.user1.na`, Context{Path: ".user1", Incomplete: "na", StartPos: 7}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := Parse(tt.filter); got != tt.want {
				t.Errorf("Parse(%q) = %+v, want %+v", tt.filter, got, tt.want)
			}
		})
	}
}
