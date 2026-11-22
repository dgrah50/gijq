package ui

import (
	"context"
	"time"

	"charm.land/bubbles/v2/textinput"
	tea "charm.land/bubbletea/v2"

	"github.com/dayangraham/gijq/internal/autocomplete"
	"github.com/dayangraham/gijq/internal/clipboard"
	"github.com/dayangraham/gijq/internal/history"
	"github.com/dayangraham/gijq/internal/jq"
	"github.com/dayangraham/gijq/internal/jsonview"
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
//   - The result is a resultView (result.go): lazily rendered text over the
//     values collected so far. Only Update changes it; View merely reads it
//     (the window memo it owns is pure memoisation).
//   - Every goroutine started for a query (collector, index builder, the
//     commands waiting on them) runs under queryState's context and is
//     cancelled by queryState.stop; a new dispatch, quit and a synchronous
//     Enter all go through it.
type Model struct {
	// Services
	jq           *jq.Service
	autocomplete *autocomplete.Service
	history      *history.Store
	clipboard    *clipboard.Service

	// Filter input and the last accepted result.
	filter textinput.Model
	query  queryState
	result resultView

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
	output *jsonview.Doc
}

// queryPhase is where the dispatched query is in its life.
type queryPhase int

const (
	// queryIdle: nothing in flight (nothing dispatched yet, finished, failed
	// or stopped). No goroutine belongs to the query.
	queryIdle queryPhase = iota
	// queryStreaming: the collector is draining the gojq iterator into
	// snapshots. The header shows "Running...".
	queryStreaming
	// queryIndexing: the result is fully collected and a background index
	// build (line count, widest line, random access) is publishing snapshots.
	queryIndexing
)

// queryState tracks jq evaluations of the filter. Every evaluation request
// gets a sequence number. The lifecycle is:
//
//	idle --request--> debouncing --dispatch--> streaming --collected--> indexing --indexed--> idle
//	                                              |                                  |
//	                                              +--- error / stop ---> idle <------+
//
// "debouncing" is implicit: latest != active. A newer request supersedes
// older ones: their debounce ticks are dropped and, once a newer one is
// dispatched, everything the older one started is cancelled and its
// messages are ignored (live is false for them).
//
// queryState is the single owner of seq, the phase and the context under
// which all of a query's goroutines run.
type queryState struct {
	latest int                // seq of the most recent request
	active int                // seq of the most recently dispatched request
	phase  queryPhase         // phase of active
	ctx    context.Context    // non-nil while phase != queryIdle
	cancel context.CancelFunc // non-nil while phase != queryIdle
}

// request returns the seq for a new evaluation that supersedes all others.
func (q *queryState) request() int {
	q.latest++
	return q.latest
}

// isLatest reports whether seq is still the most recent request.
func (q *queryState) isLatest(seq int) bool { return seq == q.latest }

// dispatch starts seq streaming, cancelling any evaluation still in flight
// (collector and index build), and returns the context everything started
// for seq must run under.
func (q *queryState) dispatch(seq int) context.Context {
	q.stop()
	q.ctx, q.cancel = context.WithCancel(context.Background())
	q.active, q.phase = seq, queryStreaming
	return q.ctx
}

// live reports whether messages for seq should still be applied: seq is the
// dispatched query and it has work in flight.
func (q *queryState) live(seq int) bool { return seq == q.active && q.phase != queryIdle }

// collected moves streaming to indexing.
func (q *queryState) collected() {
	if q.phase == queryStreaming {
		q.phase = queryIndexing
	}
}

// stop cancels everything in flight and returns to idle. It is the only
// way out of streaming/indexing (finishing, failing and superseding included).
func (q *queryState) stop() {
	if q.cancel != nil {
		q.cancel()
	}
	q.ctx, q.cancel = nil, nil
	q.phase = queryIdle
}

// running reports whether results are still being collected (header
// indicator). Indexing is shown by inexact counts in the footer instead.
func (q queryState) running() bool { return q.phase == queryStreaming }

// Output returns the document to print to stdout after the TUI exits, and
// whether there is any (the user confirmed a successful, non-empty result
// with Enter). The caller streams it with Doc.WriteTo.
func (m Model) Output() (*jsonview.Doc, bool) {
	return m.output, m.output != nil
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
		result:       newResultView(),
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

// resultMsg is the first message of a dispatched query: the first screen is
// ready (or the query failed to compile).
type resultMsg struct {
	seq    int
	filter string // filter that produced the result
	// err is a compile/parse failure; collector and snap are nil then.
	err error
	// collector is nil only if snap is already Done.
	collector *jsonview.Collector
	snap      *jsonview.Snapshot
}

// resultProgressMsg carries a newer collector snapshot.
type resultProgressMsg struct {
	seq       int
	collector *jsonview.Collector
	snap      *jsonview.Snapshot
}

// indexMsg carries a newer index snapshot for doc. index is nil when the
// builder ended without a result (cancelled).
type indexMsg struct {
	seq   int
	doc   *jsonview.Doc
	index *jsonview.Index
	ch    <-chan *jsonview.Index
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
	acSvc := m.autocomplete
	filter := m.filter.Value()
	return func() tea.Msg {
		keys, _ := acSvc.Suggest(filter)
		return keysMsg{path: path, keys: keys}
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

// firstScreenMargin is how many lines beyond the visible height the first
// message guarantees (when the result has them), so small scrolls work before
// the index exists.
const firstScreenMargin = 16

// firstScreenLines is how many lines the collector must have ready before the
// first frame.
func (m Model) firstScreenLines() int {
	return max(m.contentHeight(), 0) + firstScreenMargin
}

// startExecute dispatches the current filter. The returned command (run off
// the UI goroutine) compiles it and collects the first screen; the rest of
// the result streams in through resultProgressMsg.
func (m *Model) startExecute(seq int) tea.Cmd {
	ctx := m.query.dispatch(seq)
	m.telemetry.OnDispatch(seq)

	jqSvc, filter, screen := m.jq, m.filter.Value(), m.firstScreenLines()
	return func() tea.Msg {
		iter, err := jqSvc.Iter(ctx, filter)
		if err != nil {
			return resultMsg{seq: seq, filter: filter, err: err}
		}
		c := jsonview.Collect(ctx, iter, jsonview.CollectOptions{FirstScreenLines: screen})
		return resultMsg{seq: seq, filter: filter, collector: c, snap: c.Snapshot()}
	}
}

// waitProgress waits for the collector to publish (or finish, or the query to
// be cancelled) and reports its latest snapshot. It is re-armed by Update
// until the snapshot is Done; cancellation closes c.Done, so it never leaks.
func waitProgress(ctx context.Context, seq int, c *jsonview.Collector) tea.Cmd {
	return func() tea.Msg {
		select {
		case <-c.Updated():
		case <-c.Done():
		case <-ctx.Done():
		}
		return resultProgressMsg{seq: seq, collector: c, snap: c.Snapshot()}
	}
}

// startIndex builds the index of doc in a goroutine bound to ctx, and waits
// for its first snapshot. The builder publishes into a one-slot channel
// where the newest snapshot replaces an unread older one; the channel is
// closed when the builder returns (finished or cancelled).
func startIndex(ctx context.Context, seq int, doc *jsonview.Doc) tea.Cmd {
	return func() tea.Msg {
		ch := make(chan *jsonview.Index, 1)
		go func() {
			defer close(ch)
			_, _ = doc.BuildIndex(ctx, jsonview.DefaultStep, func(ix *jsonview.Index) {
				for {
					select {
					case ch <- ix:
						return
					default:
					}
					select { // drop the unread older snapshot
					case <-ch:
					default:
					}
				}
			})
		}()
		return waitIndex(ctx, seq, doc, ch)()
	}
}

// waitIndex waits for the next index snapshot.
func waitIndex(ctx context.Context, seq int, doc *jsonview.Doc, ch <-chan *jsonview.Index) tea.Cmd {
	return func() tea.Msg {
		select {
		case ix := <-ch: // nil when the builder ended without publishing
			return indexMsg{seq: seq, doc: doc, index: ix, ch: ch}
		case <-ctx.Done():
			return indexMsg{seq: seq, doc: doc, ch: ch}
		}
	}
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
