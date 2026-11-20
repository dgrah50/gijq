package ui

import (
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"

	tea "charm.land/bubbletea/v2"
	"github.com/charmbracelet/x/ansi"

	"github.com/dayangraham/gijq/internal/autocomplete"
	"github.com/dayangraham/gijq/internal/clipboard"
	"github.com/dayangraham/gijq/internal/history"
	"github.com/dayangraham/gijq/internal/jq"
	"github.com/dayangraham/gijq/internal/jsonview"
)

const testJSON = `{"items":[{"name":"x","id":1},{"name":"y","id":2}],"meta":{"count":2}}`

func newTestModel(t testing.TB, width, height int) Model {
	t.Helper()
	jqSvc, err := jq.NewService([]byte(testJSON))
	if err != nil {
		t.Fatal(err)
	}
	hist, err := history.NewStore(filepath.Join(t.TempDir(), "hist.json"))
	if err != nil {
		t.Fatal(err)
	}
	m := NewModel(jqSvc, autocomplete.NewService(jqSvc), hist, clipboard.NewService(), Config{Filename: "t.json", Filepath: "/t.json"})
	next, _ := m.Update(tea.WindowSizeMsg{Width: width, Height: height})
	return next.(Model)
}

var namedKeys = map[string]rune{
	"tab": tea.KeyTab, "esc": tea.KeyEscape, "enter": tea.KeyEnter,
	"space": tea.KeySpace, "backspace": tea.KeyBackspace, "delete": tea.KeyDelete,
	"up": tea.KeyUp, "down": tea.KeyDown, "left": tea.KeyLeft, "right": tea.KeyRight,
	"home": tea.KeyHome, "end": tea.KeyEnd, "pgup": tea.KeyPgUp, "pgdown": tea.KeyPgDown,
	"f1": tea.KeyF1,
}

// key builds a v2 key press message from its String() form
// (e.g. "ctrl+w", "shift+tab", "alt+left", "space") or literal text.
func key(s string) tea.KeyPressMsg {
	if s == " " {
		s = "space"
	}
	var mod tea.KeyMod
	rest := s
	for len(rest) > 1 {
		i := strings.IndexByte(rest, '+')
		if i <= 0 || i == len(rest)-1 {
			break
		}
		switch rest[:i] {
		case "ctrl":
			mod |= tea.ModCtrl
		case "alt":
			mod |= tea.ModAlt
		case "shift":
			mod |= tea.ModShift
		default:
			i = -1
		}
		if i < 0 {
			break
		}
		rest = rest[i+1:]
	}
	if code, ok := namedKeys[rest]; ok {
		k := tea.KeyPressMsg{Code: code, Mod: mod}
		if code == tea.KeySpace && mod == 0 {
			k.Text = " "
		}
		return k
	}
	r := []rune(rest)
	if len(r) == 1 {
		k := tea.KeyPressMsg{Code: r[0], Mod: mod}
		if mod == 0 {
			k.Text = rest
		}
		return k
	}
	return tea.KeyPressMsg{Code: r[0], Text: rest}
}

func send(m Model, msg tea.Msg) (Model, tea.Cmd) {
	next, cmd := m.Update(msg)
	return next.(Model), cmd
}

// settle runs cmd and feeds every message it produces back into the model
// until no command is left (collection and indexing are finite in tests).
func settle(t testing.TB, m Model, cmd tea.Cmd) Model {
	t.Helper()
	for cmd != nil {
		m, cmd = send(m, cmd())
	}
	return m
}

// runQuery dispatches the latest requested query and settles it.
func runQuery(t testing.TB, m Model) Model {
	t.Helper()
	m, cmd := send(m, executeQueryMsg{seq: m.query.latest})
	return settle(t, m, cmd)
}

// runFilter sets the filter and evaluates it to completion (result fully
// collected and indexed).
func runFilter(t testing.TB, m Model, filter string) Model {
	t.Helper()
	m.setFilter(filter)
	m.query.request()
	return runQuery(t, m)
}

