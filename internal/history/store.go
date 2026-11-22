package history

import (
	"encoding/json"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"slices"
	"sync"
)

const maxPerFile = 50

// Store manages per-file query history.
type Store struct {
	// path is where history is persisted; "" means in-memory only (Save is a
	// no-op). NewStore uses that when the existing file can't be parsed, so a
	// corrupt file is never overwritten.
	path    string
	entries map[string][]string // filepath → queries (most recent first)
	mu      sync.RWMutex
}

// NewStore loads history from path. A missing file is not an error. If the
// file exists but can't be read or parsed, NewStore returns a usable
// in-memory store together with the error, and leaves the file untouched.
func NewStore(path string) (*Store, error) {
	s := &Store{path: path, entries: make(map[string][]string)}

	data, err := os.ReadFile(path)
	if errors.Is(err, fs.ErrNotExist) {
		return s, nil
	}
	if err == nil {
		err = json.Unmarshal(data, &s.entries)
	}
	if err != nil {
		return &Store{entries: make(map[string][]string)},
			fmt.Errorf("history %s unusable, not saving this session: %w", path, err)
	}
	if s.entries == nil {
		s.entries = make(map[string][]string)
	}
	return s, nil
}

// Add records query as the most recent entry for file, removing any earlier
// duplicate and keeping at most maxPerFile entries.
func (s *Store) Add(file, query string) {
	s.mu.Lock()
	defer s.mu.Unlock()

	queries := make([]string, 0, maxPerFile)
	queries = append(queries, query)
	for _, q := range s.entries[file] {
		if q != query && len(queries) < maxPerFile {
			queries = append(queries, q)
		}
	}
	s.entries[file] = queries
}

// Get returns a copy of the queries for file, most recent first.
func (s *Store) Get(file string) []string {
	s.mu.RLock()
	defer s.mu.RUnlock()

	return slices.Clone(s.entries[file])
}

// Save persists the history atomically (write to a temp file, then rename),
// so a crash mid-write can't corrupt the existing file.
func (s *Store) Save() error {
	if s.path == "" {
		return nil
	}

	s.mu.RLock()
	data, err := json.MarshalIndent(s.entries, "", "  ")
	s.mu.RUnlock()
	if err != nil {
		return err
	}

	dir := filepath.Dir(s.path)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return err
	}
	tmp, err := os.CreateTemp(dir, ".history-*.json")
	if err != nil {
		return err
	}
	defer os.Remove(tmp.Name()) // no-op after a successful rename

	if _, err := tmp.Write(data); err != nil {
		tmp.Close()
		return err
	}
	if err := tmp.Close(); err != nil {
		return err
	}
	if err := os.Chmod(tmp.Name(), 0o644); err != nil {
		return err
	}
	return os.Rename(tmp.Name(), s.path)
}
