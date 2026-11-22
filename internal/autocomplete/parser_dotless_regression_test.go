package autocomplete

import "testing"

func TestParseDotlessFilterStartPosAndApply(t *testing.T) {
	svc := NewService(nil)

	tests := []struct {
		name      string
		filter    string
		selected  string
		wantPath  string
		wantInc   string
		wantStart int
		wantApply string
	}{
		{
			name:      "dotless root filter",
			filter:    "foo",
			selected:  "foobar",
			wantPath:  ".",
			wantInc:   "foo",
			wantStart: 0,
			wantApply: "foobar",
		},
		{
			name:      "dotless filter after pipe",
			filter:    ".a | foo",
			selected:  "foobar",
			wantPath:  ".",
			wantInc:   "foo",
			wantStart: 5,
			wantApply: ".a | foobar",
		},
		{
			name:      "dotless filter after pipe with extra spaces",
			filter:    ".a |   foo",
			selected:  "foobar",
			wantPath:  ".",
			wantInc:   "foo",
			wantStart: 7,
			wantApply: ".a |   foobar",
		},
		{
			name:      "dotless utf8 filter",
			filter:    "日本語",
			selected:  "日本語キー",
			wantPath:  ".",
			wantInc:   "日本語",
			wantStart: 0,
			wantApply: "日本語キー",
		},
		{
			name:      "leading dot with unmatched closing bracket",
			filter:    ".foo]",
			selected:  "foobar",
			wantPath:  ".",
			wantInc:   "foo]",
			wantStart: 1,
			wantApply: ".foobar",
		},
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

			got := svc.Apply(tt.filter, ctx, tt.selected)
			if got != tt.wantApply {
				t.Errorf("Apply(%q, ctx, %q) = %q, want %q", tt.filter, tt.selected, got, tt.wantApply)
			}
		})
	}
}