// deliverValues shows values as the (complete, indexed) result of the
// current filter.
func deliverValues(t testing.TB, m Model, values ...any) Model {
	t.Helper()
	seq := m.query.latest
	m.query.dispatch(seq)
	snap := &jsonview.Snapshot{Values: values, Doc: jsonview.NewDoc(values), Done: true}
	m, cmd := send(m, resultMsg{seq: seq, filter: m.filter.Value(), snap: snap})
	return settle(t, m, cmd)
}

// deliverErr shows err as the result of the current filter.
func deliverErr(t testing.TB, m Model, err error) Model {
	t.Helper()
	seq := m.query.latest
	m.query.dispatch(seq)
	m, _ = send(m, resultMsg{seq: seq, filter: m.filter.Value(), err: err})
	return m
}

// docText is the full text of a document.
func docText(t testing.TB, d *jsonview.Doc) string {
	t.Helper()
	var sb strings.Builder
	if _, err := d.WriteTo(&sb); err != nil {
		t.Fatal(err)
	}
	return sb.String()
}

// outputText is what the program would print after exit.
func outputText(t testing.TB, m Model) (string, bool) {
	t.Helper()
	d, ok := m.Output()
	if !ok {
		return "", false
	}
	return docText(t, d), true
}

func TestRenderViewFitsWidthAndHeight(t *testing.T) {
	long := strings.Repeat("世界abc", 200)
	val := map[string]any{"a": long, "b": float64(1)}
	for _, width := range []int{50, 80, 120, 200} {
		for _, errResult := range []bool{false, true} {
			m := newTestModel(t, width, 30)
			if errResult {
				m = deliverErr(t, m, errTest("boom "+long))
			} else {
				m = deliverValues(t, m, val)
			}
			for _, xo := range []int{0, 7, 100} {
				m.outputXOffset = xo
				m.clampOutputXOffset()
				content := m.renderContent()
				rows := strings.Split(content, "\n")
				if len(rows) != m.contentHeight()+2 {
					t.Fatalf("w=%d: content rows = %d, want %d", width, len(rows), m.contentHeight()+2)
				}
				for i, r := range rows {
					if w := ansi.StringWidth(r); w > m.width {
						t.Fatalf("w=%d row %d width %d > %d", width, i, w, m.width)
					}
				}
				view := m.renderView()
				lines := strings.Split(view, "\n")
				if len(lines) != m.height {
					t.Fatalf("w=%d: view lines = %d, want %d", width, len(lines), m.height)
				}
				for i, l := range lines {
					if w := ansi.StringWidth(l); w > m.width {
						t.Fatalf("w=%d view line %d width %d > %d", width, i, w, m.width)
					}
				}
			}
		}
	}
}

type errTest string

func (e errTest) Error() string { return string(e) }

func TestResultKeepsScrollClamped(t *testing.T) {
	m := newTestModel(t, 100, 20)
	many := make([]any, 101)
	for i := range many {
		many[i] = "l"
	}
	m = deliverValues(t, m, many...)
	m.scrollY(1000)
	if want := 101 - m.contentHeight(); m.yOffset != want {
		t.Fatalf("yOffset = %d, want %d", m.yOffset, want)
	}
	m = deliverValues(t, m, "a", "b")
	if m.yOffset != 0 {
		t.Fatalf("yOffset after short result = %d, want 0", m.yOffset)
	}
}

func TestTabChangingFilterReruns(t *testing.T) {
	m := newTestModel(t, 100, 30)
	m.filter.SetValue(".items[0]")
	m.filter.SetCursor(len(".items[0]"))
	seq := m.query.latest
	m, cmd := send(m, key("tab"))
	if m.filter.Value() != ".items[0]." {
		t.Fatalf("filter = %q", m.filter.Value())
	}
	if m.query.latest == seq || cmd == nil {
		t.Fatalf("tab changed filter without re-running query (seq %d -> %d)", seq, m.query.latest)
	}
}

func TestTabUnchangedFilterDoesNotRerun(t *testing.T) {
	m := newTestModel(t, 100, 30)
	seq := m.query.latest
	m, _ = send(m, key("tab"))
	if m.query.latest != seq {
		t.Fatalf("query re-queued although filter unchanged")
	}
}

