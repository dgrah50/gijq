package history

import (
	"os"
	"path/filepath"
	"testing"
)

func TestNewStoreNullFileAddDoesNotPanic(t *testing.T) {
	path := filepath.Join(t.TempDir(), "history.json")
	if err := os.WriteFile(path, []byte("null"), 0o644); err != nil {
		t.Fatal(err)
	}
	s, _ := NewStore(path)
	defer func() {
		if r := recover(); r != nil {
			t.Fatalf("Add panicked: %v", r)
		}
	}()
	s.Add("f.json", ".a")
	if got := s.Get("f.json"); len(got) != 1 || got[0] != ".a" {
		t.Fatalf("got %v", got)
	}
}
