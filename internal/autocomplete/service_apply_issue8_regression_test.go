package autocomplete

import "testing"

func TestApplyIssue8QuotesNonIdentifierKeys(t *testing.T) {
	tests := []struct {
		selected string
		want     string
	}{
		{selected: "foo-bar", want: `."foo-bar"`},
		{selected: "foo bar", want: `."foo bar"`},
		{selected: "123abc", want: `."123abc"`},
		{selected: "name", want: ".name"},
	}

	svc := NewService(nil)
	for _, tt := range tests {
		t.Run(tt.selected, func(t *testing.T) {
			if got := svc.Apply(".foo", Parse(".foo"), tt.selected); got != tt.want {
				t.Errorf("Apply(%q) = %q, want %q", tt.selected, got, tt.want)
			}
		})
	}
}
