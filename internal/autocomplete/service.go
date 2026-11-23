package autocomplete

import (
	"context"
	"fmt"
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
	prefix := filter[:ctx.StartPos]
	if !isJQIdentifier(selected) && !isArrayHint(selected) {
		if strings.HasSuffix(prefix, ".") {
			return prefix + quoteJQKey(selected)
		}
		return prefix + "." + quoteJQKey(selected)
	}
	return prefix + selected
}

func isArrayHint(s string) bool {
	if len(s) < 3 || s[0] != '[' || s[len(s)-1] != ']' {
		return false
	}
	for i := 1; i < len(s)-1; i++ {
		if s[i] < '0' || s[i] > '9' {
			return false
		}
	}
	return true
}

func quoteJQKey(key string) string {
	var quoted strings.Builder
	quoted.Grow(len(key) + 2)
	quoted.WriteByte('"')
	for i := 0; i < len(key); i++ {
		b := key[i]
		switch b {
		case '\\', '"':
			quoted.WriteByte('\\')
			quoted.WriteByte(b)
		default:
			if b < 0x20 {
				fmt.Fprintf(&quoted, "\\u%04X", b)
			} else {
				quoted.WriteByte(b)
			}
		}
	}
	quoted.WriteByte('"')
	return quoted.String()
}

func isJQIdentifier(key string) bool {
	if key == "" {
		return false
	}
	for i := 0; i < len(key); i++ {
		b := key[i]
		if i == 0 {
			if !isASCIIAlpha(b) && b != '_' {
				return false
			}
		} else if !isASCIIAlpha(b) && (b < '0' || b > '9') && b != '_' {
			return false
		}
	}
	return true
}

func isASCIIAlpha(b byte) bool { return b >= 'a' && b <= 'z' || b >= 'A' && b <= 'Z' }
