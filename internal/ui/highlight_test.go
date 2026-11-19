package ui

import (
	"math/rand"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/charmbracelet/x/ansi"
	"github.com/mattn/go-runewidth"
)

// refWindow is an independent reference for the clip + ellipsis layout.
func refWindow(line string, xOffset, width int) string {
	if xOffset < 0 {
		xOffset = 0
	}
	if width <= 0 {
		return ""
	}
	rs := []rune(line)
	w := func(r rune) int { return runeWidth(r) }
	i, col := 0, 0
	if xOffset > 0 {
		for i < len(rs) && col+w(rs[i]) <= xOffset {
			col += w(rs[i])
			i++
		}
	}
	left := xOffset > 0
	j, vis := i, 0
	right := false
	for j < len(rs) {
		if vis+w(rs[j]) > width {
			right = true
			break
		}
		vis += w(rs[j])
		j++
	}
	if len(rs) == 0 {
		if left {
			return "…"
		}
		return ""
	}
	if !left && !right {
		return string(rs[i:j])
	}
	if width == 1 {
		return "…"
	}
	budget := width - 1
	if left && right {
		budget = width - 2
	}
	var sb strings.Builder
	if left {
		sb.WriteString("…")
	}
	c := 0
	for k := i; k < j; k++ {
		if c+w(rs[k]) > budget {
			break
		}
		c += w(rs[k])
		sb.WriteRune(rs[k])
	}
	if right {
		sb.WriteString("…")
	}
	return sb.String()
}

func testLines(t *testing.T) []string {
	t.Helper()
	lines := []string{
		"",
		`{"a": "b:c", "x\"y": 1, "n": null, "t": true}`,
		`[1, -2.5e3, true, null]`,
		`  "名前": "日本語のテキスト、とても長い文字列です",`,
		"  \"café\": \"ééé abc̀\",",
		`  "esc": "quote \" and \\ backslash \\\" mix {[:]}",`,
		strings.Repeat(`{"k": "v", "n": 12.5}, `, 400),
	}
	files, _ := filepath.Glob("../../testdata/*.json")
	for _, f := range files {
		b, err := os.ReadFile(f)
		if err != nil {
			t.Fatal(err)
		}
		lines = append(lines, strings.Split(string(b), "\n")...)
	}
	return lines
}

func TestHighlightWindowMatchesPlainClip(t *testing.T) {
	long := strings.Repeat(`{"k": "日本v", "n": 12.5, "é": [true, null]}, `, 300)
	lines := append(testLines(t), long)
	rng := rand.New(rand.NewSource(1))
	for _, line := range lines {
		cps := buildCheckpoints(line)
		offs := []int{0, 1, 2, 3, 7, 40, 100, 5000, 20000}
		for i := 0; i < 8; i++ {
			offs = append(offs, rng.Intn(len(line)+5))
		}
		for _, x := range offs {
			for _, width := range []int{-1, 0, 1, 2, 3, 10, 37, 80, 400} {
				want := refWindow(line, x, width)
				for _, color := range []bool{false, true} {
					got, w, _, _ := highlightWindow(line, x, width, color, cps)
					plain := ansi.Strip(got)
					if plain != want {
						t.Fatalf("color=%v x=%d width=%d line=%.60q\n got %q\nwant %q", color, x, width, line, plain, want)
					}
					if sw := ansi.StringWidth(got); sw != w || (width > 0 && sw > width) {
						t.Fatalf("width mismatch: ansi=%d returned=%d max=%d (%q)", sw, w, width, got)
					}
					if rw := runewidth.StringWidth(plain); rw != w {
						t.Fatalf("runewidth=%d returned=%d (%q)", rw, w, plain)
					}
					if strings.Contains(got, "\x1b") && !strings.HasSuffix(got, ansiReset) {
						t.Fatalf("ANSI leak: %q", got)
					}
				}
			}
		}
	}
}

