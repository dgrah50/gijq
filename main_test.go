package main

import (
	"bytes"
	"os"
	"path/filepath"
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

func TestLoadInputFileArgWinsOverStdin(t *testing.T) {
	path := filepath.Join(t.TempDir(), "data.json")
	if err := os.WriteFile(path, []byte(`{"from":"file"}`), 0o644); err != nil {
		t.Fatal(err)
	}
	data, name, histKey, err := loadInput([]string{path}, strings.NewReader(`{"from":"stdin"}`), false)
	if err != nil {
		t.Fatal(err)
	}
	if string(data) != `{"from":"file"}` || name != "data.json" || !filepath.IsAbs(histKey) {
		t.Fatalf("got data=%s name=%s key=%s", data, name, histKey)
	}
}

func TestLoadInputStdin(t *testing.T) {
	data, name, histKey, err := loadInput(nil, strings.NewReader(`[1]`), false)
	if err != nil || string(data) != `[1]` || name != "<stdin>" || histKey != "<stdin>" {
		t.Fatalf("got %s %s %s %v", data, name, histKey, err)
	}
}

func TestLoadInputNoInput(t *testing.T) {
	if _, _, _, err := loadInput(nil, strings.NewReader(""), true); err == nil || !strings.Contains(err.Error(), "usage:") {
		t.Fatalf("want usage error, got %v", err)
	}
	if _, _, _, err := loadInput([]string{"/does/not/exist.json"}, nil, true); err == nil {
		t.Fatal("want read error for missing file")
	}
}
