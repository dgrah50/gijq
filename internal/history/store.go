package history

import (
	"encoding/json"
	"os"
	"path/filepath"
	"sync"
)

const maxPerFile = 50

// Store manages per-file query history
type Store struct {
	path    string
	entries map[string][]string // filepath → queries (most recent first)
	mu      sync.RWMutex
}

// NewStore creates or loads a history store
func NewStore(path string) (*Store, error) {
	s := &Store{
		path:    path,
		entries: make(map[string][]string),
	}

	// Try to load existing
	data, err := os.ReadFile(path)
	if err == nil {
		json.Unmarshal(data, &s.entries)
	}

	return s, nil
}

// Add adds a query to the history for a file
func (s *Store) Add(file, query string) {
	s.mu.Lock()
	defer s.mu.Unlock()

	queries := s.entries[file]

	// Remove if exists (for dedupe)
	for i, q := range queries {
		if q == query {
			queries = append(queries[:i], queries[i+1:]...)
			break
		}
	}

	// Prepend (most recent first)
	queries = append([]string{query}, queries...)

	// Trim to max
	if len(queries) > maxPerFile {
		queries = queries[:maxPerFile]
	}

	s.entries[file] = queries
}

// Get returns queries for a file (most recent first)
func (s *Store) Get(file string) []string {
	s.mu.RLock()
	defer s.mu.RUnlock()

	return s.entries[file]
}

// Save persists the history to disk
func (s *Store) Save() error {
	s.mu.RLock()
	defer s.mu.RUnlock()

	// Ensure directory exists
	dir := filepath.Dir(s.path)
	if err := os.MkdirAll(dir, 0755); err != nil {
		return err
	}

	data, err := json.MarshalIndent(s.entries, "", "  ")
	if err != nil {
		return err
	}

	return os.WriteFile(s.path, data, 0644)
}
