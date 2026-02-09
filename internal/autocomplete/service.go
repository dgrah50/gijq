package autocomplete

import (
	"sort"
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

// Suggest returns matching keys for the current filter
func (s *Service) Suggest(filter string) ([]string, Context) {
	ctx := Parse(filter)

	// If filter contains a pipe, resolve context from the left side's output
	var keys []string
	var err error
	if pipeIdx := strings.LastIndex(filter, "|"); pipeIdx >= 0 {
		leftSide := strings.TrimSpace(filter[:pipeIdx])
		if leftSide != "" {
			keys, err = s.resolveKeysAfterPipe(leftSide, ctx.Path)
		}
	}

	if keys == nil {
		keys, err = s.jq.KeysAt(ctx.Path)
	}
	if err != nil || keys == nil {
		return []string{}, ctx
	}

	// Filter by prefix (case-insensitive)
	var matches []string
	incLower := strings.ToLower(ctx.Incomplete)
	for _, k := range keys {
		if strings.HasPrefix(strings.ToLower(k), incLower) {
			matches = append(matches, k)
		}
	}

	sort.Strings(matches)
	return matches, ctx
}

// resolveKeysAfterPipe determines available keys from the output of the left side of a pipe
func (s *Service) resolveKeysAfterPipe(leftSide string, rightPath string) ([]string, error) {
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
		keys, err := s.jq.KeysAt(testPath)
		if err == nil && keys != nil {
			return keys, nil
		}
	}

	return s.jq.KeysAt(evalPath)
}

// Apply inserts the selected suggestion into the filter
func (s *Service) Apply(filter string, ctx Context, selected string) string {
	return filter[:ctx.StartPos] + selected
}