// colourSeq returns the plain text and the colour of each byte.
func colourSeq(rendered string) (string, []string) {
	var text strings.Builder
	var cols []string
	cur := ""
	for i := 0; i < len(rendered); {
		if rendered[i] == 0x1b {
			j := strings.IndexByte(rendered[i:], 'm')
			code := rendered[i : i+j+1]
			if code == ansiReset {
				cur = ""
			} else {
				cur = code
			}
			i += j + 1
			continue
		}
		text.WriteByte(rendered[i])
		cols = append(cols, cur)
		i++
	}
	return text.String(), cols
}

func expectColors(t *testing.T, rendered, sub, color string) {
	t.Helper()
	text, cols := colourSeq(rendered)
	idx := strings.Index(text, sub)
	if idx < 0 {
		t.Fatalf("%q not in %q", sub, text)
	}
	for k := idx; k < idx+len(sub); k++ {
		if cols[k] != color {
			t.Fatalf("%q: byte %d (%q) colour %q, want %q", sub, k-idx, text[k], cols[k], color)
		}
	}
}

func TestTokenClassification(t *testing.T) {
	r, _, _, _ := highlightWindow(`"a": "b:c"`, 0, 80, true, nil)
	expectColors(t, r, `"a"`, ansiCyan)
	expectColors(t, r, `"b:c"`, ansiGreen)
	expectColors(t, r, `:`, "")

	r, _, _, _ = highlightWindow(`"x\"y": 1`, 0, 80, true, nil)
	expectColors(t, r, `"x\"y"`, ansiCyan)
	expectColors(t, r, `1`, ansiYellow)

	r, _, _, _ = highlightWindow(`[1, -2.5e3, true, null]`, 0, 80, true, nil)
	expectColors(t, r, `1`, ansiYellow)
	expectColors(t, r, `-2.5e3`, ansiYellow)
	expectColors(t, r, `true`, ansiMagenta)
	expectColors(t, r, `null`, ansiGray)
	expectColors(t, r, `[`, ansiWhite)
	expectColors(t, r, `, `, "")

	r, _, _, _ = highlightWindow(`"s": "has { [ : \" \\ ] }" , "k" :  false`, 0, 80, true, nil)
	expectColors(t, r, `"has { [ : \" \\ ] }"`, ansiGreen)
	expectColors(t, r, `"k"`, ansiCyan)
	expectColors(t, r, `false`, ansiMagenta)

	// Key whose closing `":` is scrolled off to the right is still a key.
	line := `  "some_long_key_name": 1`
	r, _, _, _ = highlightWindow(line, 0, 10, true, nil)
	expectColors(t, r, `"some_`, ansiCyan)

	// String whose opening quote is scrolled off to the left is still green.
	line = `  "a": "hello world value"`
	r, _, _, _ = highlightWindow(line, 10, 10, true, nil)
	expectColors(t, r, `lo wor`, ansiGreen)
	line = `  "a_long_key_name": 1`
	r, _, _, _ = highlightWindow(line, 6, 20, true, nil)
	expectColors(t, r, `ong_key`, ansiCyan)
}

func TestCheckpointsMatchFullLex(t *testing.T) {
	var sb strings.Builder
	sb.WriteString("[")
	for sb.Len() < 1<<20 {
		sb.WriteString(`{"key\"x": "val:ue \\", "n": -1.5e2, "ok": true, "z": null, "名": "日本"}, `)
	}
	line := sb.String()
	cps := buildCheckpoints(line)
	if len(cps) < 200 {
		t.Fatalf("expected many checkpoints, got %d", len(cps))
	}
	rng := rand.New(rand.NewSource(7))
	for i := 0; i < 300; i++ {
		x := rng.Intn(len(line) / 2)
		if i < 5 {
			x = cps[i+3].col // exactly on a checkpoint column
		}
		a, aw, al, ar := highlightWindow(line, x, 120, true, cps)
		b, bw, bl, br := highlightWindow(line, x, 120, true, nil)
		if a != b || aw != bw || al != bl || ar != br {
			t.Fatalf("x=%d checkpoint render differs:\n%q\n%q", x, a, b)
		}
	}
}

