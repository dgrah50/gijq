package ui

import (
	"sort"
	"strings"
	"unicode/utf8"

	"github.com/mattn/go-runewidth"
)

// tokKind classifies a rune for syntax highlighting.
type tokKind uint8

const (
	tkPlain tokKind = iota
	tkKey
	tkString
	tkNumber
	tkBool
	tkNull
	tkBracket
)

// tokColor maps a token kind to its ANSI colour prefix ("" means uncoloured).
var tokColor = [...]string{
	tkPlain:   "",
	tkKey:     ansiCyan,
	tkString:  ansiGreen,
	tkNumber:  ansiYellow,
	tkBool:    ansiMagenta,
	tkNull:    ansiGray,
	tkBracket: ansiWhite,
}

type lexMode uint8

const (
	modeBetween lexMode = iota // between tokens
	modeString                 // inside a string literal (kind is key or string)
	modeNumber                 // inside a number
	modeWord                   // inside a bare word (true/false/null/other)
)

// lexState is the resumable state of the JSON lexer. It describes what the
// next rune is part of, so lexing can restart at any rune boundary.
type lexState struct {
	mode    lexMode
	kind    tokKind // classification of the current token (string/number/word)
	escaped bool    // previous rune in a string was an unescaped backslash
}

// step advances the lexer over rune r found at line[pos:] and returns the
// token kind r belongs to. Any input is accepted; unexpected runes are plain.
func (s *lexState) step(line string, pos int, r rune) tokKind {
	switch s.mode {
	case modeString:
		switch {
		case s.escaped:
			s.escaped = false
		case r == '\\':
			s.escaped = true
		case r == '"':
			s.mode = modeBetween
		}
		return s.kind
	case modeNumber:
		if isNumberRune(r) {
			return tkNumber
		}
		s.mode = modeBetween
	case modeWord:
		if isLitRune(r) {
			return s.kind
		}
		s.mode = modeBetween
	}

	switch {
	case r == '"':
		s.mode = modeString
		s.escaped = false
		s.kind = classifyString(line, pos)
		return s.kind
	case r == '{' || r == '}' || r == '[' || r == ']':
		return tkBracket
	case r == '-' || (r >= '0' && r <= '9'):
		s.mode = modeNumber
		s.kind = tkNumber
		return tkNumber
	case isLitStart(r):
		s.mode = modeWord
		s.kind = classifyWord(line, pos)
		return s.kind
	}
	return tkPlain
}

func isNumberRune(r rune) bool {
	return (r >= '0' && r <= '9') || r == '.' || r == '+' || r == '-' || r == 'e' || r == 'E'
}

func isLitStart(r rune) bool {
	return (r >= 'a' && r <= 'z') || (r >= 'A' && r <= 'Z')
}

func isLitRune(r rune) bool {
	return isLitStart(r) || (r >= '0' && r <= '9') || r == '_'
}

func isJSONSpace(c byte) bool {
	return c == ' ' || c == '\t' || c == '\n' || c == '\r'
}

// classifyWord looks ahead from the start of a bare word and reports whether
// it is exactly true/false/null.
func classifyWord(line string, pos int) tokKind {
	end := pos
	for end < len(line) && end-pos <= 5 {
		c := line[end]
		if !(c < utf8.RuneSelf && isLitRune(rune(c))) {
			break
		}
		end++
	}
	// A longer word (still running at end) is never a literal.
	if end < len(line) && end-pos > 5 {
		return tkPlain
	}
	if end < len(line) && line[end] < utf8.RuneSelf && isLitRune(rune(line[end])) {
		return tkPlain
	}
	switch line[pos:end] {
	case "true", "false":
		return tkBool
	case "null":
		return tkNull
	}
	return tkPlain
}

// classifyString looks ahead from the opening quote at line[pos] to decide
// whether the string is an object key (closing quote followed by optional
// whitespace and ':') or a plain string value.
func classifyString(line string, pos int) tokKind {
	i := pos + 1
	for {
		j := strings.IndexByte(line[i:], '"')
		if j < 0 {
			return tkString
		}
		q := i + j
		bs := 0
		for k := q - 1; k > pos && line[k] == '\\'; k-- {
			bs++
		}
		i = q + 1
		if bs%2 == 0 {
			break
		}
	}
	for i < len(line) && isJSONSpace(line[i]) {
		i++
	}
	if i < len(line) && line[i] == ':' {
		return tkKey
	}
	return tkString
}

// runeWidth is the single display-width function used for layout: ASCII is
// one column, otherwise runewidth.RuneWidth (combining marks take zero).
func runeWidth(r rune) int {
	if r < 0x80 {
		return 1
	}
	return runewidth.RuneWidth(r)
}

func decodeRune(s string, pos int) (rune, int) {
	if c := s[pos]; c < utf8.RuneSelf {
		return rune(c), 1
	}
	return utf8.DecodeRuneInString(s[pos:])
}

// hlCheckpoint records the lexer position at a rune boundary of a long line.
type hlCheckpoint struct {
	off   int // byte offset
	col   int // display column at off
	state lexState
}

const (
	hlStride   = 4096 // bytes between checkpoints
	hlMinLong  = 4096 // lines at least this long get checkpoints
	hlMaxLines = 64   // bound on cached long lines
)

