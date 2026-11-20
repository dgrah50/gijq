// Package jsonview renders jq results lazily.
//
// A Doc is an immutable sequence of jq result values whose pretty-printed
// text (identical, byte for byte, to encoding/json with SetIndent("", "  ")
// and SetEscapeHTML(false), values separated by one "\n") can be produced
// line by line without formatting the whole thing. An Index, built in the
// background, adds O(K) random access, the total line count and the widest
// line.
//
// Concurrency model: Doc, Index and Snapshot are immutable once published
// and may be shared freely between goroutines. The only internal mutation is
// memoisation (per-value classification, sorted keys of very large maps),
// which is guarded by sync primitives. Producers publish new values rather
// than mutating old ones.
package jsonview

import (
	"reflect"
	"slices"
	"sync"
)

// bigMapKeys is the size from which sorted key slices are cached per map.
const bigMapKeys = 2048

type shared struct {
	keys sync.Map // uintptr(map pointer) -> []string, for big maps
}

// Doc is an immutable sequence of jq result values.
type Doc struct {
	values []any
	shared *shared
}

// NewDoc creates a Doc over values. The slice must not be modified afterwards.
func NewDoc(values []any) *Doc {
	return &Doc{values: values, shared: &shared{}}
}

// Extend returns a new Doc with more values appended; d is unchanged.
func (d *Doc) Extend(more []any) *Doc {
	vals := make([]any, 0, len(d.values)+len(more))
	vals = append(append(vals, d.values...), more...)
	return &Doc{values: vals, shared: d.shared}
}

// NumValues returns the number of top-level values.
func (d *Doc) NumValues() int { return len(d.values) }

// keysFor returns the keys of m in Go string order (what encoding/json
// uses). Keys of big maps come from a shared cache (immutable, owned=false).
// Small maps are sorted into *scratch, a buffer the printer reuses per depth
// (owned=true: the slice is only valid until that depth is reused, and must
// be copied before being stored in a checkpoint).
func (d *Doc) keysFor(m map[string]any, scratch *[]string) (keys []string, scratchBacked bool) {
	if len(m) >= bigMapKeys {
		id := reflect.ValueOf(m).Pointer()
		if k, ok := d.shared.keys.Load(id); ok {
			return k.([]string), false
		}
		k := make([]string, 0, len(m))
		for key := range m {
			k = append(k, key)
		}
		slices.Sort(k)
		d.shared.keys.Store(id, k)
		return k, false
	}
	k := (*scratch)[:0]
	for key := range m {
		k = append(k, key)
	}
	slices.Sort(k)
	*scratch = k
	return k, true
}

// frame is one open container of a printer. Frames hold references only
// (the container and its sorted keys), so copying them is cheap.
type frame struct {
	arr   []any
	m     map[string]any
	keys  []string // sorted keys when m != nil
	pos   int      // next element to print
	comma bool     // closing bracket is followed by a comma
	// scratch means keys aliases the printer's per-depth scratch buffer and
	// must be cloned before leaving the printer (see snapshot).
	scratch bool
}

func (f *frame) isMap() bool { return f.m != nil }

func (f *frame) length() int {
	if f.m != nil {
		return len(f.keys)
	}
	return len(f.arr)
}

// printer is a resumable line generator. Its whole state (vi, fbPos, stack)
// is what an index checkpoint stores.
type printer struct {
	doc   *Doc
	vi    int // current top-level value
	stack []frame
	// keyBufs[depth] is reused for the sorted keys of maps opened at that
	// stack depth during sequential walking.
	keyBufs [][]string
}

func (d *Doc) newPrinter() *printer { return &printer{doc: d} }

func (p *printer) restore(c *checkpoint) {
	p.vi = c.vi
	p.stack = append(p.stack[:0], c.frames...)
}

func (p *printer) snapshot(line int) checkpoint {
	frames := append([]frame(nil), p.stack...)
	for i := range frames {
		if frames[i].scratch {
			frames[i].keys = slices.Clone(frames[i].keys)
			frames[i].scratch = false
		}
	}
	return checkpoint{line: line, vi: p.vi, frames: frames}
}

func appendIndent(dst []byte, depth int) []byte {
	for ; depth > 0; depth-- {
		dst = append(dst, ' ', ' ')
	}
	return dst
}

// next appends the next output line (without newline) to dst.
func (p *printer) next(dst []byte) ([]byte, bool) {
	d := p.doc
	for {
		if len(p.stack) == 0 {
			if p.vi >= len(d.values) {
				return dst, false
			}
			dst = p.node(dst, d.values[p.vi], false)
			if len(p.stack) == 0 {
				p.vi++
			}
			return dst, true
		}

		f := &p.stack[len(p.stack)-1]
		n := f.length()
		if f.pos < n {
			var v any
			dst = appendIndent(dst, len(p.stack))
			if f.isMap() {
				key := f.keys[f.pos]
				v = f.m[key]
				dst = appendString(dst, key)
				dst = append(dst, ':', ' ')
			} else {
				v = f.arr[f.pos]
			}
			comma := f.pos < n-1
			f.pos++
			return p.node(dst, v, comma), true
		}

		dst = appendIndent(dst, len(p.stack)-1)
		if f.isMap() {
			dst = append(dst, '}')
		} else {
			dst = append(dst, ']')
		}
		if f.comma {
			dst = append(dst, ',')
		}
		p.stack = p.stack[:len(p.stack)-1]
		if len(p.stack) == 0 {
			p.vi++
		}
		return dst, true
	}
}

// node appends v (indent and key already written). Non-empty containers
// push a frame and emit only their opening bracket.
func (p *printer) node(dst []byte, v any, comma bool) []byte {
	switch x := v.(type) {
	case []any:
		if len(x) > 0 {
			p.stack = append(p.stack, frame{arr: x, comma: comma})
			return append(dst, '[')
		}
		if x != nil {
			dst = append(dst, '[', ']')
			if comma {
				dst = append(dst, ',')
			}
			return dst
		}
	case map[string]any:
		if len(x) > 0 {
			depth := len(p.stack)
			for len(p.keyBufs) <= depth {
				p.keyBufs = append(p.keyBufs, nil)
			}
			keys, sc := p.doc.keysFor(x, &p.keyBufs[depth])
			p.stack = append(p.stack, frame{m: x, keys: keys, comma: comma, scratch: sc})
			return append(dst, '{')
		}
		if x != nil {
			dst = append(dst, '{', '}')
			if comma {
				dst = append(dst, ',')
			}
			return dst
		}
	}
	dst = appendScalar(dst, v)
	if comma {
		dst = append(dst, ',')
	}
	return dst
}
