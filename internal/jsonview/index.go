package jsonview

import (
	"bufio"
	"context"
	"io"
	"time"

	"github.com/mattn/go-runewidth"
)

// DefaultStep is the number of lines between index checkpoints.
const DefaultStep = 1024

// checkpoint is the printer state at the start of line `line`.
type checkpoint struct {
	line   int
	vi     int
	frames []frame
}

// Index is an immutable snapshot of indexing progress for one Doc. Partial
// snapshots (Complete() == false) are safe to use: they cover the first
// Lines() lines. Builders append to a shared checkpoint array but never
// modify elements a published snapshot can see.
type Index struct {
	doc      *Doc
	step     int
	cps      []checkpoint // cps[i] is the state at line i*step
	lines    int          // lines walked so far (total when complete)
	maxWidth int          // widest line walked so far (display cells)
	complete bool
}

// Lines returns the number of lines counted so far; exact when Complete.
func (ix *Index) Lines() int { return ix.lines }

// MaxWidth returns the widest line seen so far in display cells.
func (ix *Index) MaxWidth() int { return ix.maxWidth }

// Complete reports whether the whole document was walked.
func (ix *Index) Complete() bool { return ix.complete }

// lineWidth is the display width of a line: len for printable ASCII, else the sum of runewidth.RuneWidth per rune.
func lineWidth(b []byte) int {
	for _, c := range b {
		if c >= 0x7f || c < 0x20 {
			w := 0
			for _, r := range string(b) {
				w += runewidth.RuneWidth(r)
			}
			return w
		}
	}
	return len(b)
}

// BuildIndex walks the whole output without allocating strings, taking a
// checkpoint every step lines (DefaultStep if step <= 0). If publish is not
// nil it is called with partial snapshots at most every ~50ms. The final
// (or, on cancellation, partial) Index is returned along with ctx.Err().
func (d *Doc) BuildIndex(ctx context.Context, step int, publish func(*Index)) (*Index, error) {
	if step <= 0 {
		step = DefaultStep
	}
	p := d.newPrinter()
	var cps []checkpoint
	var buf []byte
	line, maxW := 0, 0
	last := time.Now()
	snap := func(complete bool) *Index {
		return &Index{doc: d, step: step, cps: cps[:len(cps):len(cps)],
			lines: line, maxWidth: maxW, complete: complete}
	}
	for {
		if line%step == 0 {
			cps = append(cps, p.snapshot(line))
			if line&(step*4-1) == 0 {
				if err := ctx.Err(); err != nil {
					return snap(false), err
				}
				if publish != nil && time.Since(last) > 50*time.Millisecond {
					publish(snap(false))
					last = time.Now()
				}
			}
		}
		var ok bool
		buf, ok = p.next(buf[:0])
		if !ok {
			break
		}
		if w := lineWidth(buf); w > maxW {
			maxW = w
		}
		line++
	}
	// A checkpoint exactly at the end would point past the last line; drop it
	// so every checkpoint refers to an existing line.
	if line > 0 && (line-1)/step+1 < len(cps) {
		cps = cps[:(line-1)/step+1]
	}
	ix := snap(true)
	if publish != nil {
		publish(ix)
	}
	return ix, nil
}

// Lines returns up to n lines starting at line start (0-based), formatting
// only what is needed. ix may be nil (or built for another Doc), in which
// case printing starts at the top; otherwise it starts at the nearest
// checkpoint at or before start, so deep lines cost at most one step of
// skipped lines when the index covers them (and the distance from the last
// covered checkpoint when it is still partial).
func (d *Doc) Lines(ix *Index, start, n int) []string {
	if start < 0 {
		start = 0
	}
	if n <= 0 {
		return nil
	}
	p := d.newPrinter()
	line := 0
	if ix != nil && ix.doc == d && len(ix.cps) > 0 {
		i := start / ix.step
		if i >= len(ix.cps) {
			i = len(ix.cps) - 1
		}
		p.restore(&ix.cps[i])
		line = ix.cps[i].line
	}
	var buf []byte
	var ok bool
	for ; line < start; line++ {
		if buf, ok = p.next(buf[:0]); !ok {
			return nil
		}
	}
	out := make([]string, 0, min(n, 4096))
	for len(out) < n {
		if buf, ok = p.next(buf[:0]); !ok {
			break
		}
		out = append(out, string(buf))
	}
	return out
}

// WriteTo streams the complete text (lines joined by "\n", no trailing
// newline) to w without materialising it.
func (d *Doc) WriteTo(w io.Writer) (int64, error) {
	bw := bufio.NewWriterSize(w, 64<<10)
	p := d.newPrinter()
	var buf []byte
	var total int64
	first := true
	for {
		var ok bool
		buf, ok = p.next(buf[:0])
		if !ok {
			break
		}
		if !first {
			if err := bw.WriteByte('\n'); err != nil {
				return total, err
			}
			total++
		}
		first = false
		n, err := bw.Write(buf)
		total += int64(n)
		if err != nil {
			return total, err
		}
	}
	return total, bw.Flush()
}

// countLines returns the number of lines of value i, stopping at limit.
func (d *Doc) countLines(i, limit int) int {
	p := d.newPrinter()
	p.vi = i
	var buf []byte
	n := 0
	for p.vi == i && n < limit {
		var ok bool
		if buf, ok = p.next(buf[:0]); !ok {
			break
		}
		n++
	}
	return n
}
