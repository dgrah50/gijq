package ui

import "testing"

func TestFilterKeysByPrefix(t *testing.T) {
	keys := []string{"meta", "users", "version", "Value"}

	tests := []struct {
		name       string
		incomplete string
		want       []string
	}{
		{name: "empty", incomplete: "", want: []string{"meta", "users", "version", "Value"}},
		{name: "single exact prefix", incomplete: "me", want: []string{"meta"}},
		{name: "case insensitive", incomplete: "va", want: []string{"Value"}},
		{name: "no match", incomplete: "zzz", want: []string{}},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := filterKeysByPrefix(keys, tt.incomplete)
			if !equalStringSlices(got, tt.want) {
				t.Fatalf("filterKeysByPrefix(%q) = %v, want %v", tt.incomplete, got, tt.want)
			}
		})
	}
}

func equalStringSlices(a, b []string) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}
