package jsonview

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"math"
	"math/big"
	"math/rand"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
	"time"
	"unicode/utf8"

	"github.com/dayangraham/gijq/internal/jsontest"
	"github.com/itchyny/gojq"
	"github.com/mattn/go-runewidth"
)

// ref is the reference formatter applied to a tree normalised the way the
// new printer (and gojq) treat non-finite floats: NaN -> null, +-Inf ->
// +-MaxFloat64. Everything else must be byte-identical to encoding/json.
// invalidMark stands in for invalid UTF-8 bytes while the reference encodes,
// then becomes the escape jsonview emits. encoding/json's own handling of
// invalid UTF-8 changed in Go 1.27 (\ufffd escape -> literal rune), so the
// reference must not depend on it. The generators never produce this rune.
const invalidMark = "\uE000"

func ref(results []any) string {
	out := make([]any, len(results))
	for i, r := range results {
		out[i] = normalize(r)
	}
	return strings.ReplaceAll(jsontest.ReferenceFormat(out), invalidMark, `\ufffd`)
}

// markInvalid replaces each invalid UTF-8 byte with invalidMark, exactly as
// encoding/json would substitute one replacement per invalid byte.
func markInvalid(s string) string {
	if utf8.ValidString(s) {
		return s
	}
	var b strings.Builder
	for i := 0; i < len(s); {
		r, size := utf8.DecodeRuneInString(s[i:])
		if r == utf8.RuneError && size == 1 {
			b.WriteString(invalidMark)
		} else {
			b.WriteString(s[i : i+size])
		}
		i += size
	}
	return b.String()
}

func normalize(v any) any {
	switch x := v.(type) {
	case float64:
		if math.IsNaN(x) {
			return nil
		}
		return min(max(x, -math.MaxFloat64), math.MaxFloat64)
	case []any:
		if x == nil {
			return x
		}
		o := make([]any, len(x))
		for i, e := range x {
			o[i] = normalize(e)
		}
		return o
	case map[string]any:
		if x == nil {
			return x
		}
		o := make(map[string]any, len(x))
		for k, e := range x {
			o[markInvalid(k)] = normalize(e)
		}
		return o
	case string:
		return markInvalid(x)
	}
	return v
}

func allLines(d *Doc, ix *Index) []string {
	return d.Lines(ix, 0, 1<<30)
}

func checkEquivalent(t testing.TB, name string, vals []any) {
	t.Helper()
	want := ref(vals)
	d := NewDoc(vals)
	got := strings.Join(allLines(d, nil), "\n")
	if got != want {
		t.Fatalf("%s: lines mismatch\nwant: %q\n got: %q", name, trunc(want), trunc(got))
	}
	var sb strings.Builder
	n, err := d.WriteTo(&sb)
	if err != nil || sb.String() != want || n != int64(len(want)) {
		t.Fatalf("%s: WriteTo mismatch (err=%v n=%d)", name, err, n)
	}
}

func trunc(s string) string {
	if len(s) > 400 {
		return s[:400] + "..."
	}
	return s
}

func loadTestdata(t testing.TB) map[string]any {
	files, _ := filepath.Glob("../../testdata/*.json")
	out := map[string]any{}
	for _, f := range files {
		b, err := os.ReadFile(f)
		if err != nil {
			t.Fatal(err)
		}
		var v any
		if err := json.Unmarshal(b, &v); err != nil {
			t.Fatalf("%s: %v", f, err)
		}
		out[filepath.Base(f)] = v
	}
	if len(out) == 0 {
		t.Fatal("no testdata found")
	}
	return out
}

func TestEquivalenceTestdata(t *testing.T) {
	for name, v := range loadTestdata(t) {
		checkEquivalent(t, name, []any{v})
	}
}