func TestEscFromAutocompleteResyncsKeys(t *testing.T) {
	m := newTestModel(t, 100, 30)
	m.mode = ModeAutocomplete
	m.suggestions = []string{"a"}
	m.filter.SetValue(".items[0].")
	m, cmd := send(m, key("esc"))
	if m.mode != ModeNormal || m.suggestions != nil {
		t.Fatalf("mode=%v suggestions=%v", m.mode, m.suggestions)
	}
	if want := m.autocomplete.ParseContext(".items[0].").Path; m.acContext().Path != want {
		t.Fatalf("acContext.Path = %q, want %q", m.acContext().Path, want)
	}
	if !m.keys.loadingFor(m.currentPath()) || cmd == nil {
		t.Fatalf("keys not re-fetched: %+v cmd=%v", m.keys, cmd)
	}
}

func TestQuestionMarkTypedAndHelpKeys(t *testing.T) {
	m := newTestModel(t, 100, 30)
	m, _ = send(m, key("?"))
	if m.mode != ModeNormal || !strings.HasSuffix(m.filter.Value(), "?") {
		t.Fatalf("'?' not inserted: mode=%v filter=%q", m.mode, m.filter.Value())
	}
	for _, k := range []string{"f1", "ctrl+_"} {
		m2, _ := send(m, key(k))
		if m2.mode != ModeHelp {
			t.Fatalf("%s did not open help", k)
		}
		m2, _ = send(m2, key(k))
		if m2.mode != ModeNormal {
			t.Fatalf("%s did not close help", k)
		}
	}
	for _, k := range []string{"?", "esc", "enter", " "} {
		m2, _ := send(m, key("f1"))
		m2, _ = send(m2, key(k))
		if m2.mode != ModeNormal {
			t.Fatalf("%q did not close help", k)
		}
	}
}

func TestStatusClearOnlyMatchingSeq(t *testing.T) {
	m := newTestModel(t, 100, 30)
	m.setStatus("first")
	first := m.statusSeq
	m.setStatus("second")
	m, _ = send(m, statusClearMsg{seq: first})
	if m.status != "second" {
		t.Fatalf("stale timer cleared status: %q", m.status)
	}
	m, _ = send(m, statusClearMsg{seq: m.statusSeq})
	if m.status != "" {
		t.Fatalf("status not cleared: %q", m.status)
	}
}

func TestEnterHelpOrHistoryClearsSuggestions(t *testing.T) {
	for _, k := range []string{"f1", "ctrl+h"} {
		m := newTestModel(t, 100, 30)
		m.mode = ModeAutocomplete
		m.suggestions = []string{"a", "b"}
		m.selectedIdx = 1
		m, _ = send(m, key(k))
		if m.suggestions != nil || m.selectedIdx != 0 {
			t.Fatalf("%s left suggestions=%v idx=%d", k, m.suggestions, m.selectedIdx)
		}
	}
}

func TestHistoryUnhandledKeyIsProcessed(t *testing.T) {
	m := newTestModel(t, 100, 30)
	m, _ = send(m, key("ctrl+h"))
	if m.mode != ModeHistory {
		t.Fatal("not in history")
	}
	before := m.filter.Value()
	m, _ = send(m, key("x"))
	if m.mode != ModeNormal || m.filter.Value() != before+"x" {
		t.Fatalf("mode=%v filter=%q", m.mode, m.filter.Value())
	}
}

func TestPlaceOverlayANSIAndMultibyte(t *testing.T) {
	base := "\x1b[31mhello world\x1b[0m 世界世界 tail"
	baseW := ansi.StringWidth(base)
	got := placeOverlay(base+"\nxx", "[AB]", baseW, 2)
	line := strings.Split(got, "\n")[0]
	if w := ansi.StringWidth(line); w != baseW {
		t.Fatalf("width = %d, want %d: %q", w, baseW, line)
	}
	if !strings.Contains(ansi.Strip(line), "[AB]") {
		t.Fatalf("overlay missing: %q", ansi.Strip(line))
	}
	// Overlay landing in the middle of a wide rune must not corrupt output.
	for x := 0; x < baseW; x++ {
		out := placeOverlay(base, "Z", x*2+1, 1)
		if !strings.Contains(out, "Z") {
			t.Fatalf("overlay lost at %d", x)
		}
	}
}

