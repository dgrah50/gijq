package jq

import (
	"bytes"
	"encoding/json"
	"fmt"
	"sort"

	"github.com/itchyny/gojq"
)

// Result holds both display and raw output from jq execution
type Result struct {
	Colored string // Syntax highlighted for display
	Raw     string // Plain text for clipboard
	Error   error
}

// Service wraps gojq for executing jq filters
type Service struct {
	data any // Parsed JSON kept in memory
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

	return &Service{data: data}, nil
}

// Execute runs a jq filter and returns the result
func (s *Service) Execute(filter string) Result {
	query, err := gojq.Parse(filter)
	if err != nil {
		return Result{Error: fmt.Errorf("parse error: %w", err)}
	}

	var results []any
	iter := query.Run(s.data)
	for {
		v, ok := iter.Next()
		if !ok {
			break
		}
		if err, isErr := v.(error); isErr {
			return Result{Error: err}
		}
		results = append(results, v)
	}

	raw := formatResults(results)
	colored := Colorize(raw)

	return Result{Raw: raw, Colored: colored}
}

func formatResults(results []any) string {
	var buf bytes.Buffer
	for i, r := range results {
		if i > 0 {
			buf.WriteByte('\n')
		}
		b, err := json.MarshalIndent(r, "", "  ")
		if err != nil {
			buf.WriteString(fmt.Sprintf("%v", r))
		} else {
			buf.Write(b)
		}
	}
	return buf.String()
}

// Data returns the parsed JSON data (for autocomplete)
func (s *Service) Data() any {
	return s.data
}

// KeysAt returns available keys at the given jq path
func (s *Service) KeysAt(path string) ([]string, error) {
	query, err := gojq.Parse(path)
	if err != nil {
		return nil, fmt.Errorf("parse error: %w", err)
	}

	iter := query.Run(s.data)
	v, ok := iter.Next()
	if !ok {
		return nil, nil
	}
	if err, isErr := v.(error); isErr {
		return nil, err
	}

	return extractKeys(v), nil
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
		// For arrays, return index hints
		keys := make([]string, len(val))
		for i := range val {
			keys[i] = fmt.Sprintf("[%d]", i)
		}
		return keys
	default:
		return nil
	}
}