func edgeCases() map[string][]any {
	bi, _ := new(big.Int).SetString("123456789012345678901234567890", 10)
	return map[string][]any{
		"scalars":        {nil, true, false, 0, -1, 42, "", "x"},
		"empties":        {[]any{}, map[string]any{}, []any{[]any{}, map[string]any{}, []any{[]any{}}}, map[string]any{"a": []any{}, "b": map[string]any{}}},
		"nil-containers": {[]any(nil), map[string]any(nil), []any{[]any(nil), map[string]any(nil)}},
		"unicode":        {"héllo 世界 😀", map[string]any{"ключ": "значение", "日本": []any{"語"}}},
		"control":        {"a\x00b\x01\x1f\tc\nd\re\bf\fg\x7f", map[string]any{"k\n\"\\": "v"}},
		"badutf8":        {"a\xffb\xc0\xaf", map[string]any{"\xff": "\xfe"}},
		"lineseps":       {"x y z"},
		"html":           {"<a href=\"x\">&amp;</a>", map[string]any{"<k>": "&"}},
		"bigints":        {bi, []any{bi, json.Number("1234567890123456789012345678901234567890")}, (*big.Int)(nil)},
		"jsonnumber":     {json.Number("1.50"), json.Number("-0"), json.Number("1e400"), json.Number("")},
		"floats":         {1e21, 1e20, 1e-7, 0.000001, 0.0000001234, math.Copysign(0, -1), 123456789.123, 1.5, 100.0, -2.5e-10, 1e300, math.MaxFloat64, math.SmallestNonzeroFloat64},
		"ints":           {math.MaxInt64, math.MinInt64, 0},
		"nan":            {math.NaN(), math.Inf(1), math.Inf(-1)},
		"nested-nan":     {map[string]any{"a": []any{1, math.NaN()}, "b": "x"}, []any{math.Inf(1)}, "after"},
		"multi":          {1, "two", []any{3}, map[string]any{"four": 4}, nil},
		"sorted-keys":    {map[string]any{"b": 1, "a": 2, "B": 3, "aa": 4, "": 5, "é": 6, "Z": 7}},
		"none":           {},
	}
}

func TestEquivalenceEdgeCases(t *testing.T) {
	for name, vals := range edgeCases() {
		t.Run(name, func(t *testing.T) { checkEquivalent(t, name, vals) })
	}
}

func TestNonFiniteAndUnknown(t *testing.T) {
	inf := math.Inf(1)
	vals := []any{math.NaN(), inf, -inf, []any{math.NaN(), inf, 1.5}, map[string]any{"a": -inf, "b": math.NaN()}}
	got := strings.Join(allLines(NewDoc(vals), nil), "\n")
	want := "null\n1.7976931348623157e+308\n-1.7976931348623157e+308\n" +
		"[\n  null,\n  1.7976931348623157e+308,\n  1.5\n]\n" +
		"{\n  \"a\": -1.7976931348623157e+308,\n  \"b\": null\n}"
	if got != want {
		t.Fatalf("got %q\nwant %q", got, want)
	}
	// Invalid json.Number is emitted verbatim; unknown types are rendered
	// per node, on one line, without disturbing the rest.
	vals = []any{json.Number("abc"), []any{int64(5), []int{1, 2}, struct{ A string }{"<x>"}, "ok"}}
	got = strings.Join(allLines(NewDoc(vals), nil), "\n")
	want = "abc\n[\n  5,\n  [1,2],\n  {\"A\":\"<x>\"},\n  \"ok\"\n]"
	if got != want {
		t.Fatalf("got %q\nwant %q", got, want)
	}
}

func randValue(r *rand.Rand, depth int) any {
	k := r.Intn(10)
	if depth <= 0 && k >= 7 {
		k = r.Intn(7)
	}
	switch k {
	case 0:
		return nil
	case 1:
		return r.Intn(2) == 0
	case 2:
		return r.Intn(2000000) - 1000000
	case 3:
		return []float64{r.Float64(), r.NormFloat64() * 1e22, r.NormFloat64() * 1e-9, 0.5, 1e21}[r.Intn(5)]
	case 4, 5:
		const alphabet = "abc \"\\\n\t<>&é世 \x01\xff"
		rs := []byte(alphabet)
		n := r.Intn(8)
		b := make([]byte, n)
		for i := range b {
			b[i] = rs[r.Intn(len(rs))]
		}
		return string(b)
	case 6:
		return json.Number(fmt.Sprint(r.Intn(1000)))
	case 7, 8:
		n := r.Intn(6)
		if r.Intn(4) == 0 {
			n = 0
		}
		a := make([]any, n)
		for i := range a {
			a[i] = randValue(r, depth-1)
		}
		return a
	default:
		n := r.Intn(6)
		m := make(map[string]any, n)
		for i := 0; i < n; i++ {
			m[fmt.Sprintf("k%d%s", r.Intn(20), string(rune('a'+r.Intn(5))))] = randValue(r, depth-1)
		}
		return m
	}
}

