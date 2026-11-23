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
	keys, err := s.keysForContext(ctx, filter, pc)
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

// KeysContext returns a stable identifier for the keys available at filter.
func (s *Service) KeysContext(filter string) string {
	pc := s.ParseContext(filter)
	if pipeIdx := findLastPipe(filter); pipeIdx >= 0 {
		return strings.TrimSpace(filter[:pipeIdx]) + " | " + pc.Path
	}
	return pc.Path
}

// KeysFor returns the unfiltered keys available at the current filter context.
func (s *Service) KeysFor(ctx context.Context, filter string) ([]string, error) {
	return s.keysForContext(ctx, filter, s.ParseContext(filter))
}

func (s *Service) keysForContext(ctx context.Context, filter string, pc Context) ([]string, error) {
	if pipeIdx := findLastPipe(filter); pipeIdx >= 0 {
		leftSide := strings.TrimSpace(filter[:pipeIdx])
		if leftSide == "" {
			return nil, nil
		}
		return s.resolveKeysAfterPipe(ctx, leftSide, pc.Path)
	}
	return s.jq.KeysAtWithContext(ctx, pc.Path)
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