func TestKeysStateStaleIgnored(t *testing.T) {
	var k keysState
	k.request(".a")
	k.request(".b")
	k.receive(".a", []string{"x"}, nil)
	if k.status != keysLoading {
		t.Fatalf("stale result applied: %+v", k)
	}
	k.receive(".b", []string{"y"}, nil)
	if k.status != keysLoaded || len(k.keys) != 1 {
		t.Fatalf("state = %+v", k)
	}
	if k.request(".b") {
		t.Fatal("re-request of loaded path should be a no-op")
	}
}

func TestKeyHelperStringsMatchBindings(t *testing.T) {
	for _, s := range []string{
		"ctrl+c", "f1", "ctrl+/", "ctrl+_", "esc", "ctrl+y", "ctrl+f", "ctrl+h",
		"enter", "tab", "shift+tab", "up", "down", "shift+up", "shift+down",
		"shift+left", "shift+right", "home", "end", "pgup", "pgdown",
		"alt+left", "alt+b", "ctrl+left", "alt+right", "alt+f", "ctrl+right",
		"alt+backspace", "ctrl+backspace", "ctrl+w", "alt+delete", "alt+d",
		"ctrl+delete", "space", "?", "a",
	} {
		if got := key(s).String(); got != s {
			t.Errorf("key(%q).String() = %q", s, got)
		}
	}
}

func TestEnterRecordsOutputAndQuitsWithoutWriting(t *testing.T) {
	m := newTestModel(t, 100, 30)
	m = deliverValues(t, m, map[string]any{"a": float64(1)})

	// Capture anything written to stdout during Update.
	old := os.Stdout
	r, w, err := os.Pipe()
	if err != nil {
		t.Fatal(err)
	}
	os.Stdout = w
	next, cmd := m.Update(key("enter"))
	os.Stdout = old
	_ = w.Close()
	written, _ := io.ReadAll(r)
	if len(written) != 0 {
		t.Fatalf("Update wrote to stdout: %q", written)
	}

	got := next.(Model)
	out, ok := outputText(t, got)
	if !ok || out != "{\n  \"a\": 1\n}" {
		t.Fatalf("Output() = %q, %v", out, ok)
	}
	if cmd == nil {
		t.Fatal("expected quit command")
	}
	if _, isQuit := cmd().(tea.QuitMsg); !isQuit {
		t.Fatalf("enter did not return tea.Quit")
	}

	// Errors produce no output but still quit.
	m = deliverErr(t, newTestModel(t, 100, 30), errTest("boom"))
	next, cmd = m.Update(key("enter"))
	if _, ok := next.(Model).Output(); ok {
		t.Fatal("error result must not be output")
	}
	if _, isQuit := cmd().(tea.QuitMsg); !isQuit {
		t.Fatal("expected quit")
	}
}

func TestEnterWithStaleResultEvaluatesCurrentFilter(t *testing.T) {
	m := newTestModel(t, 100, 30)
	// Result shown is for "." but the user has since typed a new filter
	// whose query hasn't come back yet (debounce pending / still running).
	m = deliverValues(t, m, "STALE")
	m.filter.SetValue(".items[0].name")

	next, cmd := m.Update(key("enter"))
	got := next.(Model)
	out, ok := outputText(t, got)
	if !ok || out != `"x"` {
		t.Fatalf("Output() = %q, %v; want current filter's result", out, ok)
	}
	if h := got.history.Get("/t.json"); len(h) == 0 || h[0] != ".items[0].name" {
		t.Fatalf("history = %v", h)
	}
	if _, isQuit := cmd().(tea.QuitMsg); !isQuit {
		t.Fatal("expected quit")
	}

	// Same filter, fully collected: the shown result is used as-is (no re-run).
	m = deliverValues(t, newTestModel(t, 100, 30), "SHOWN")
	next, _ = m.Update(key("enter"))
	if out, _ := outputText(t, next.(Model)); out != `"SHOWN"` {
		t.Fatalf("fresh result should be used, got %q", out)
	}
}

