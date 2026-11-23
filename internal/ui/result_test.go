package ui

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
	"time"

	tea "charm.land/bubbletea/v2"
	"github.com/charmbracelet/x/ansi"

	"github.com/dayangraham/gijq/internal/autocomplete"
	"github.com/dayangraham/gijq/internal/clipboard"
	"github.com/dayangraham/gijq/internal/history"
	"github.com/dayangraham/gijq/internal/jq"
	"github.com/dayangraham/gijq/internal/jsontest"
	"github.com/dayangraham/gijq/internal/jsonview"
)

func bigService(t testing.TB, n int) *jq.Service {
	t.Helper()
	var sb strings.Builder
	sb.WriteString(`{"items":[`)
	statuses := []string{"ok", "warn", "error"}
	for i := 0; i < n; i++ {
		if i > 0 {
			sb.WriteByte(',')
		}
		fmt.Fprintf(&sb, `{"id":%d,"name":"item-%d","status":%q,"tags":["a","b"]}`, i, i, statuses[i%3])
	}
	sb.WriteString(`]}`)
	svc, err := jq.NewService([]byte(sb.String()))
	if err != nil {
		t.Fatal(err)
	}
	return svc
}

func modelFor(t testing.TB, svc *jq.Service, width, height int) Model {
	t.Helper()
	hist, err := history.NewStore(filepath.Join(t.TempDir(), "hist.json"))
	if err != nil {
		t.Fatal(err)
	}
	m := NewModel(svc, autocomplete.NewService(svc), hist, clipboard.NewService(), Config{Filename: "t.json", Filepath: "/t.json"})
	next, _ := m.Update(tea.WindowSizeMsg{Width: width, Height: height})
	return next.(Model)
}

// startQuery requests and dispatches filter, runs the query command and
// applies its resultMsg (the first frame). It returns the message and the
// follow-up command (waiting for more values / the index).
func startQuery(t testing.TB, m *Model, filter string) (resultMsg, tea.Cmd) {
	t.Helper()
	m.setFilter(filter)
	seq := m.query.request()
	msg, ok := m.startExecute(seq)().(resultMsg)
	if !ok {
		t.Fatal("query command did not return a resultMsg")
	}
	var next tea.Cmd
	*m, next = send(*m, msg)
	return msg, next
}

func waitClosed(t testing.TB, ch <-chan struct{}, what string) {
	t.Helper()
	select {
	case <-ch:
	case <-time.After(10 * time.Second):
		t.Fatalf("%s did not stop after cancellation", what)
	}
}

func waitGoroutines(t testing.TB, base int) {
	t.Helper()
	for i := 0; i < 300 && runtime.NumGoroutine() > base; i++ {
		time.Sleep(10 * time.Millisecond)
	}
	if n := runtime.NumGoroutine(); n > base {
		t.Fatalf("goroutine leak: %d -> %d", base, n)
	}
}

func TestStreamingProgressRearmedUntilDone(t *testing.T) {
	m := modelFor(t, bigService(t, 100000), 100, 30)
	_, next := startQuery(t, &m, `.items[] | .id`)
	if m.query.phase != queryStreaming || !m.query.running() || next == nil {
		t.Fatalf("after first frame: phase=%v cmd=%v", m.query.phase, next)
	}
	if n, exact := m.result.lineCount(); exact || n < m.firstScreenLines() {
		t.Fatalf("first screen lines = %d exact=%v", n, exact)
	}
	if !strings.Contains(m.renderFooter(), "lines: ") || !strings.Contains(m.renderFooter(), "+") {
		t.Fatalf("footer should show inexact counts: %q", ansi.Strip(m.renderFooter()))
	}
	if !strings.Contains(ansi.Strip(m.renderHeader()), "Running...") {
		t.Fatal("header should show Running... while streaming")
	}

	progress, indexed := 0, 0
	for next != nil {
		msg := next()
		switch msg.(type) {
		case resultProgressMsg:
			progress++
			if m.query.phase != queryStreaming {
				t.Fatalf("progress while %v", m.query.phase)
			}
		case indexMsg:
			indexed++
			if m.query.phase != queryIndexing {
				t.Fatalf("index msg while %v", m.query.phase)
			}
		default:
			t.Fatalf("unexpected message %T", msg)
		}
		m, next = send(m, msg)
	}
	if progress == 0 || indexed == 0 {
		t.Fatalf("progress=%d index=%d msgs", progress, indexed)
	}
	if m.query.phase != queryIdle || m.query.running() || m.query.cancel != nil {
		t.Fatalf("not idle at the end: %+v", m.query)
	}
	if !m.result.complete() {
		t.Fatal("result not complete")
	}
	if n, exact := m.result.lineCount(); n != 100000 || !exact {
		t.Fatalf("lines = %d exact=%v", n, exact)
	}
	foot := ansi.Strip(m.renderFooter())
	if !strings.Contains(foot, "results: 100,000") || !strings.Contains(foot, "lines: 100,000") || strings.Contains(foot, "+") {
		t.Fatalf("footer should show exact counts: %q", foot)
	}
}