// buildCheckpoints lexes line once and records a checkpoint about every
// hlStride bytes.
func buildCheckpoints(line string) []hlCheckpoint {
	cps := make([]hlCheckpoint, 0, len(line)/hlStride+1)
	var st lexState
	col, next := 0, hlStride
	for pos := 0; pos < len(line); {
		if pos >= next {
			cps = append(cps, hlCheckpoint{off: pos, col: col, state: st})
			next = pos + hlStride
		}
		r, size := decodeRune(line, pos)
		st.step(line, pos, r)
		col += runeWidth(r)
		pos += size
	}
	return cps
}

// prebuildCheckpoints computes checkpoints for every long line. It is CPU
// heavy for large minified outputs and must run off the UI goroutine.
func prebuildCheckpoints(lines []string) map[int][]hlCheckpoint {
	var out map[int][]hlCheckpoint
	for i, line := range lines {
		if len(line) < hlMinLong {
			continue
		}
		if out == nil {
			out = make(map[int][]hlCheckpoint)
		}
		out[i] = buildCheckpoints(line)
	}
	return out
}

// highlightCache holds checkpoints for long lines, keyed by line index:
// pre is built in the background when a result arrives (never evicted);
// cps holds lazily built fallbacks. It is reset whenever a new result is
// accepted.
type highlightCache struct {
	pre map[int][]hlCheckpoint
	cps map[int][]hlCheckpoint
}

func newHighlightCache() *highlightCache {
	return &highlightCache{cps: make(map[int][]hlCheckpoint)}
}

func (c *highlightCache) reset() {
	if c == nil {
		return
	}
	c.pre = nil
	clear(c.cps)
}

// resetWith resets the cache and seeds it with prebuilt checkpoints.
func (c *highlightCache) resetWith(pre map[int][]hlCheckpoint) {
	if c == nil {
		return
	}
	c.reset()
	c.pre = pre
}

// forLine returns checkpoints for line idx, or nil for short lines.
func (c *highlightCache) forLine(idx int, line string) []hlCheckpoint {
	if c == nil || len(line) < hlMinLong {
		return nil
	}
	if cps, ok := c.pre[idx]; ok {
		return cps
	}
	if cps, ok := c.cps[idx]; ok {
		return cps
	}
	if len(c.cps) >= hlMaxLines {
		clear(c.cps)
	}
	cps := buildCheckpoints(line)
	c.cps[idx] = cps
	return cps
}

// highlightWindow renders the part of line visible in a window of width
// columns starting at display column xOffset, including "…" markers for cut
// sides. If color is false the output is plain text. cps (optional, sorted)
// lets lexing resume near xOffset. It returns the rendered text, its display
// width, and whether content was cut on the left / right.
func highlightWindow(line string, xOffset, width int, color bool, cps []hlCheckpoint) (string, int, bool, bool) {
	if xOffset < 0 {
		xOffset = 0
	}
	leftCut := xOffset > 0
	if width <= 0 {
		return "", 0, leftCut, len(line) > 0
	}
	if len(line) == 0 {
		if leftCut {
			return "…", 1, true, false
		}
		return "", 0, false, false
	}

	// Skip to the first rune that ends past xOffset (a wide rune straddling
	// the edge is shown whole), lexing along the way for state.
	pos, col := 0, 0
	var st lexState
	if xOffset > 0 {
		if color && len(cps) > 0 {
			i := sort.Search(len(cps), func(i int) bool { return cps[i].col > xOffset })
			if i > 0 {
				cp := cps[i-1]
				pos, col, st = cp.off, cp.col, cp.state
			}
		}
		for pos < len(line) {
			r, size := decodeRune(line, pos)
			w := runeWidth(r)
			if col+w > xOffset {
				break
			}
			if color {
				st.step(line, pos, r)
			}
			col += w
			pos += size
		}
	}
	start := pos

	// Find the end of the window and whether the right side is cut.
	visible, end, rightCut := 0, start, false
	for end < len(line) {
		r, size := decodeRune(line, end)
		w := runeWidth(r)
		if visible+w > width {
			rightCut = true
			break
		}
		visible += w
		end += size
	}

	budget := width
	if leftCut || rightCut {
		if width == 1 {
			return "…", 1, leftCut, rightCut
		}
		budget = width - 1
		if leftCut && rightCut {
			budget = width - 2
		}
	}

	var sb strings.Builder
	sb.Grow(end - start + 32)
	outW := 0
	if leftCut {
		sb.WriteString("…")
		outW++
	}
	cur := ""
	anyEsc := false
	cum := 0
	for pos = start; pos < end; {
		r, size := decodeRune(line, pos)
		w := runeWidth(r)
		if cum+w > budget {
			break
		}
		if color {
			c := tokColor[st.step(line, pos, r)]
			if c != cur {
				if cur != "" {
					sb.WriteString(ansiReset)
				}
				if c != "" {
					sb.WriteString(c)
					anyEsc = true
				}
				cur = c
			}
		}
		sb.WriteString(line[pos : pos+size])
		cum += w
		pos += size
	}
	outW += cum
	if cur != "" {
		sb.WriteString(ansiReset)
	}
	if rightCut {
		sb.WriteString("…")
		outW++
	}
	// Any colour use ends with a reset so ANSI never leaks into the border.
	if anyEsc && !strings.HasSuffix(sb.String(), ansiReset) {
		sb.WriteString(ansiReset)
	}
	return sb.String(), outW, leftCut, rightCut
}
