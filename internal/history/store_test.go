package history

import (
	"os"
	"path/filepath"
	"slices"
	"testing"
)

func TestStore(t *testing.T) {
	// Use temp directory
	tmpDir := t.TempDir()
	store, err := NewStore(filepath.Join(tmpDir, "history.json"))
	if err != nil {
		t.Fatalf("NewStore failed: %v", err)
	}

	file := "/path/to/data.json"

	// Initially empty
	if got := store.Get(file); len(got) != 0 {
		t.Errorf("expected empty, got %v", got)
	}

	// Add queries
	store.Add(file, ".users")
	store.Add(file, ".meta")
	store.Add(file, ".users[0]")

	got := store.Get(file)
	want := []string{".users[0]", ".meta", ".users"} // Most recent first
	if !equalSlices(got, want) {
		t.Errorf("Get() = %v, want %v", got, want)
	}

	// Dedupe: adding existing moves to front
	store.Add(file, ".users")
	got = store.Get(file)
	want = []string{".users", ".users[0]", ".meta"}
	if !equalSlices(got, want) {
		t.Errorf("after dedupe: Get() = %v, want %v", got, want)
	}
}

func TestStorePersistence(t *testing.T) {
	tmpDir := t.TempDir()
	path := filepath.Join(tmpDir, "history.json")

	store1, _ := NewStore(path)
	store1.Add("/data.json", ".foo")
	store1.Add("/data.json", ".bar")
	store1.Save()

	// Reload
	store2, _ := NewStore(path)
	got := store2.Get("/data.json")
	want := []string{".bar", ".foo"}
	if !equalSlices(got, want) {
		t.Errorf("after reload: Get() = %v, want %v", got, want)
	}
}

func TestStoreMaxLimit(t *testing.T) {
	tmpDir := t.TempDir()
	store, _ := NewStore(filepath.Join(tmpDir, "history.json"))

	file := "/data.json"
	// Add more than max
	for i := 0; i < 60; i++ {
		store.Add(file, string(rune('a'+i%26))+string(rune('0'+i/26)))
	}

	got := store.Get(file)
	if len(got) > 50 {
		t.Errorf("expected max 50, got %d", len(got))
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

func TestStoreGetReturnsCopy(t *testing.T) {
	s, _ := NewStore(filepath.Join(t.TempDir(), "h.json"))
	s.Add("f", "a")
	s.Add("f", "b")
	got := s.Get("f")
	got[0] = "mutated"
	s.Add("f", "a") // dedupe used to rewrite the shared backing array
	if want := []string{"a", "b"}; !slices.Equal(s.Get("f"), want) {
		t.Fatalf("Get = %v, want %v", s.Get("f"), want)
	}
	if got[0] != "mutated" || got[1] != "a" {
		t.Fatalf("caller's copy changed: %v", got)
	}
}

func TestStoreCorruptFileIsNeverOverwritten(t *testing.T) {
	path := filepath.Join(t.TempDir(), "h.json")
	if err := os.WriteFile(path, []byte("{not json"), 0o644); err != nil {
		t.Fatal(err)
	}
	s, err := NewStore(path)
	if err == nil || s == nil {
		t.Fatalf("want usable store plus error, got %v, %v", s, err)
	}
	s.Add("f", ".a")
	if got := s.Get("f"); len(got) != 1 {
		t.Fatalf("in-memory store not usable: %v", got)
	}
	if err := s.Save(); err != nil {
		t.Fatal(err)
	}
	if data, _ := os.ReadFile(path); string(data) != "{not json" {
		t.Fatalf("corrupt file was overwritten: %q", data)
	}
}

func TestStoreSaveIsAtomic(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "sub", "h.json")
	s, _ := NewStore(path)
	s.Add("f", ".a")
	if err := s.Save(); err != nil {
		t.Fatal(err)
	}
	entries, _ := os.ReadDir(filepath.Dir(path))
	if len(entries) != 1 || entries[0].Name() != "h.json" {
		t.Fatalf("leftover temp files: %v", entries)
	}
	re, err := NewStore(path)
	if err != nil || !slices.Equal(re.Get("f"), []string{".a"}) {
		t.Fatalf("reload = %v, %v", re.Get("f"), err)
	}
}
