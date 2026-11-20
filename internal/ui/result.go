package ui

import (
	"fmt"
	"strings"
	"sync"

	"github.com/dayangraham/gijq/internal/jsonview"
)

// resultView is the result shown for one evaluated filter. It is replaced
// (never mutated in place by the view) as collector and index snapshots
// arrive; only Update changes it.
//
// Exactly one of three shapes holds:
//   - nothing yet: snap == nil && err == nil (empty pane)
//   - error: err != nil. The whole result is the error text (errWin); any
//     values collected before the error are not shown.
//   - values: snap != nil. The text is snap.Doc rendered lazily.
//
// Line counts: known is a lower bound from the first screen; the index
// (built once collection is Done) refines it to the exact total.
type resultView struct {
	filter string // filter that produced it

	snap  *jsonview.Snapshot // latest collector snapshot
	index *jsonview.Index    // latest index of snap.Doc; nil until built

	err    error
	errWin *window // error text as lines

	known      int  // lines certainly present (first screen)
	knownExact bool // known is the total (Done and fewer lines than asked for)

	// seenWidth is the widest line among the windows the user has had on
	// screen: a lower bound for horizontal scrolling until the index is
	// complete. Only Update raises it.
	seenWidth int

	cache *visibleCache
}

func newResultView() resultView { return resultView{cache: &visibleCache{}} }

// begin starts a new result for filter, dropping the previous one.
func (r *resultView) begin(filter string) {
	*r = resultView{filter: filter, cache: r.cache}
}

// setError makes err the whole result.
func (r *resultView) setError(err error) {
	r.snap, r.index = nil, nil
	r.known, r.knownExact, r.seenWidth = 0, false, 0
	r.err = err
	lines := strings.Split(err.Error(), "\n")
	w := &window{lines: lines, widths: make([]int, len(lines))}
	for i, l := range lines {
		w.widths[i] = lineDisplayWidth(l)
	}
	r.errWin = w
}

// setSnapshot adopts a collector snapshot. want is the number of lines the
// first screen should hold.
func (r *resultView) setSnapshot(snap *jsonview.Snapshot, want int) {
	r.snap = snap
	if !r.knownExact && r.known < want {
		n := len(snap.Doc.Lines(nil, 0, want))
		r.known = n
		r.knownExact = snap.Done && n < want
	}
}

// setIndex adopts an index snapshot of the current snap.Doc.
func (r *resultView) setIndex(ix *jsonview.Index) { r.index = ix }

// hasError reports whether the whole result is an error.
func (r *resultView) hasError() bool { return r.err != nil }

// lineCount returns the number of lines known so far and whether it is the
// exact total.
func (r *resultView) lineCount() (n int, exact bool) {
	if r.err != nil {
		return len(r.errWin.lines), true
	}
	if r.snap == nil {
		return 0, true
	}
	n, exact = r.known, r.knownExact
	if ix := r.index; ix != nil {
		n = max(n, ix.Lines())
		exact = exact || ix.Complete()
	}
	return n, exact
}

// maxWidth returns a lower bound of the widest line (display cells) and
// whether it is exact.
func (r *resultView) maxWidth() (w int, exact bool) {
	if r.err != nil {
		return r.errWin.maxWidth(), true
	}
	w = r.seenWidth
	if ix := r.index; ix != nil {
		w = max(w, ix.MaxWidth())
		exact = ix.Complete()
	}
	return w, exact
}

// observe raises the lower bound of the widest line.
func (r *resultView) observe(width int) { r.seenWidth = max(r.seenWidth, width) }

// complete reports whether all values were collected without error.
func (r *resultView) complete() bool {
	return r.err == nil && r.snap != nil && r.snap.Done && r.snap.Err == nil
}

// numValues is the number of values collected so far and whether it is final.
func (r *resultView) numValues() (n int, exact bool) {
	if r.snap == nil {
		return 0, r.err != nil
	}
	return len(r.snap.Values), r.complete()
}

