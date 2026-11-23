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
	lastPipe := -1
	scanUnquoted(s, func(i int, c byte) bool {
		if c == '|' {
			lastPipe = i
		}
		return true
	})
	return lastPipe
}

func scanUnquoted(s string, visit func(int, byte) bool) {
	quoted, escaped := false, false
	for i := 0; i < len(s); i++ {
		c := s[i]
		if quoted {
			if escaped {
				escaped = false
			} else if c == '\\' {
				escaped = true
			} else if c == '"' {
				quoted = false
			}
			continue
		}
		if c == '"' {
			quoted = true
			continue
		}
		if !visit(i, c) {
			return
		}
	}
}

// findLastKeyDot finds the last '.' that starts a key access
// (not inside brackets, strings, or numeric literals).
func findLastKeyDot(s string) int {
	bracketDepth := 0
	lastDot := -1
	scanUnquoted(s, func(i int, c byte) bool {
		switch c {
		case '[':
			bracketDepth++
		case ']':
			bracketDepth--
		case '.':
			if bracketDepth == 0 && !isNumberDot(s, i) {
				lastDot = i
			}
		}
		return true
	})
	return lastDot
}

func isNumberDot(s string, i int) bool {
	j := i - 1
	for j >= 0 && s[j] >= '0' && s[j] <= '9' {
		j--
	}
	if j == i-1 {
		return false
	}
	return j < 0 || (!isIdentifierByte(s[j]) && s[j] != '.' && s[j] != ']' && s[j] != ')' && s[j] != '"')
}

func isIdentifierByte(c byte) bool {
	return c == '_' || c >= 'a' && c <= 'z' || c >= 'A' && c <= 'Z' || c >= '0' && c <= '9'
}

func isLikelyPath(path string) bool {
	if path == "" || path == "." {
		return true
	}
	if !strings.HasPrefix(path, ".") {
		return false
	}

	depth := 0
	valid := true
	scanUnquoted(path, func(_ int, c byte) bool {
		switch c {
		case '[':
			depth++
		case ']':
			depth--
			if depth < 0 {
				valid = false
				return false
			}
		}
		return true
	})
	return valid && depth == 0
}
