package autocomplete

import (
	"strings"

	"github.com/itchyny/gojq"
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
	pipeIdx := strings.LastIndex(filter, "|")
	workingFilter := filter
	offset := 0
	if pipeIdx >= 0 {
		workingFilter = strings.TrimSpace(filter[pipeIdx+1:])
		offset = pipeIdx + 1 + (len(filter[pipeIdx+1:]) - len(strings.TrimSpace(filter[pipeIdx+1:])))
	}

	// Find the last dot that starts a key
	lastDot := findLastKeyDot(workingFilter)
	if lastDot < 0 {
		// No dot found, treat as incomplete from start
		if workingFilter == "" || workingFilter == "." {
			return Context{Path: ".", Incomplete: "", StartPos: offset + len(workingFilter)}
		}
		return Context{Path: ".", Incomplete: strings.TrimPrefix(workingFilter, "."), StartPos: offset + 1}
	}

	path := workingFilter[:lastDot]
	incomplete := workingFilter[lastDot+1:]

	// Validate the path with gojq
	if path == "" {
		path = "."
	}
	if !isValidPath(path) {
		// Path invalid, try without the last segment
		return Context{Path: ".", Incomplete: workingFilter, StartPos: offset}
	}

	return Context{
		Path:       path,
		Incomplete: incomplete,
		StartPos:   offset + lastDot + 1,
	}
}

// findLastKeyDot finds the last '.' that starts a key access
// (not inside brackets)
func findLastKeyDot(s string) int {
	bracketDepth := 0
	lastDot := -1

	for i := len(s) - 1; i >= 0; i-- {
		switch s[i] {
		case ']':
			bracketDepth++
		case '[':
			bracketDepth--
		case '.':
			if bracketDepth == 0 {
				lastDot = i
				// Check if this is a valid split point
				path := s[:i]
				if path == "" || isValidPath(path) {
					return i
				}
			}
		}
	}
	return lastDot
}

func isValidPath(path string) bool {
	if path == "" || path == "." {
		return true
	}
	_, err := gojq.Parse(path)
	return err == nil
}