// window returns up to n lines starting at line start. It is a pure function
// of the result, memoised in the visibleCache.
func (r *resultView) window(start, n int) *window {
	if start < 0 {
		start = 0
	}
	if r.err != nil {
		return r.errWin.slice(start, n)
	}
	if r.snap == nil || n <= 0 {
		return &window{}
	}
	doc := r.snap.Doc
	if c := r.cache; c != nil {
		if w := c.get(doc, start, n); w != nil {
			return w
		}
	}
	lines := doc.Lines(r.index, start, n)
	w := &window{lines: lines, widths: make([]int, len(lines))}
	for i, l := range lines {
		w.widths[i] = lineDisplayWidth(l)
	}
	if c := r.cache; c != nil {
		c.put(doc, start, n, w)
	}
	return w
}

// summary is the footer text describing the size of the result. Counts that
// may still grow end with "+".
func (r *resultView) summary() string {
	if r.err != nil || r.snap == nil {
		return ""
	}
	var parts []string
	if nv, exact := r.numValues(); !exact || nv != 1 {
		parts = append(parts, "results: "+groupDigits(nv)+plus(exact))
	}
	nl, exact := r.lineCount()
	if exact {
		parts = append(parts, "lines: "+groupDigits(nl))
	} else if nl >= 1_000_000 {
		parts = append(parts, fmt.Sprintf("lines: %.1fM+", float64(nl)/1e6))
	} else {
		parts = append(parts, "lines: "+groupDigits(nl)+"+")
	}
	return strings.Join(parts, "  ")
}

func plus(exact bool) string {
	if exact {
		return ""
	}
	return "+"
}

// groupDigits formats n with thousands separators.
func groupDigits(n int) string {
	s := fmt.Sprint(n)
	if len(s) <= 3 {
		return s
	}
	var b strings.Builder
	for i, c := range s {
		if i > 0 && (len(s)-i)%3 == 0 {
			b.WriteByte(',')
		}
		b.WriteRune(c)
	}
	return b.String()
}

// lineDisplayWidth is the display width of line in cells (see runeWidth).
func lineDisplayWidth(line string) int {
	for i := 0; i < len(line); i++ {
		if line[i] >= 0x80 {
			w := 0
			for _, r := range line {
				w += runeWidth(r)
			}
			return w
		}
	}
	return len(line)
}

// window is a run of consecutive output lines with their display widths.
// Highlight checkpoints of long lines are built on first use (see
// checkpoints); that is the only mutation after construction.
type window struct {
	lines  []string
	widths []int

	mu  sync.Mutex
	cps map[int][]hlCheckpoint
}

func (w *window) maxWidth() int {
	m := 0
	for _, x := range w.widths {
		m = max(m, x)
	}
	return m
}

// slice returns the sub-window [start, start+n) (error text only).
func (w *window) slice(start, n int) *window {
	if start > len(w.lines) {
		start = len(w.lines)
	}
	end := min(start+max(n, 0), len(w.lines))
	return &window{lines: w.lines[start:end], widths: w.widths[start:end]}
}

// checkpoints returns the highlight checkpoints of line i, or nil for short
// lines. They are only needed when scrolled right, so callers ask lazily.
func (w *window) checkpoints(i int) []hlCheckpoint {
	if len(w.lines[i]) < hlMinLong {
		return nil
	}
	w.mu.Lock()
	defer w.mu.Unlock()
	if cps, ok := w.cps[i]; ok {
		return cps
	}
	if w.cps == nil {
		w.cps = make(map[int][]hlCheckpoint)
	}
	cps := buildCheckpoints(w.lines[i])
	w.cps[i] = cps
	return cps
}

// visibleCache memoises the most recent window. It is pure memoisation: the
// window for (doc, start, n) is a deterministic function of its key, so a
// hit returns exactly what a miss would compute. It holds one entry, so it
// is bounded by one screenful of lines (plus checkpoints of long lines), and
// repeated frames (cursor blink, typing in the footer) neither regenerate nor
// re-lex a multi-megabyte line.
type visibleCache struct {
	mu    sync.Mutex
	doc   *jsonview.Doc
	start int
	n     int
	win   *window
}

func (c *visibleCache) get(doc *jsonview.Doc, start, n int) *window {
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.win != nil && c.doc == doc && c.start == start && c.n == n {
		return c.win
	}
	return nil
}

func (c *visibleCache) put(doc *jsonview.Doc, start, n int, w *window) {
	c.mu.Lock()
	c.doc, c.start, c.n, c.win = doc, start, n, w
	c.mu.Unlock()
}