func TestHighlightCacheLongLinesOnly(t *testing.T) {
	c := newHighlightCache()
	if c.forLine(0, "short") != nil || len(c.cps) != 0 {
		t.Fatal("short lines must not be cached")
	}
	long := strings.Repeat("a", 3*hlStride)
	if len(c.forLine(3, long)) == 0 || len(c.cps) != 1 {
		t.Fatal("long line not cached")
	}
	c.reset()
	if len(c.cps) != 0 {
		t.Fatal("reset did not clear")
	}
	var nilCache *highlightCache
	nilCache.reset()
	if nilCache.forLine(0, long) != nil {
		t.Fatal("nil cache must be inert")
	}
}

func TestMalformedInputDoesNotPanic(t *testing.T) {
	inputs := []string{
		`"unterminated`, `"abc\`, `\`, `\\\`, `"`, `""`, `:`, `{[}]`, "\x00\xff\xfe", `-`, `tru`, `truee`, `nullx`,
		"\"\xc3", strings.Repeat(`"`, 100), strings.Repeat(`\`, 9000) + `"`,
	}
	rng := rand.New(rand.NewSource(3))
	for i := 0; i < 200; i++ {
		b := make([]byte, rng.Intn(6000))
		for j := range b {
			b[j] = byte(rng.Intn(256))
			if b[j] == 0x1b {
				b[j] = 'x'
			}
		}
		inputs = append(inputs, string(b))
	}
	for _, in := range inputs {
		cps := buildCheckpoints(in)
		for _, x := range []int{0, 1, 5, 4000} {
			for _, w := range []int{1, 2, 20} {
				got, gw, _, _ := highlightWindow(in, x, w, true, cps)
				if gw > w {
					t.Fatalf("width %d > %d for %q", gw, w, in)
				}
				if strings.Contains(got, "\x1b") && !strings.HasSuffix(got, ansiReset) {
					t.Fatalf("leak %q", got)
				}
			}
		}
	}
}

func TestLineDisplayWidthConsistent(t *testing.T) {
	for _, l := range testLines(t) {
		w := 0
		for _, r := range l {
			w += runeWidth(r)
		}
		if got := lineDisplayWidth(l); got != w {
			t.Fatalf("lineDisplayWidth=%d want %d for %q", got, w, l)
		}
		if got, _, _, _ := highlightWindow(l, 0, w+5, false, nil); got != l {
			t.Fatalf("full window %q != line %q", got, l)
		}
	}
	if lineDisplayWidth("é") != 1 {
		t.Fatal("combining mark should take zero columns")
	}
}

func TestPrebuiltCheckpointsMatchLazy(t *testing.T) {
	long := `{"payload":"` + strings.Repeat("ab\\\"c", 5000) + `","n":1}`
	lines := []string{"short", long}
	pre := prebuildCheckpoints(lines)
	if _, ok := pre[0]; ok {
		t.Fatalf("short line should have no checkpoints")
	}
	c := newHighlightCache()
	c.resetWith(pre)
	got := c.forLine(1, long)
	want := buildCheckpoints(long)
	if len(got) != len(want) || len(got) == 0 {
		t.Fatalf("checkpoints len = %d, want %d (non-zero)", len(got), len(want))
	}
	for _, x := range []int{0, 5000, 20000} {
		a, _, _, _ := highlightWindow(long, x, 80, true, got)
		b, _, _, _ := highlightWindow(long, x, 80, true, nil)
		if a != b {
			t.Fatalf("offset %d: prebuilt render differs from lex-from-0", x)
		}
	}
	c.reset()
	if c.pre != nil {
		t.Fatalf("reset should drop prebuilt checkpoints")
	}
}
