package jq

import (
	"strings"
	"testing"
)

func TestNewService(t *testing.T) {
	tests := []struct {
		name    string
		input   string
		wantErr bool
	}{
		{"valid object", `{"a":1}`, false},
		{"valid array", `[1,2,3]`, false},
		{"invalid json", `{bad`, true},
		{"empty", ``, true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := NewService([]byte(tt.input))
			if (err != nil) != tt.wantErr {
				t.Errorf("NewService() error = %v, wantErr %v", err, tt.wantErr)
			}
		})
	}
}

func TestExecute(t *testing.T) {
	json := `{"users":[{"name":"alice","age":30},{"name":"bob","age":25}],"meta":{"version":"1.0"}}`
	svc, err := NewService([]byte(json))
	if err != nil {
		t.Fatalf("NewService failed: %v", err)
	}

	tests := []struct {
		name        string
		filter      string
		wantRaw     string
		wantContain string // For tests where exact match is hard (pretty-printing)
		wantErr     bool
	}{
		{"identity", ".", "", "alice", false}, // Identity returns pretty-printed, just check content
		{"simple key", ".meta.version", `"1.0"`, "", false},
		{"array index", ".users[0].name", `"alice"`, "", false},
		{"array iterate", ".users[].name", "\"alice\"\n\"bob\"", "", false},
		{"nested", ".users[1].age", "25", "", false},
		{"missing key", ".notfound", "null", "", false},
		{"parse error", ".[invalid", "", "", true},
		{"pipe", ".users | length", "2", "", false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := svc.Execute(tt.filter)
			if tt.wantErr {
				if result.Error == nil {
					t.Error("expected error, got nil")
				}
				return
			}
			if result.Error != nil {
				t.Errorf("unexpected error: %v", result.Error)
				return
			}
			got := strings.TrimSpace(result.Raw)
			// Use wantContain for content checks, wantRaw for exact matches
			if tt.wantContain != "" {
				if !strings.Contains(got, tt.wantContain) {
					t.Errorf("Raw = %q, want to contain %q", got, tt.wantContain)
				}
			} else {
				want := strings.TrimSpace(tt.wantRaw)
				if got != want {
					t.Errorf("Raw = %q, want %q", got, want)
				}
			}
		})
	}
}

func TestKeysAt(t *testing.T) {
	json := `{"users":[{"name":"alice","age":30},{"name":"bob","age":25}],"meta":{"version":"1.0","count":2}}`
	svc, _ := NewService([]byte(json))

	tests := []struct {
		name     string
		path     string
		wantKeys []string
		wantErr  bool
	}{
		{"root", ".", []string{"meta", "users"}, false},
		{"nested object", ".meta", []string{"count", "version"}, false},
		{"array element", ".users[0]", []string{"age", "name"}, false},
		{"invalid path", ".notfound", nil, false}, // null has no keys
		{"parse error", ".[bad", nil, true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			keys, err := svc.KeysAt(tt.path)
			if tt.wantErr {
				if err == nil {
					t.Error("expected error, got nil")
				}
				return
			}
			if err != nil {
				t.Errorf("unexpected error: %v", err)
				return
			}
			if !equalSlices(keys, tt.wantKeys) {
				t.Errorf("KeysAt(%q) = %v, want %v", tt.path, keys, tt.wantKeys)
			}
		})
	}
}

func equalSlices(a, b []string) bool {
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
