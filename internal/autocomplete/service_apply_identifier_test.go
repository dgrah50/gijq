package autocomplete

import "testing"

func TestApplyQuotesNonIdentifierKeys(t *testing.T) {
	svc := NewService(nil)
	tests := []struct {
		selected string
		want     string
	}{
		{"foo-bar", `."foo-bar"`},
		{"foo bar", `."foo bar"`},
		{"123abc", `."123abc"`},
		{"name", ".name"},
	}

	for _, tt := range tests {
		t.Run(tt.selected, func(t *testing.T) {
			filter := ".foo"
			got := svc.Apply(filter, Parse(filter), tt.selected)
			if got != tt.want {
				t.Errorf("Apply(%q) = %q, want %q", tt.selected, got, tt.want)
			}
		})
	}
}
