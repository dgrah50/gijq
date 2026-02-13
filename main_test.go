package main

import (
	"bytes"
	"strings"
	"testing"
)

func TestWantsHelp(t *testing.T) {
	tests := []struct {
		name string
		args []string
		want bool
	}{
		{name: "short flag", args: []string{"-h"}, want: true},
		{name: "long flag", args: []string{"--help"}, want: true},
		{name: "mixed args", args: []string{"data.json", "--help"}, want: true},
		{name: "no help", args: []string{"data.json"}, want: false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := wantsHelp(tt.args)
			if got != tt.want {
				t.Fatalf("wantsHelp(%v) = %v, want %v", tt.args, got, tt.want)
			}
		})
	}
}

func TestPrintHelp(t *testing.T) {
	var buf bytes.Buffer
	printHelp(&buf)

	out := buf.String()
	if !strings.Contains(out, "usage: gijq <file.json>") {
		t.Fatalf("help output missing usage line: %q", out)
	}
	if !strings.Contains(out, "-h, --help") {
		t.Fatalf("help output missing help option: %q", out)
	}
}
