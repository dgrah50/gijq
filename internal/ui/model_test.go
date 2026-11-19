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

func deliver(t testing.TB, m Model, result jq.Result) Model {
	t.Helper()
	m.activeQuerySeq = m.querySeq
	lines, w := processResult(result)
	m, _ = send(m, resultMsg{seq: m.querySeq, result: result, lines: lines, maxLineWidth: w})
	return m
}

func TestRenderViewFitsWidthAndHeight(t *testing.T) {
	long := strings.Repeat("世界abc", 200)
	raw := "{\n  \"a\": \"" + long + "\",\n  \"b\": 1\n}"
	for _, width := range []int{50, 80, 120, 200} {
		for _, errResult := range []bool{false, true} {
			m := newTestModel(t, width, 30)
			if errResult {
				m = deliver(t, m, jq.Result{Error: errTest("boom " + long)})
			} else {
				m = deliver(t, m, jq.Result{Raw: raw})
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
	m = deliver(t, m, jq.Result{Raw: strings.Repeat("l\n", 100) + "l"})
	m.scrollY(1000)
	if want := 101 - m.contentHeight(); m.yOffset != want {
		t.Fatalf("yOffset = %d, want %d", m.yOffset, want)
	}
	m = deliver(t, m, jq.Result{Raw: "a\nb"})
	if m.yOffset != 0 {
		t.Fatalf("yOffset after short result = %d, want 0", m.yOffset)
	}
}

func TestTabChangingFilterReruns(t *testing.T) {
	m := newTestModel(t, 100, 30)
	m.filter.SetValue(".items[0]")
	m.filter.SetCursor(len(".items[0]"))
	m.acContext = m.autocomplete.ParseContext(m.filter.Value())
	seq := m.querySeq
	m, cmd := send(m, key("tab"))
	if m.filter.Value() != ".items[0]." {
		t.Fatalf("filter = %q", m.filter.Value())
	}
	if m.querySeq == seq || cmd == nil {
		t.Fatalf("tab changed filter without re-running query (seq %d -> %d)", seq, m.querySeq)
	}
}

func TestTabUnchangedFilterDoesNotRerun(t *testing.T) {
	m := newTestModel(t, 100, 30)
	seq := m.querySeq
	m, _ = send(m, key("tab"))
	if m.querySeq != seq {
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
	if want := m.autocomplete.ParseContext(".items[0].").Path; m.acContext.Path != want {
		t.Fatalf("acContext.Path = %q, want %q", m.acContext.Path, want)
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
	m = deliver(t, m, jq.Result{Raw: "{\n  \"a\": 1\n}"})

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
	out, ok := got.Output()
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
	m = deliver(t, newTestModel(t, 100, 30), jq.Result{Error: errTest("boom")})
	next, cmd = m.Update(key("enter"))
	if _, ok := next.(Model).Output(); ok {
		t.Fatal("error result must not be output")
	}
	if _, isQuit := cmd().(tea.QuitMsg); !isQuit {
		t.Fatal("expected quit")
	}
}