func TestEquivalenceRandom(t *testing.T) {
	r := rand.New(rand.NewSource(42))
	for i := 0; i < 300; i++ {
		n := 1 + r.Intn(3)
		vals := make([]any, n)
		for j := range vals {
			vals[j] = randValue(r, 2+r.Intn(8))
		}
		checkEquivalent(t, fmt.Sprintf("random-%d", i), vals)
	}
	// deep and wide
	var deep any = "leaf"
	for i := 0; i < 500; i++ {
		deep = map[string]any{"n": []any{deep, i}}
	}
	wide := make([]any, 20000)
	for i := range wide {
		wide[i] = map[string]any{"i": i}
	}
	big := make(map[string]any, 5000) // exercises sorted-key cache
	for i := range big {
		big[fmt.Sprint(i)] = i
	}
	checkEquivalent(t, "deep/wide", []any{deep, wide, big})
}

func FuzzEquivalence(f *testing.F) {
	f.Add([]byte(`{"a":[1,2.5,"x "],"b":{}}`))
	f.Add([]byte(`[1e21,1e-7,-0,[],{},"\u0000"]`))
	f.Fuzz(func(t *testing.T, data []byte) {
		var v any
		dec := json.NewDecoder(bytes.NewReader(data))
		var vals []any
		for dec.Decode(&v) == nil {
			vals = append(vals, v)
			v = nil
		}
		checkEquivalent(t, "fuzz", vals)
	})
}

// referenceMetrics returns line count and max display width of text.
func referenceMetrics(text string) (lines, maxW int) {
	if text == "" {
		return 0, 0
	}
	for _, l := range strings.Split(text, "\n") {
		lines++
		w := 0
		for _, r := range l {
			w += runewidth.RuneWidth(r)
		}
		if w > maxW {
			maxW = w
		}
	}
	return
}

func TestRandomAccessAndIndex(t *testing.T) {
	var sets [][]any
	for _, v := range loadTestdata(t) {
		sets = append(sets, []any{v})
	}
	for _, v := range edgeCases() {
		sets = append(sets, v)
	}
	r := rand.New(rand.NewSource(7))
	for i := 0; i < 30; i++ {
		sets = append(sets, []any{randValue(r, 9), randValue(r, 9), randValue(r, 9)})
	}
	big := make([]any, 5000)
	for i := range big {
		big[i] = map[string]any{"i": i, "s": "é世", "l": []any{1, 2}}
	}
	sets = append(sets, []any{big}, big)

	for si, vals := range sets {
		text := ref(vals)
		wantLines, wantW := referenceMetrics(text)
		var want []string
		if text != "" {
			want = strings.Split(text, "\n")
		}
		d := NewDoc(vals)
		for _, step := range []int{1, 3, 64, DefaultStep} {
			ix, err := d.BuildIndex(context.Background(), step, nil)
			if err != nil || !ix.Complete() {
				t.Fatalf("set %d: index err=%v complete=%v", si, err, ix.Complete())
			}
			if ix.Lines() != wantLines || ix.MaxWidth() != wantW {
				t.Fatalf("set %d step %d: lines/width = %d/%d, want %d/%d", si, step, ix.Lines(), ix.MaxWidth(), wantLines, wantW)
			}
			starts := []int{0, 1, 2, wantLines - 1, wantLines, wantLines + 5, wantLines / 2, step - 1, step, step + 1, 2*step + 3}
			for k := 0; k < 20; k++ {
				starts = append(starts, r.Intn(wantLines+2))
			}
			for _, s := range starts {
				for _, n := range []int{1, 7, 50, 3000} {
					exp := slice(want, s, n)
					for _, idx := range []*Index{nil, ix} {
						got := d.Lines(idx, s, n)
						if strings.Join(got, "\x00") != strings.Join(exp, "\x00") || len(got) != len(exp) {
							t.Fatalf("set %d step %d Lines(%d,%d) idx=%v mismatch: got %d lines want %d", si, step, s, n, idx != nil, len(got), len(exp))
						}
					}
				}
			}
		}
	}
}

