package autocomplete

import (
	"context"
	"strings"

	"github.com/dayangraham/gijq/internal/jq"
)

// Service provides autocomplete suggestions
type Service struct {
	jq *jq.Service
}

// NewService creates an autocomplete service
func NewService(jqSvc *jq.Service) *Service {
	return &Service{jq: jqSvc}
}

// ParseContext extracts path/incomplete state without querying keys.
func (s *Service) ParseContext(filter string) Context {
	return Parse(filter)
}

// Suggest returns matching keys for the current filter
func (s *Service) Suggest(filter string) ([]string, Context) {
	return s.SuggestWithContext(context.Background(), filter)
}

// SuggestWithContext is Suggest with cancellation support.
func (s *Service) SuggestWithContext(ctx context.Context, filter string) ([]string, Context) {
	pc := s.ParseContext(filter)

	// If filter contains a pipe, resolve context from the left side's output
	var keys []string
	var err error
	if pipeIdx := findLastPipe(filter); pipeIdx >= 0 {
		leftSide := strings.TrimSpace(filter[:pipeIdx])
		if leftSide != "" {
			keys, err = s.resolveKeysAfterPipe(ctx, leftSide, pc.Path)
		}
	}

	if keys == nil {
		keys, err = s.jq.KeysAtWithContext(ctx, pc.Path)
	}
	if err != nil || keys == nil {
		return []string{}, pc
	}

	// Keys are already ordered (object keys sorted, array hints numeric).
	if pc.Incomplete == "" {
		return keys, pc // KeysAt already returns a private copy
	}

	// Filter by prefix (case-insensitive)
	matches := make([]string, 0, len(keys))
	incLower := strings.ToLower(pc.Incomplete)
	for _, k := range keys {
		if strings.HasPrefix(strings.ToLower(k), incLower) {
			matches = append(matches, k)
		}
	}
	return matches, pc
}

// resolveKeysAfterPipe determines available keys from the output of the left side of a pipe
func (s *Service) resolveKeysAfterPipe(ctx context.Context, leftSide string, rightPath string) ([]string, error) {
	// Build the full evaluation path: leftSide | rightPath
	evalPath := leftSide
	if rightPath != "" && rightPath != "." {
		evalPath = leftSide + " | " + rightPath
	}

	// For array iterators like .users[], get keys from the first element
	if strings.HasSuffix(leftSide, "[]") {
		testPath := strings.TrimSuffix(leftSide, "[]") + "[0]"
		if rightPath != "" && rightPath != "." {
			testPath = testPath + " | " + rightPath
		}
		keys, err := s.jq.KeysAtWithContext(ctx, testPath)
		if err == nil && keys != nil {
			return keys, nil
		}
	}

	return s.jq.KeysAtWithContext(ctx, evalPath)
}

// Apply inserts the selected suggestion into the filter
func (s *Service) Apply(filter string, ctx Context, selected string) string {
	return filter[:ctx.StartPos] + selected
}