func TestStaleMessagesIgnored(t *testing.T) {
	m := modelFor(t, bigService(t, 50000), 100, 30)
	old, oldNext := startQuery(t, &m, `.items[] | .id`)
	if oldNext == nil {
		t.Fatal("expected a wait command")
	}
	progress := oldNext().(resultProgressMsg)

	_, _ = startQuery(t, &m, `.items[] | .name`) // supersedes the first
	snap, filter := m.result.snap, m.result.filter

	// Late messages of the superseded query: dropped and not re-armed.
	for _, msg := range []tea.Msg{old, progress, indexMsg{seq: old.seq, doc: snap.Doc, index: nil}} {
		var cmd tea.Cmd
		m, cmd = send(m, msg)
		if cmd != nil || m.result.snap != snap || m.result.filter != filter {
			t.Fatalf("%T from a superseded query was applied (cmd=%v)", msg, cmd != nil)
		}
	}

	// An index message for another document of the live query is dropped too.
	other := jsonview.NewDoc([]any{1})
	ix, _ := other.BuildIndex(t.Context(), 0, nil)
	m.query.phase = queryIndexing
	m, cmd := send(m, indexMsg{seq: m.query.active, doc: other, index: ix})
	if cmd != nil || m.result.index != nil {
		t.Fatal("index of a foreign document applied")
	}
	m.query.stop()
}

func TestNewQueryCancelsCollector(t *testing.T) {
	base := runtime.NumGoroutine()
	m := modelFor(t, bigService(t, 10), 100, 30)
	old, _ := startQuery(t, &m, `repeat(1)`) // endless
	if old.snap.Done {
		t.Fatal("endless query finished")
	}
	_, next := startQuery(t, &m, `.items[0]`)
	waitClosed(t, old.collector.Done(), "collector")
	if next != nil {
		m = settle(t, m, next)
	}
	m.query.stop()
	waitGoroutines(t, base)
}

func TestNewQueryCancelsIndexBuild(t *testing.T) {
	base := runtime.NumGoroutine()
	m := modelFor(t, bigService(t, 150000), 100, 30)
	_, next := startQuery(t, &m, `.items[] | .tags`)
	// Drive collection to its end, then start (but do not finish) indexing.
	for m.query.phase == queryStreaming {
		m, next = send(m, next())
	}
	if m.query.phase != queryIndexing {
		t.Fatalf("phase = %v", m.query.phase)
	}
	done := make(chan tea.Msg, 1)
	go func() { done <- next() }() // builds the index; first publish after ~50ms

	// A new query supersedes it; the builder must stop and the waiting
	// command must return.
	_, _ = startQuery(t, &m, `.items[0]`)
	select {
	case msg := <-done:
		if _, ok := msg.(indexMsg); !ok {
			t.Fatalf("got %T", msg)
		}
	case <-time.After(10 * time.Second):
		t.Fatal("index command still blocked after cancellation")
	}
	m.query.stop()
	waitGoroutines(t, base)
}

