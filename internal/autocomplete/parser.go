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
	code := jqCodeMask(filter)
	pipeIdx := findLastPipeOutsideString(filter, code)
	expression := filter
	offset := 0
	if pipeIdx >= 0 {
		expression = filter[pipeIdx+1:]
		offset = pipeIdx + 1
	}
	offset += len(expression) - len(strings.TrimLeftFunc(expression, unicode.IsSpace))
	workingFilter := strings.TrimSpace(expression)

	// Find the last dot that starts a key
	lastDot := findLastKeyDot(workingFilter, code[offset:offset+len(workingFilter)])
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

func findLastPipeOutsideString(s string, code []bool) int {
	lastPipe := -1
	for i := 0; i < len(s); i++ {
		if code[i] && s[i] == '|' {
			lastPipe = i
		}
	}
	return lastPipe
}

// jqCodeMask marks jq expressions, including expressions inside string interpolation.
func jqCodeMask(s string) []bool {
	type frame struct {
		kind   byte // c: root code, s: string, i: interpolation code
		parens int
	}
	mask := make([]bool, len(s))
	stack := []frame{{kind: 'c'}}
	for i := 0; i < len(s); i++ {
		current := &stack[len(stack)-1]
		if current.kind == 's' {
			if s[i] == '\\' && i+1 < len(s) {
				if s[i+1] == '(' {
					stack = append(stack, frame{kind: 'i', parens: 1})
					i++
				} else {
					i++
				}
			} else if s[i] == '"' {
				stack = stack[:len(stack)-1]
			}
			continue
		}
		mask[i] = true
		if s[i] == '"' {
			stack = append(stack, frame{kind: 's'})
		} else if current.kind == 'i' {
			switch s[i] {
			case '(':
				current.parens++
			case ')':
				current.parens--
				if current.parens == 0 {
					stack = stack[:len(stack)-1]
				}
			}
		}
	}
	return mask
}

// findLastKeyDot finds the last '.' that starts a key access
// (not inside brackets)
func findLastKeyDot(s string, code []bool) int {
	bracketDepth := 0

	for i := len(s) - 1; i >= 0; i-- {
		if !code[i] {
			continue
		}
		switch s[i] {
		case ']':
			bracketDepth++
		case '[':
			bracketDepth--
		case '.':
			if bracketDepth == 0 {
				return i
			}
		}
	}
	return -1
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
