package jq

import (
	"testing"
)

func TestColorize(t *testing.T) {
	tests := []struct {
		name  string
		input string
	}{
		{"string", `"hello"`},
		{"number", `42`},
		{"bool true", `true`},
		{"bool false", `false`},
		{"null", `null`},
		{"object", `{"key": "value"}`},
		{"array", `[1, 2, 3]`},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := Colorize(tt.input)
			// Colorize currently returns input unchanged (disabled)
			if result != tt.input {
				t.Errorf("expected %q, got %q", tt.input, result)
			}
		})
	}
}