func TestQuitCancelsQuery(t *testing.T) {
	base := runtime.NumGoroutine()
	m := modelFor(t, bigService(t, 10), 100, 30)
	msg, _ := startQuery(t, &m, `repeat(1)`)
	m, _ = send(m, key("ctrl+c"))
	if m.query.phase != queryIdle || m.query.cancel != nil {
		t.Fatalf("query still active after quit: %+v", m.query)
	}
	waitClosed(t, msg.collector.Done(), "collector")
	waitGoroutines(t, base)
}

func TestEnterWhileStreamingOrStaleOutputsFullResult(t *testing.T) {
	svc := bigService(t, 30000)
	filters := []string{`.`, `.items`, `.items[0]`, `.items[] | select(.status == "ok") | .id`, `.items[] | {id, s: .status}`}
	for _, f := range filters {
		want := referenceResult(t, svc, f)
		for _, mode := range []string{"streaming", "stale", "complete"} {
			t.Run(mode+"/"+f, func(t *testing.T) {
				m := modelFor(t, svc, 100, 30)
				switch mode {
				case "streaming":
					_, _ = startQuery(t, &m, f)
				case "stale":
					_, _ = startQuery(t, &m, `.items[0].name`)
					m.setFilter(f)
				case "complete":
					m = runFilter(t, m, f)
				}
				next, cmd := m.Update(key("enter"))
				got := next.(Model)
				out, ok := outputText(t, got)
				if !ok || out != want {
					t.Fatalf("output differs from Execute (%d vs %d bytes, ok=%v)", len(out), len(want), ok)
				}
				if got.query.phase != queryIdle {
					t.Fatalf("query still in flight after Enter: %v", got.query.phase)
				}
				if _, isQuit := cmd().(tea.QuitMsg); !isQuit {
					t.Fatal("expected quit")
				}
				if h := got.history.Get("/t.json"); len(h) == 0 || h[0] != f {
					t.Fatalf("history = %v", h)
				}
			})
		}
	}
}

func TestCopyTextIsFullResult(t *testing.T) {
	svc := bigService(t, 30000)
	f := `.items[] | .id`
	m := modelFor(t, svc, 100, 30)
	_, _ = startQuery(t, &m, f)
	if m.result.complete() {
		t.Fatal("test needs the result to still be streaming")
	}
	// While streaming, copy evaluates the full result rather than copying
	// the partial one, and leaves the background query running.
	if text, problem := m.resultText(); problem != "" || text != referenceResult(t, svc, f) {
		t.Fatalf("copy while streaming: problem=%q len=%d", problem, len(text))
	}
	if !m.query.running() {
		t.Fatal("copy must not stop the background query")
	}
	m = runFilter(t, m, f)
	text, problem := m.resultText()
	if problem != "" || text != referenceResult(t, svc, f) {
		t.Fatalf("copy text differs: problem=%q len=%d", problem, len(text))
	}
	m = runFilter(t, m, `empty`)
	if _, problem := m.resultText(); problem != "Nothing to copy" {
		t.Fatalf("empty: %q", problem)
	}
	m = runFilter(t, m, `error("x")`)
	if _, problem := m.resultText(); problem != "Nothing to copy" {
		t.Fatalf("error: %q", problem)
	}
}

func TestScrollClampedBeforeIndexThenExtends(t *testing.T) {
	m := modelFor(t, bigService(t, 100000), 100, 30)
	_, next := startQuery(t, &m, `.items[] | .id`)
	known, exact := m.result.lineCount()
	if exact || known >= 100000 {
		t.Fatalf("known=%d exact=%v", known, exact)
	}
	m.scrollY(1 << 30)
	if want := known - m.contentHeight(); m.yOffset != want {
		t.Fatalf("streaming: yOffset=%d, want %d", m.yOffset, want)
	}
	m = settle(t, m, next)
	m.scrollY(1 << 30)
	if want := 100000 - m.contentHeight(); m.yOffset != want {
		t.Fatalf("indexed: yOffset=%d, want %d", m.yOffset, want)
	}
	rows := strings.Split(ansi.Strip(m.renderContent()), "\n")
	if !strings.Contains(rows[len(rows)-2], "99999") {
		t.Fatalf("last row = %q", rows[len(rows)-2])
	}
}

