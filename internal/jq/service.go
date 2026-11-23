package jq

import (
	"context"
	"encoding/json"
	"fmt"
	"sort"
	"strconv"
	"strings"
	"sync"

	"github.com/itchyny/gojq"
)

// Service wraps gojq for executing jq filters
type Service struct {
	data any // Parsed JSON kept in memory

	mu        sync.Mutex // LRU Get mutates recency, so a plain mutex is used
	codeCache *lru[*gojq.Code]
	keysCache *lru[[]string]
}

// NewService creates a jq service from JSON bytes
func NewService(jsonData []byte) (*Service, error) {
	if len(jsonData) == 0 {
		return nil, fmt.Errorf("empty input")
	}

	var data any
	if err := json.Unmarshal(jsonData, &data); err != nil {
		return nil, fmt.Errorf("invalid JSON: %w", err)
	}

	return &Service{
		data:      data,
		codeCache: newLRU[*gojq.Code](cacheCap),
		keysCache: newLRU[[]string](cacheCap),
	}, nil
}

// Iter compiles filter (through the code cache) and starts it on the parsed
// input. The caller drains the iterator; error values come out of Next as
// error values. Cancelling ctx stops the evaluation.
func (s *Service) Iter(ctx context.Context, filter string) (gojq.Iter, error) {
	code, err := s.compiledQuery(filter)
	if err != nil {
		return nil, err
	}
	return code.RunWithContext(ctx, s.data), nil
}

// KeysAt returns available keys at the given jq path
func (s *Service) KeysAt(path string) ([]string, error) {
	return s.KeysAtWithContext(context.Background(), path)
}

// KeysAtWithContext is KeysAt with cancellation support.
func (s *Service) KeysAtWithContext(ctx context.Context, path string) ([]string, error) {
	if path == "" {
		path = "."
	}

	if keys, ok := s.cachedKeys(path); ok {
		return cloneStrings(keys), nil
	}

	if keys, ok := keysAtSimplePath(s.data, path); ok {
		s.storeKeys(path, keys)
		return cloneStrings(keys), nil
	}

	code, err := s.compiledQuery(path)
	if err != nil {
		return nil, err
	}

	iter := code.RunWithContext(ctx, s.data)
	v, ok := iter.Next()
	if !ok {
		if err := ctx.Err(); err != nil {
			return nil, err
		}
		s.storeKeys(path, nil)
		return nil, nil
	}
	if err, isErr := v.(error); isErr {
		return nil, err
	}

	keys := extractKeys(v)
	s.storeKeys(path, keys)
	return cloneStrings(keys), nil
}

func extractKeys(v any) []string {
	switch val := v.(type) {
	case map[string]any:
		keys := make([]string, 0, len(val))
		for k := range val {
			keys = append(keys, k)
		}
		sort.Strings(keys)
		return keys
	case []any:
		// For large arrays, cap index hints to avoid huge allocations in the UI.
		const maxArrayHints = 256
		n := len(val)
		if n > maxArrayHints {
			n = maxArrayHints
		}
		keys := make([]string, n)
		for i := 0; i < n; i++ {
			keys[i] = fmt.Sprintf("[%d]", i)
		}
		return keys
	default:
		return nil
	}
}

// cachedKeys returns the cached slice itself; callers must clone before
// handing it out.
func (s *Service) cachedKeys(path string) ([]string, bool) {
	s.mu.Lock()
	keys, ok := s.keysCache.Get(path)
	s.mu.Unlock()
	return keys, ok
}

// storeKeys takes ownership of keys; the cache never exposes it uncloned.
func (s *Service) storeKeys(path string, keys []string) {
	s.mu.Lock()
	s.keysCache.Put(path, keys)
	s.mu.Unlock()
}

func (s *Service) compiledQuery(filter string) (*gojq.Code, error) {
	s.mu.Lock()
	code, ok := s.codeCache.Get(filter)
	s.mu.Unlock()
	if ok {
		return code, nil
	}

	query, err := gojq.Parse(filter)
	if err != nil {
		return nil, fmt.Errorf("parse error: %w", err)
	}

	code, err = gojq.Compile(query)
	if err != nil {
		return nil, fmt.Errorf("compile error: %w", err)
	}

	s.mu.Lock()
	if existing, ok := s.codeCache.Get(filter); ok {
		s.mu.Unlock()
		return existing, nil
	}
	s.codeCache.Put(filter, code)
	s.mu.Unlock()
	return code, nil
}

func cloneStrings(in []string) []string {
	if in == nil {
		return nil
	}
	out := make([]string, len(in))
	copy(out, in)
	return out
}

type pathTokenKind int

const (
	pathTokenKey pathTokenKind = iota
	pathTokenIndex
	pathTokenIter
)

type pathToken struct {
	kind  pathTokenKind
	key   string
	index int
}

func keysAtSimplePath(data any, path string) ([]string, bool) {
	tokens, ok := parseSimplePath(path)
	if !ok {
		return nil, false
	}

	current := data
	for _, token := range tokens {
		switch token.kind {
		case pathTokenKey:
			obj, isObj := current.(map[string]any)
			if !isObj {
				return nil, true
			}
			current = obj[token.key]
		case pathTokenIndex:
			arr, isArr := current.([]any)
			if !isArr || token.index < 0 || token.index >= len(arr) {
				return nil, true
			}
			current = arr[token.index]
		case pathTokenIter:
			switch val := current.(type) {
			case []any:
				if len(val) == 0 {
					return nil, true
				}
				current = val[0]
			case map[string]any:
				if len(val) == 0 {
					return nil, true
				}
				keys := make([]string, 0, len(val))
				for key := range val {
					keys = append(keys, key)
				}
				sort.Strings(keys)
				current = val[keys[0]]
			default:
				return nil, true
			}
		}
	}

	return extractKeys(current), true
}

func parseSimplePath(path string) ([]pathToken, bool) {
	if path == "" || path == "." {
		return nil, true
	}
	if !strings.HasPrefix(path, ".") {
		return nil, false
	}

	var tokens []pathToken
	for i := 1; i < len(path); {
		switch path[i] {
		case '.':
			i++
		case '[':
			i++
			if i >= len(path) {
				return nil, false
			}
			if path[i] == ']' {
				tokens = append(tokens, pathToken{kind: pathTokenIter})
				i++
				continue
			}
			start := i
			for i < len(path) && path[i] >= '0' && path[i] <= '9' {
				i++
			}
			if start == i || i >= len(path) || path[i] != ']' {
				return nil, false
			}
			index, err := strconv.Atoi(path[start:i])
			if err != nil {
				return nil, false
			}
			tokens = append(tokens, pathToken{kind: pathTokenIndex, index: index})
			i++
		default:
			start := i
			for i < len(path) && isSimpleIdentifierChar(path[i]) {
				i++
			}
			if start == i {
				return nil, false
			}
			tokens = append(tokens, pathToken{kind: pathTokenKey, key: path[start:i]})
		}
	}
	return tokens, true
}

func isSimpleIdentifierChar(ch byte) bool {
	return ch == '_' ||
		(ch >= 'a' && ch <= 'z') ||
		(ch >= 'A' && ch <= 'Z') ||
		(ch >= '0' && ch <= '9')
}