func slice(all []string, s, n int) []string {
	if s < 0 {
		s = 0
	}
	if s >= len(all) {
		return []string{}
	}
	e := s + n
	if e > len(all) {
		e = len(all)
	}
	return all[s:e]
}

func TestPartialIndexAndCancel(t *testing.T) {
	big := make([]any, 200000)
	for i := range big {
		big[i] = map[string]any{"i": i}
	}
	d := NewDoc(big)
	var partials []*Index
	ix, err := d.BuildIndex(context.Background(), 64, func(p *Index) { partials = append(partials, p) })
	if err != nil || !ix.Complete() {
		t.Fatal(err)
	}
	want := d.Lines(nil, 500000, 20)
	// Every published partial snapshot must give correct lines (use the
	// snapshots after the builder is done: they must not have been mutated).
	for _, p := range partials {
		if got := d.Lines(p, 500000, 20); strings.Join(got, "\n") != strings.Join(want, "\n") {
			t.Fatal("partial index returned wrong lines")
		}
	}

	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	pix, err := d.BuildIndex(ctx, 64, nil)
	if err == nil || pix.Complete() {
		t.Fatalf("expected cancellation, got err=%v complete=%v", err, pix.Complete())
	}
	if got := d.Lines(pix, 1000, 5); len(got) != 5 {
		t.Fatal("partial index after cancel unusable")
	}
}

func TestIndexConcurrentRead(t *testing.T) {
	vals := make([]any, 100000)
	for i := range vals {
		vals[i] = map[string]any{"i": i, "t": []any{1, 2}}
	}
	d := NewDoc(vals)
	ch := make(chan *Index, 1000)
	go func() {
		d.BuildIndex(context.Background(), 128, func(p *Index) { ch <- p })
		close(ch)
	}()
	var last *Index
	for p := range ch {
		last = p
		d.Lines(p, p.Lines()+10, 10) // reading ahead of the covered region
		d.Lines(p, 0, 50)
	}
	if last == nil || !last.Complete() {
		t.Fatal("no complete index published")
	}
}

func TestExtendSharesStates(t *testing.T) {
	d := NewDoc([]any{1, []any{2}})
	e := d.Extend([]any{"x"})
	if got := strings.Join(allLines(e, nil), "\n"); got != ref([]any{1, []any{2}, "x"}) {
		t.Fatalf("got %q", got)
	}
	if d.NumValues() != 2 || e.NumValues() != 3 {
		t.Fatal("Extend mutated the original")
	}
}

func run(t testing.TB, query string, data any) gojq.Iter {
	q, err := gojq.Parse(query)
	if err != nil {
		t.Fatal(err)
	}
	return runQ(t, q, data, context.Background())
}

func runQ(t testing.TB, q *gojq.Query, data any, ctx context.Context) gojq.Iter {
	code, err := gojq.Compile(q)
	if err != nil {
		t.Fatal(err)
	}
	return code.RunWithContext(ctx, data)
}

func syntheticData(n int) any {
	items := make([]any, n)
	st := []string{"ok", "warn", "error"}
	for i := range items {
		items[i] = map[string]any{"id": i, "status": st[i%3], "tags": []any{"a", "b"}}
	}
	return map[string]any{"items": items}
}

func drain(it gojq.Iter) ([]any, error) {
	var out []any
	for {
		v, ok := it.Next()
		if !ok {
			return out, nil
		}
		if e, isErr := v.(error); isErr {
			return out, e
		}
		out = append(out, v)
	}
}

func waitDone(t *testing.T, c *Collector) {
	t.Helper()
	select {
	case <-c.Done():
	case <-time.After(10 * time.Second):
		t.Fatal("collector did not finish")
	}
}