func TestHorizontalScrollBeforeIndex(t *testing.T) {
	long := strings.Repeat("x", 500)
	m := modelFor(t, bigService(t, 10), 100, 30)
	m.filter.SetValue(`"long"`)
	m = deliverValuesNoIndex(t, m, long)
	if w, exact := m.result.maxWidth(); exact || w < 500 {
		t.Fatalf("maxWidth = %d exact=%v", w, exact)
	}
	if m.maxHorizontalOffset() == 0 {
		t.Fatal("cannot h-scroll before the index exists")
	}
	m.scrollHorizontal(1 << 20)
	if m.outputXOffset != m.maxHorizontalOffset() {
		t.Fatalf("x offset = %d", m.outputXOffset)
	}
}

// deliverValuesNoIndex shows a complete snapshot but does not run the index.
func deliverValuesNoIndex(t testing.TB, m Model, values ...any) Model {
	t.Helper()
	seq := m.query.latest
	m.query.dispatch(seq)
	snap := &jsonview.Snapshot{Values: values, Doc: jsonview.NewDoc(values), Done: true}
	m, _ = send(m, resultMsg{seq: seq, filter: m.filter.Value(), snap: snap})
	return m
}

func TestErrors(t *testing.T) {
	svc := bigService(t, 60000)
	cases := map[string]string{
		"mid-stream":    `.items[] | if .id == 40000 then error("boom") else .id end`,
		"first screen":  `.items[] | if .id == 1 then error("boom") else .id end`,
		"compile error": `.items[`,
	}
	for name, f := range cases {
		t.Run(name, func(t *testing.T) {
			m := modelFor(t, svc, 100, 30)
			m = runFilter(t, m, f)
			if !m.result.hasError() || m.query.phase != queryIdle {
				t.Fatalf("err=%v phase=%v", m.result.err, m.query.phase)
			}
			header := ansi.Strip(m.renderHeader())
			if !strings.Contains(header, "Error: ") {
				t.Fatalf("header = %q", header)
			}
			pane := m.renderContent()
			errPre, _ := errorWrap()
			if !strings.Contains(pane, errPre) || strings.Contains(ansi.Strip(pane), "39999") {
				t.Fatalf("pane should show only the error:\n%s", ansi.Strip(pane))
			}
			if n, exact := m.result.lineCount(); !exact || n == 0 {
				t.Fatalf("error lines = %d exact=%v", n, exact)
			}
			next, cmd := m.Update(key("enter"))
			if _, ok := next.(Model).Output(); ok {
				t.Fatal("error result must not be output")
			}
			if _, isQuit := cmd().(tea.QuitMsg); !isQuit {
				t.Fatal("expected quit")
			}
		})
	}
}

func TestEmptyResult(t *testing.T) {
	m := modelFor(t, bigService(t, 10), 100, 30)
	m = runFilter(t, m, `empty`)
	if n, exact := m.result.lineCount(); n != 0 || !exact {
		t.Fatalf("lines = %d exact=%v", n, exact)
	}
	if rows := strings.Split(m.renderContent(), "\n"); len(rows) != m.contentHeight()+2 {
		t.Fatalf("rows = %d", len(rows))
	}
	_ = m.renderView()
	next, cmd := m.Update(key("enter"))
	if _, ok := next.(Model).Output(); ok {
		t.Fatal("empty result must not be output")
	}
	if h := next.(Model).history.Get("/t.json"); len(h) != 0 {
		t.Fatalf("history = %v", h)
	}
	if _, isQuit := cmd().(tea.QuitMsg); !isQuit {
		t.Fatal("expected quit")
	}
}

