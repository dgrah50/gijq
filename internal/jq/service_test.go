package jq

import (
	"context"
	"errors"
	"strconv"
	"strings"
	"sync"
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

func TestExecuteNoHTMLEscape(t *testing.T) {
	svc, _ := NewService([]byte(`{"s":"<a&b>"}`))
	r := svc.Execute(".s")
	if r.Error != nil || r.Raw != `"<a&b>"` {
		t.Fatalf("got %q err=%v", r.Raw, r.Error)
	}
}

func TestExecuteMultiResultFormat(t *testing.T) {
	svc, _ := NewService([]byte(`[1,2]`))
	if r := svc.Execute(".[]"); r.Raw != "1\n2" {
		t.Fatalf("got %q", r.Raw)
	}
	svc2, _ := NewService([]byte(`{"a":[1]}`))
	if r := svc2.Execute("."); r.Raw != "{\n  \"a\": [\n    1\n  ]\n}" {
		t.Fatalf("got %q", r.Raw)
	}
}

func TestExecuteCanceledContext(t *testing.T) {
	svc, _ := NewService([]byte(`[1,2]`))
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if r := svc.ExecuteWithContext(ctx, ".[]"); !errors.Is(r.Error, context.Canceled) {
		t.Fatalf("got %v", r.Error)
	}
}

func TestLRUBound(t *testing.T) {
	c := newLRU[int](3)
	for i := 0; i < 10; i++ {
		c.Put(strconv.Itoa(i), i)
		if c.Len() > 3 {
			t.Fatalf("len %d", c.Len())
		}
	}
	if _, ok := c.Get("0"); ok {
		t.Fatal("0 should be evicted")
	}
	if v, ok := c.Get("9"); !ok || v != 9 {
		t.Fatal("9 missing")
	}
	c.Put("a", 1)
	if _, ok := c.Get("9"); !ok {
		t.Fatal("recency not honored")
	}
}

func TestServiceCachesBounded(t *testing.T) {
	svc, _ := NewService([]byte(`{"a":1}`))
	for i := 0; i < cacheCap*2; i++ {
		f := ". + " + strconv.Itoa(i)
		svc.KeysAt(f)
		svc.Execute(f)
	}
	if svc.codeCache.Len() > cacheCap || svc.keysCache.Len() > cacheCap {
		t.Fatalf("caches exceed cap: %d %d", svc.codeCache.Len(), svc.keysCache.Len())
	}
}

func TestKeysAtMutationIsolated(t *testing.T) {
	svc, _ := NewService([]byte(`{"a":1,"b":2}`))
	k1, _ := svc.KeysAt(".")
	k1[0] = "zzz"
	k2, _ := svc.KeysAt(".")
	if k2[0] != "a" {
		t.Fatalf("cache mutated: %v", k2)
	}
	k2[0] = "yyy"
	k3, _ := svc.KeysAt(".")
	if k3[0] != "a" {
		t.Fatalf("cache mutated: %v", k3)
	}
}

func TestKeysAtConcurrent(t *testing.T) {
	svc, _ := NewService([]byte(`{"a":{"b":1}}`))
	var wg sync.WaitGroup
	for i := 0; i < 8; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for j := 0; j < 300; j++ {
				svc.KeysAt(".a" + strconv.Itoa(j))
				svc.Execute(".a")
			}
		}()
	}
	wg.Wait()
}
