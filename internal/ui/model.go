package ui

import (
	"context"
	"strings"
	"time"

	"charm.land/bubbles/v2/textinput"
	tea "charm.land/bubbletea/v2"

	"github.com/dayangraham/gijq/internal/autocomplete"
	"github.com/dayangraham/gijq/internal/clipboard"
	"github.com/dayangraham/gijq/internal/history"
	"github.com/dayangraham/gijq/internal/jq"
)

// Mode represents the current UI mode
type Mode int

const (
	ModeNormal Mode = iota
	ModeAutocomplete
	ModeHistory
	ModeHelp
)

const (
	queryDebounce = 30 * time.Millisecond
	statusTimeout = 3 * time.Second
)

// Model is the Bubble Tea model.
//
// Invariants (maintained by the code in keys.go):
//   - Autocomplete context and the keys-pane path are always derived from the
//     filter text (see acContext/currentPath); they are never stored.
//   - Any key that changes the filter text queues a debounced query and a
//     keys fetch (see handleKey). Individual handlers never do this.
//   - Mode-specific data (suggestions, history list) only exists while in
//     that mode; every mode change goes through setMode.
type Model struct {
	// Services
	jq           *jq.Service
	autocomplete *autocomplete.Service
	history      *history.Store
	clipboard    *clipboard.Service

	// Filter input and the last accepted result.
	filter       textinput.Model
	query        queryState
	result       jq.Result
	resultFilter string // filter that produced result
	lines        []string
	maxLineWidth int
	// checkpoints lets the highlighter resume lexing deep inside long lines.
	// Built off the UI goroutine with each result; read-only afterwards.
	checkpoints map[int][]hlCheckpoint

	// Scroll position of the output pane.
	yOffset       int
	outputXOffset int

	// Mode and mode-specific data (see setMode).
	mode         Mode
	suggestions  []string // ModeAutocomplete only
	selectedIdx  int      // ModeAutocomplete only
	historyItems []string // ModeHistory only
	historyIdx   int      // ModeHistory only

	// Keys pane data for the current path.
	keys keysState

	// Transient status line; only the timer of the latest status clears it.
	status    string
	statusSeq int

	// Layout
	width    int
	height   int
	ready    bool   // a WindowSizeMsg has been received
	helpLine string // header help text, recomputed on resize

	filename  string
	filepath  string
	telemetry *latencyTelemetry

	// quitting is set once the program has been told to quit; no further
	// work is started after that.
	quitting bool
	// output is printed to stdout by the caller after the program exits.
	output string
}

// queryState tracks jq evaluations of the filter. Every evaluation request
// gets a sequence number. The lifecycle is:
//
//	idle --request--> debouncing --dispatch--> running --finish--> idle
//
// A newer request supersedes older ones: their debounce ticks are dropped and
// their results are ignored.
type queryState struct {
	latest int                // seq of the most recent request
	active int                // seq of the most recently dispatched request
	cancel context.CancelFunc // non-nil while active is running
}

// request returns the seq for a new evaluation that supersedes all others.
func (q *queryState) request() int {
	q.latest++
	return q.latest
}

// isLatest reports whether seq is still the most recent request.
func (q *queryState) isLatest(seq int) bool { return seq == q.latest }

// dispatch marks seq as running, cancelling any evaluation still in flight,
// and returns the context the evaluation must run under.
func (q *queryState) dispatch(seq int) context.Context {
	q.stop()
	ctx, cancel := context.WithCancel(context.Background())
	q.active, q.cancel = seq, cancel
	return ctx
}

// finish reports whether a result for seq should be accepted, and if so
// marks the query idle.
func (q *queryState) finish(seq int) bool {
	if seq != q.active {
		return false
	}
	q.stop()
	return true
}

// stop cancels the running evaluation, if any.
func (q *queryState) stop() {
	if q.cancel != nil {
		q.cancel()
		q.cancel = nil
	}
}

func (q queryState) running() bool { return q.cancel != nil }

// Output returns the text to print to stdout after the TUI exits, and
// whether there is any (the user confirmed a successful result with Enter).
func (m Model) Output() (string, bool) {
	return m.output, m.output != ""
}

// Config holds initialization options
type Config struct {
	Filename  string
	Filepath  string
	Telemetry bool
}

// NewModel creates a new UI model
func NewModel(
	jqSvc *jq.Service,
	acSvc *autocomplete.Service,
	hist *history.Store,
	clip *clipboard.Service,
	cfg Config,
) Model {
	ti := textinput.New()
	ti.Placeholder = "."
	ti.Focus()
	ti.CharLimit = 500
	ti.SetWidth(50)
	ti.SetValue(".")

	m := Model{
		jq:           jqSvc,
		autocomplete: acSvc,
		history:      hist,
		clipboard:    clip,
		filter:       ti,
		filename:     cfg.Filename,
		filepath:     cfg.Filepath,
		mode:         ModeNormal,
		lines:        []string{""},
		telemetry:    newLatencyTelemetry(cfg.Telemetry),
	}
	// Init dispatches these: the first query and the root keys.
	m.query.request()
	m.keys.request(m.currentPath())
	return m
}

