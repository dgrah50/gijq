package ui

import (
	"context"
	"time"

	"github.com/charmbracelet/bubbles/textinput"
	"github.com/charmbracelet/bubbles/viewport"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/mattn/go-runewidth"

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
	output viewport.Model
	result jq.Result
	lines  []string
	// Output geometry state
	outputXOffset int
	maxLineWidth  int

	// Autocomplete state
	suggestions   []string
	selectedIdx   int
	acContext     autocomplete.Context
	keysPath      string
	keysInFlight  string
	availableKeys []string

	// History state
	historyItems []string
	historyIdx   int

	// Query execution state
	querySeq       int
	activeQuerySeq int
	queryCancel    context.CancelFunc
	queryRunning   bool

	// Render cache
	colorCache *lineColorCache
	telemetry  *latencyTelemetry

	// UI state
	mode        Mode
	filename    string
	filepath    string
	status      string
	statusClear time.Time
	width       int
	height      int
	ready       bool
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
	ti.Width = 50
	ti.SetValue(".")
	initialCtx := acSvc.ParseContext(ti.Value())

	return Model{
		jq:           jqSvc,
		autocomplete: acSvc,
		history:      hist,
		clipboard:    clip,
		filter:       ti,
		filename:     cfg.Filename,
		filepath:     cfg.Filepath,
		mode:         ModeNormal,
		acContext:    initialCtx,
		keysInFlight: initialCtx.Path,
		querySeq:     1,
		colorCache:   newLineColorCache(4096),
		lines:        []string{""},
		maxLineWidth: 0,
		telemetry:    newLatencyTelemetry(cfg.Telemetry),
	}
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
}

type executeQueryMsg struct {
	seq int
}

type keysMsg struct {
	path string
	keys []string
	err  error
}

type statusClearMsg struct{}

func clearStatusAfter(d time.Duration) tea.Cmd {
	return tea.Tick(d, func(time.Time) tea.Msg {
		return statusClearMsg{}
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
		return resultMsg{seq: seq, result: result}
	}
}

func maxDisplayLineWidth(lines []string) int {
	maxWidth := 0
	for _, line := range lines {
		w := runewidth.StringWidth(line)
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
func (m Model) View() string {
	if !m.ready {
		return "Loading..."
	}
	return m.renderView()
}
