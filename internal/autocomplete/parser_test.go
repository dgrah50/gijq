package autocomplete

import "testing"

func TestParse(t *testing.T) {
	tests := []struct {
		name      string
		filter    string
		wantPath  string
		wantInc   string
		wantStart int
	}{
		// Basic cases
		{"empty", "", ".", "", 0},
		{"just dot", ".", ".", "", 1},
		{"simple key", ".foo", ".", "foo", 1},

		// After complete key
		{"complete key then dot", ".foo.", ".foo", "", 5},
		{"incomplete after dot", ".foo.ba", ".foo", "ba", 5},

		// Array access - when ending with ], path is complete
		{"array then dot", ".users[0].", ".users[0]", "", 10},
		{"array then incomplete", ".users[0].na", ".users[0]", "na", 10},

		// Nested paths
		{"nested", ".a.b.c", ".a.b", "c", 5},
		{"nested complete", ".a.b.c.", ".a.b.c", "", 7},

		// After pipe - context resets
		{"pipe then dot", ".foo | .", ".", "", 8},
		{"pipe then key", ".foo | .bar", ".", "bar", 8},
		{"pipe then nested", ".foo | .bar.", ".bar", "", 12},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ctx := Parse(tt.filter)
			if ctx.Path != tt.wantPath {
				t.Errorf("Path = %q, want %q", ctx.Path, tt.wantPath)
			}
			if ctx.Incomplete != tt.wantInc {
				t.Errorf("Incomplete = %q, want %q", ctx.Incomplete, tt.wantInc)
			}
			if ctx.StartPos != tt.wantStart {
				t.Errorf("StartPos = %d, want %d", ctx.StartPos, tt.wantStart)
			}
		})
	}
}