// The rendered window must equal the old whole-string pipeline's lines.
func TestWindowsMatchOldPipelineOnTestdata(t *testing.T) {
	files, _ := filepath.Glob("../../testdata/*.json")
	if len(files) == 0 {
		t.Fatal("no testdata")
	}
	for _, file := range files {
		data, err := os.ReadFile(file)
		if err != nil {
			t.Fatal(err)
		}
		svc, err := jq.NewService(data)
		if err != nil {
			t.Fatal(err)
		}
		for _, f := range []string{`.`, `..`, `[.. | scalars]`, `.. | strings`, `[paths]`} {
			res := referenceResult(t, svc, f)
			var want []string
			if res != "" {
				want = strings.Split(res, "\n")
			}
			m := runFilter(t, modelFor(t, svc, 120, 20), f)
			if n, exact := m.result.lineCount(); n != len(want) || !exact {
				t.Fatalf("%s %s: lines %d exact=%v, want %d", file, f, n, exact, len(want))
			}
			maxW := 0
			for _, l := range want {
				maxW = max(maxW, lineDisplayWidth(l))
			}
			if w, exact := m.result.maxWidth(); w != maxW || !exact {
				t.Fatalf("%s %s: maxWidth %d exact=%v, want %d", file, f, w, exact, maxW)
			}
			for _, h := range []int{1, 12, 40} {
				for start := 0; start < len(want)+2; start += 7 {
					end := min(start+h, len(want))
					var exp []string
					if start < end {
						exp = want[start:end]
					}
					got := m.result.window(start, h).lines
					if strings.Join(got, "\n") != strings.Join(exp, "\n") || len(got) != len(exp) {
						t.Fatalf("%s %s: window(%d,%d) differs", file, f, start, h)
					}
				}
			}
		}
	}
}

func TestSummaryFormatting(t *testing.T) {
	for n, want := range map[int]string{0: "0", 999: "999", 1000: "1,000", 69394: "69,394", 3120044: "3,120,044"} {
		if got := groupDigits(n); got != want {
			t.Errorf("groupDigits(%d) = %q, want %q", n, got, want)
		}
	}
	r := newResultView()
	vals := make([]any, 69394)
	r.snap = &jsonview.Snapshot{Values: vals, Doc: jsonview.NewDoc(vals)}
	r.known = 3_100_000
	if got := r.summary(); got != "results: 69,394+  lines: 3.1M+" {
		t.Fatalf("summary = %q", got)
	}
	r.snap.Done, r.knownExact = true, true
	if got := r.summary(); got != "results: 69,394  lines: 3,100,000" {
		t.Fatalf("summary = %q", got)
	}
}

func TestQueryPhases(t *testing.T) {
	var q queryState
	if q.live(0) || q.running() {
		t.Fatal("zero state must be idle")
	}
	ctx := q.dispatch(q.request())
	if !q.live(1) || !q.running() || q.phase != queryStreaming {
		t.Fatalf("after dispatch: %+v", q)
	}
	q.collected()
	if q.phase != queryIndexing || q.running() || !q.live(1) {
		t.Fatalf("after collected: %+v", q)
	}
	ctx2 := q.dispatch(q.request()) // supersedes
	if ctx.Err() == nil || ctx2.Err() != nil || q.live(1) || !q.live(2) {
		t.Fatalf("after supersede: %+v", q)
	}
	q.stop()
	if ctx2.Err() == nil || q.live(2) || q.phase != queryIdle {
		t.Fatalf("after stop: %+v", q)
	}
	q.collected() // no-op when idle
	if q.phase != queryIdle {
		t.Fatal("collected must not revive an idle query")
	}
}

func TestNaNAndInfRender(t *testing.T) {
	m := modelFor(t, bigService(t, 10), 100, 30)
	m = runFilter(t, m, `[nan, infinite, -infinite]`)
	text, problem := m.resultText()
	if problem != "" || !strings.Contains(text, "null") || !strings.Contains(text, "1.7976931348623157e+308") {
		t.Fatalf("text = %q problem=%q", text, problem)
	}
}

func referenceResult(t testing.TB, svc *jq.Service, filter string) string {
	t.Helper()
	result, err := jsontest.RunAndFormat(context.Background(), svc, filter)
	if err != nil {
		t.Fatal(err)
	}
	return result
}
