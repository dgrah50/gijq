package ui

import "testing"

func TestPrevWordStart(t *testing.T) {
	value := ".users[] | select(.active) | .name"

	tests := []struct {
		name string
		pos  int
		want int
	}{
		{name: "end to current word", pos: len([]rune(value)), want: 30}, // .name
		{name: "from punctuation", pos: 28, want: 19},                    // active
		{name: "from inside word", pos: 24, want: 19},                    // active
		{name: "from start", pos: 0, want: 0},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := prevWordStart(value, tt.pos)
			if got != tt.want {
				t.Fatalf("prevWordStart(%q, %d) = %d, want %d", value, tt.pos, got, tt.want)
			}
		})
	}
}

func TestNextWordStart(t *testing.T) {
	value := ".users[] | select(.active) | .name"

	tests := []struct {
		name string
		pos  int
		want int
	}{
		{name: "start to users", pos: 0, want: 1},
		{name: "from users to select", pos: 1, want: 11},
		{name: "from select to active", pos: 11, want: 19},
		{name: "from active to name", pos: 19, want: 30},
		{name: "at end", pos: len([]rune(value)), want: len([]rune(value))},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := nextWordStart(value, tt.pos)
			if got != tt.want {
				t.Fatalf("nextWordStart(%q, %d) = %d, want %d", value, tt.pos, got, tt.want)
			}
		})
	}
}

func TestNextWordDeleteEnd(t *testing.T) {
	value := ".users[] | select(.active) | .name"

	tests := []struct {
		name string
		pos  int
		want int
	}{
		{name: "inside word", pos: 1, want: 6},
		{name: "at delimiter deletes next word", pos: 7, want: 17},
		{name: "at end", pos: len([]rune(value)), want: len([]rune(value))},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := nextWordDeleteEnd(value, tt.pos)
			if got != tt.want {
				t.Fatalf("nextWordDeleteEnd(%q, %d) = %d, want %d", value, tt.pos, got, tt.want)
			}
		})
	}
}