// Every way of editing the filter must queue a query and keep the keys pane
// on the filter's path; handlers never do this themselves (see handleKey).
func TestEveryFilterEditQueuesQueryAndSyncsKeys(t *testing.T) {
	type step func(m Model) Model
	typeText := func(s string) step {
		return func(m Model) Model {
			for _, r := range s {
				m, _ = send(m, key(string(r)))
			}
			return m
		}
	}
	press := func(k string) step { return func(m Model) Model { m, _ = send(m, key(k)); return m } }
	cases := []struct {
		name  string
		setup []step
		edit  step
	}{
		{"typing", nil, typeText("i")},
		{"delete prev word", []step{typeText("items")}, press("ctrl+w")},
		{"delete next word", []step{typeText("items"), press("home"), press("alt+left")}, press("alt+d")},
		{"tab drills into array element", []step{typeText("items[0]")}, press("tab")},
		{"autocomplete enter", []step{press("tab")}, press("enter")},
		{"history enter", []step{func(m Model) Model {
			m.history.Add(m.filepath, ".meta")
			return m
		}, press("ctrl+h")}, press("enter")},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			m := newTestModel(t, 100, 30)
			for _, s := range tc.setup {
				m = s(m)
			}
			before, seq := m.filter.Value(), m.query.latest
			m = tc.edit(m)
			if m.filter.Value() == before {
				t.Fatalf("edit did not change filter %q", before)
			}
			if m.query.latest == seq {
				t.Fatalf("filter changed %q -> %q without queuing a query", before, m.filter.Value())
			}
			if m.keys.path != m.currentPath() {
				t.Fatalf("keys pane path %q != current path %q", m.keys.path, m.currentPath())
			}
		})
	}
}

func TestModeChangesDropOtherModeData(t *testing.T) {
	m := newTestModel(t, 100, 30)
	m.history.Add(m.filepath, ".meta")
	m, _ = send(m, key("ctrl+h"))
	if m.mode != ModeHistory || len(m.historyItems) != 1 {
		t.Fatalf("history not opened: mode=%v items=%v", m.mode, m.historyItems)
	}
	m, _ = send(m, key("esc"))
	if m.mode != ModeNormal || m.historyItems != nil || m.historyIdx != 0 {
		t.Fatalf("history data survived leaving history: %v %d", m.historyItems, m.historyIdx)
	}
	m, _ = send(m, key("tab"))
	if m.mode != ModeAutocomplete || len(m.suggestions) == 0 {
		t.Fatalf("autocomplete not opened: mode=%v", m.mode)
	}
	m, _ = send(m, key("f1"))
	if m.mode != ModeHelp || m.suggestions != nil || m.selectedIdx != 0 {
		t.Fatalf("suggestions survived entering help")
	}
}

func TestTabWithoutSuggestionsStaysNormal(t *testing.T) {
	m := newTestModel(t, 100, 30)
	m.setFilter(".nope")
	m, _ = send(m, key("tab"))
	if m.mode != ModeNormal {
		t.Fatalf("mode = %v, want normal when nothing to suggest", m.mode)
	}
}

func TestNothingStartsAfterQuit(t *testing.T) {
	m := newTestModel(t, 100, 30)
	m.setFilter(".items") // path differs from the keys pane's
	m, cmd := send(m, key("ctrl+c"))
	if !m.quitting {
		t.Fatal("not quitting")
	}
	if _, ok := cmd().(tea.QuitMsg); !ok {
		t.Fatal("expected only the quit command")
	}
}

func TestWrapIndex(t *testing.T) {
	for _, tc := range []struct{ i, n, want int }{{0, 0, 0}, {-1, 3, 2}, {3, 3, 0}, {4, 3, 1}, {-4, 3, 2}} {
		if got := wrapIndex(tc.i, tc.n); got != tc.want {
			t.Errorf("wrapIndex(%d,%d) = %d, want %d", tc.i, tc.n, got, tc.want)
		}
	}
}
