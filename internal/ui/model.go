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

const queryDebounce = 30 * time.Millisecond

// Model is the Bubble Tea model
type Model struct {
	// Services
	jq           *jq.Service
	autocomplete *autocomplete.Service
	history      *history.Store
	clipboard    *clipboard.Service

	// Input/Output
	filter textinput.Model
	result jq.Result
	// resultFilter is the filter that produced result.
	resultFilter string
	lines        []string
	// yOffset is the index of the first visible output line.
	yOffset int
	// Output geometry state
	outputXOffset int
	maxLineWidth  int

	// Autocomplete state
	suggestions []string
	selectedIdx int
	acContext   autocomplete.Context
	keys        keysState

	// History state
	historyItems []string
	historyIdx   int

	// Query execution state
	querySeq       int
	activeQuerySeq int
	queryCancel    context.CancelFunc
	queryRunning   bool

	// Render cache
	hl        *highlightCache
	telemetry *latencyTelemetry

	// UI state
	mode      Mode
	filename  string
	filepath  string
	status    string
	statusSeq int
	helpLine  string
	width     int
	height    int
	ready     bool

	// Output printed to stdout by the caller after the program exits.
	output    string
	confirmed bool
}

// Output returns the text to print to stdout after the TUI exits and whether
// the user confirmed it with Enter.
func (m Model) Output() (string, bool) {
	return m.output, m.confirmed
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
	initialCtx := acSvc.ParseContext(ti.Value())

	m := Model{
		jq:           jqSvc,
		autocomplete: acSvc,
		history:      hist,
		clipboard:    clip,
		filter:       ti,
		filename:     cfg.Filename,
		filepath:     cfg.Filepath,
		mode:         ModeNormal,
		acContext:    initialCtx,
		querySeq:     1,
		hl:           newHighlightCache(),
		lines:        []string{""},
		maxLineWidth: 0,
		telemetry:    newLatencyTelemetry(cfg.Telemetry),
	}
	m.keys.request(m.currentPath())
	return m
}

// Init initializes the model
func (m Model) Init() tea.Cmd {
	return tea.Batch(
		textinput.Blink,
		func() tea.Msg { return executeQueryMsg{seq: m.querySeq} },
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
	// Highlight checkpoints for long lines, keyed by line index.
	checkpoints map[int][]hlCheckpoint
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

// setStatus shows a status message and returns the command that clears it.
// Only the most recent status's timer can clear it.
func (m *Model) setStatus(text string) tea.Cmd {
	m.statusSeq++
	m.status = text
	seq := m.statusSeq
	return tea.Tick(3*time.Second, func(time.Time) tea.Msg {
		return statusClearMsg{seq: seq}
	})
}

func (m Model) fetchKeys(path string) tea.Cmd {
	if path == "" {
		path = "."
	}
	return func() tea.Msg {
		keys, err := m.jq.KeysAt(path)
		return keysMsg{path: path, keys: keys, err: err}
	}
}

func (m *Model) queueExecute() tea.Cmd {
	m.querySeq++
	seq := m.querySeq
	m.telemetry.OnQueued(seq)
	return tea.Tick(queryDebounce, func(time.Time) tea.Msg {
		return executeQueryMsg{seq: seq}
	})
}

func (m *Model) executeNow() tea.Cmd {
	m.querySeq++
	m.telemetry.OnQueued(m.querySeq)
	return m.startExecute(m.querySeq)
}

func (m *Model) startExecute(seq int) tea.Cmd {
	if m.queryCancel != nil {
		m.queryCancel()
		m.queryCancel = nil
	}

	m.activeQuerySeq = seq
	m.queryRunning = true
	m.telemetry.OnDispatch(seq)

	filter := m.filter.Value()
	ctx, cancel := context.WithCancel(context.Background())
	m.queryCancel = cancel

	return func() tea.Msg {
		result := m.jq.ExecuteWithContext(ctx, filter)
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
	var lines []string
	if result.Error != nil {
		lines = strings.Split(result.Error.Error(), "\n")
	} else {
		lines = strings.Split(result.Raw, "\n")
	}
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
		w := lineDisplayWidth(line)
		if w > maxWidth {
			maxWidth = w
		}
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