// Init starts the requests queued by NewModel.
func (m Model) Init() tea.Cmd {
	return tea.Batch(
		textinput.Blink,
		func() tea.Msg { return executeQueryMsg{seq: m.query.latest} },
		m.fetchKeys(m.currentPath()),
	)
}

// Message types
type resultMsg struct {
	seq    int
	result jq.Result
	filter string // filter that produced result
	// Post-processed off the UI goroutine.
	lines        []string
	maxLineWidth int
	checkpoints  map[int][]hlCheckpoint
}

type executeQueryMsg struct {
	seq int
}

type keysMsg struct {
	path string
	keys []string
	err  error
}

type statusClearMsg struct{ seq int }

// acContext is the autocomplete context of the current filter text.
func (m Model) acContext() autocomplete.Context {
	return m.autocomplete.ParseContext(m.filter.Value())
}

// currentPath is the jq path whose keys the keys pane shows.
func (m Model) currentPath() string {
	if p := m.acContext().Path; p != "" {
		return p
	}
	return "."
}

// setMode switches mode and drops the data of every other mode.
func (m *Model) setMode(mode Mode) {
	m.mode = mode
	m.suggestions, m.selectedIdx = nil, 0
	m.historyItems, m.historyIdx = nil, 0
}

// setFilter replaces the filter text and moves the cursor to the end.
func (m *Model) setFilter(value string) {
	m.filter.SetValue(value)
	m.filter.CursorEnd()
}

// setStatus shows a status message and returns the command that clears it.
func (m *Model) setStatus(text string) tea.Cmd {
	m.statusSeq++
	m.status = text
	seq := m.statusSeq
	return tea.Tick(statusTimeout, func(time.Time) tea.Msg {
		return statusClearMsg{seq: seq}
	})
}

// fetchKeysIfNeeded starts a keys fetch when the current path changed.
func (m *Model) fetchKeysIfNeeded() tea.Cmd {
	path := m.currentPath()
	if !m.keys.request(path) {
		return nil
	}
	return m.fetchKeys(path)
}

func (m Model) fetchKeys(path string) tea.Cmd {
	jqSvc := m.jq
	return func() tea.Msg {
		keys, err := jqSvc.KeysAt(path)
		return keysMsg{path: path, keys: keys, err: err}
	}
}

// queueExecute requests an evaluation of the filter after the debounce delay.
func (m *Model) queueExecute() tea.Cmd {
	seq := m.query.request()
	m.telemetry.OnQueued(seq)
	return tea.Tick(queryDebounce, func(time.Time) tea.Msg {
		return executeQueryMsg{seq: seq}
	})
}

// startExecute evaluates the current filter in the background. All heavy
// post-processing happens in the returned command, off the UI goroutine.
func (m *Model) startExecute(seq int) tea.Cmd {
	ctx := m.query.dispatch(seq)
	m.telemetry.OnDispatch(seq)

	jqSvc, filter := m.jq, m.filter.Value()
	return func() tea.Msg {
		result := jqSvc.ExecuteWithContext(ctx, filter)
		lines, maxW := processResult(result)
		var cps map[int][]hlCheckpoint
		if ctx.Err() == nil {
			cps = prebuildCheckpoints(lines)
		}
		return resultMsg{seq: seq, result: result, filter: filter, lines: lines, maxLineWidth: maxW, checkpoints: cps}
	}
}

// processResult splits a result into display lines and measures the widest
// one. It is CPU heavy for large outputs and must run off the UI goroutine.
func processResult(result jq.Result) ([]string, int) {
	text := result.Raw
	if result.Error != nil {
		text = result.Error.Error()
	}
	lines := strings.Split(text, "\n")
	return lines, maxDisplayLineWidth(lines)
}

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

func maxDisplayLineWidth(lines []string) int {
	maxWidth := 0
	for _, line := range lines {
		maxWidth = max(maxWidth, lineDisplayWidth(line))
	}
	return maxWidth
}

// TelemetrySummary returns a printable latency summary when telemetry is enabled.
func (m Model) TelemetrySummary() (string, bool) {
	if m.telemetry == nil {
		return "", false
	}
	return m.telemetry.Summary()
}

// View renders the UI
func (m Model) View() tea.View {
	content := "Loading..."
	if m.ready {
		content = m.renderView()
	}
	v := tea.NewView(content)
	v.AltScreen = true
	return v
}
