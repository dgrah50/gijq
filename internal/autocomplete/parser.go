package autocomplete

import (
	"strings"
	"unicode"
)

// Context represents the parsed autocomplete context
type Context struct {
	Path       string // Valid jq path prefix
	Incomplete string // Partial key being typed
	StartPos   int    // Where incomplete begins in filter
}

// Parse extracts autocomplete context from a filter string
func Parse(filter string) Context {
	if filter == "" {
		return Context{Path: ".", Incomplete: "", StartPos: 0}
	}

	// Find the last segment to autocomplete
	// Look for last pipe first (indicates new expression)
	pipeIdx := findLastPipe(filter)
	expression := filter
	offset := 0
	if pipeIdx >= 0 {
		expression = filter[pipeIdx+1:]
		offset = pipeIdx + 1
	}
	offset += len(expression) - len(strings.TrimLeftFunc(expression, unicode.IsSpace))
	workingFilter := strings.TrimSpace(expression)

	// Find the last dot that starts a key
	lastDot := findLastKeyDot(workingFilter)
	if lastDot == 0 && strings.IndexFunc(workingFilter[1:], unicode.IsSpace) >= 0 {
		return Context{Path: ".", Incomplete: workingFilter, StartPos: offset}
	}
	if lastDot < 0 {
		// No dot found, treat as incomplete from start
		if workingFilter == "" || workingFilter == "." {
			return Context{Path: ".", Incomplete: "", StartPos: offset + len(workingFilter)}
		}
		if trimmed, ok := strings.CutPrefix(workingFilter, "."); ok {
			return Context{Path: ".", Incomplete: trimmed, StartPos: offset + 1}
		}
		return Context{Path: ".", Incomplete: workingFilter, StartPos: offset}
	}

	path := workingFilter[:lastDot]
	incomplete := workingFilter[lastDot+1:]

	if path == "" {
		path = "."
	}
	if !isLikelyPath(path) {
		// Path invalid, try without the last segment
		return Context{Path: ".", Incomplete: workingFilter, StartPos: offset}
	}

	return Context{
		Path:       path,
		Incomplete: incomplete,
		StartPos:   offset + lastDot + 1,
	}
}

// findLastPipe returns the last pipe outside a quoted string.
func findLastPipe(s string) int {
	quote := byte(0)
	escaped := false
	lastPipe := -1
	for i := 0; i < len(s); i++ {
		c := s[i]
		if quote != 0 {
			if escaped {
				escaped = false
			} else if c == '\\' {
				escaped = true
			} else if c == quote {
				quote = 0
			}
			continue
		}
		if c == '"' || c == '\'' {
			quote = c
		} else if c == '|' {
			lastPipe = i
		}
	}
	return lastPipe
}

// findLastKeyDot finds the last '.' that starts a key access
// (not inside brackets)
func findLastKeyDot(s string) int {
	quote := byte(0)
	escaped := false
	bracketDepth := 0
	lastDot := -1

	for i := 0; i < len(s); i++ {
		c := s[i]
		if quote != 0 {
			if escaped {
				escaped = false
			} else if c == '\\' {
				escaped = true
			} else if c == quote {
				quote = 0
			}
			continue
		}
		if c == '"' || c == '\'' {
			quote = c
			continue
		}
		switch c {
		case '[':
			bracketDepth++
		case ']':
			if bracketDepth > 0 {
				bracketDepth--
			}
		case '.':
			if bracketDepth == 0 && !(i > 0 && i+1 < len(s) && s[i-1] >= '0' && s[i-1] <= '9' && s[i+1] >= '0' && s[i+1] <= '9') {
				lastDot = i
			}
		}
	}
	return lastDot
}

func isLikelyPath(path string) bool {
	if path == "" || path == "." {
		return true
	}
	if !strings.HasPrefix(path, ".") {
		return false
	}

	depth := 0
	for i := 0; i < len(path); i++ {
		switch path[i] {
		case '[':
			depth++
		case ']':
			depth--
			if depth < 0 {
				return false
			}
		}
	}
	return depth == 0
}
