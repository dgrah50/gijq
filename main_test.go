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

func TestEnvEnabled(t *testing.T) {
	tests := []struct {
		name  string
		value string
		want  bool
	}{
		{name: "one", value: "1", want: true},
		{name: "true", value: "true", want: true},
		{name: "yes", value: "yes", want: true},
		{name: "on", value: "on", want: true},
		{name: "false", value: "false", want: false},
		{name: "zero", value: "0", want: false},
		{name: "empty", value: "", want: false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Setenv("GIJQ_TELEMETRY", tt.value)
			got := envEnabled("GIJQ_TELEMETRY")
			if got != tt.want {
				t.Fatalf("envEnabled(%q) = %v, want %v", tt.value, got, tt.want)
			}
		})
	}
}