func TestCollectMatchesDrain(t *testing.T) {
	data := syntheticData(20000)
	for _, q := range []string{
		`.items[] | select(.status == "ok") | .id`, `.`, `.items[0]`, `.items[] | .tags`, `empty`, `[.items[] | .id] | length`, `.items[] | {id, s: .status}`,
	} {
		want, err := drain(run(t, q, data))
		if err != nil {
			t.Fatal(err)
		}
		c := Collect(context.Background(), run(t, q, data), CollectOptions{})
		waitDone(t, c)
		s := c.Snapshot()
		if !s.Done || s.Err != nil {
			t.Fatalf("%s: done=%v err=%v", q, s.Done, s.Err)
		}
		if ref(s.Values) != ref(want) || len(s.Values) != len(want) {
			t.Fatalf("%s: values differ (%d vs %d)", q, len(s.Values), len(want))
		}
		if strings.Join(allLines(s.Doc, nil), "\n") != ref(want) {
			t.Fatalf("%s: snapshot doc differs", q)
		}
	}
}

func TestCollectFirstScreenAndProgress(t *testing.T) {
	data := syntheticData(50000)
	c := Collect(context.Background(), run(t, `.items[] | .id`, data), CollectOptions{FirstScreenLines: 50})
	s := c.Snapshot() // available synchronously
	if s == nil || len(s.Values) < 50 || len(s.Values) > 50 {
		t.Fatalf("first screen should hold exactly 50 scalar values, got %v", s)
	}
	if len(s.Doc.Lines(nil, 0, 50)) != 50 {
		t.Fatal("first screen not full")
	}
	waitDone(t, c)
	if n := len(c.Snapshot().Values); n != 50000 {
		t.Fatalf("got %d values", n)
	}
	// An old snapshot is unchanged.
	if len(s.Values) != 50 {
		t.Fatal("snapshot mutated")
	}
}

func TestCollectError(t *testing.T) {
	data := syntheticData(10)
	c := Collect(context.Background(), run(t, `.items[] | if .id == 7 then error("boom") else .id end`, data), CollectOptions{FirstScreenLines: 3})
	waitDone(t, c)
	s := c.Snapshot()
	if !s.Done || s.Err == nil || !strings.Contains(s.Err.Error(), "boom") {
		t.Fatalf("expected boom, got %v", s.Err)
	}
	// Error during the first screen.
	c = Collect(context.Background(), run(t, `.items[] | if .id == 1 then error("early") else .id end`, data), CollectOptions{})
	waitDone(t, c)
	if s := c.Snapshot(); s.Err == nil || !s.Done {
		t.Fatal("expected early error")
	}
}

func TestCollectCancel(t *testing.T) {
	before := runtime.NumGoroutine()
	data := syntheticData(10)
	// An endless, cheap generator.
	ctx, cancel := context.WithCancel(context.Background())
	c := Collect(ctx, run(t, `repeat(1)`, data), CollectOptions{})
	_ = data
	time.Sleep(20 * time.Millisecond)
	if c.Snapshot().Done {
		t.Fatal("should still be running")
	}
	cancel()
	waitDone(t, c)
	s := c.Snapshot()
	if s.Err != context.Canceled || !s.Done {
		t.Fatalf("err=%v done=%v", s.Err, s.Done)
	}
	for i := 0; i < 50 && runtime.NumGoroutine() > before; i++ {
		time.Sleep(10 * time.Millisecond)
	}
	if after := runtime.NumGoroutine(); after > before {
		t.Fatalf("goroutine leak: %d -> %d", before, after)
	}

	// Already-cancelled context.
	ctx2, cancel2 := context.WithCancel(context.Background())
	cancel2()
	c = Collect(ctx2, run(t, `repeat(1)`, data), CollectOptions{})
	waitDone(t, c)
	if c.Snapshot().Err == nil {
		t.Fatal("expected error for cancelled ctx")
	}
}

type gojqCode struct{ c *gojq.Code }

func (g *gojqCode) run(ctx context.Context, v any) gojq.Iter { return g.c.RunWithContext(ctx, v) }

func compileBench(t testing.TB, query string) *gojqCode {
	q, err := gojq.Parse(query)
	if err != nil {
		t.Fatal(err)
	}
	c, err := gojq.Compile(q)
	if err != nil {
		t.Fatal(err)
	}
	return &gojqCode{c}
}
