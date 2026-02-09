package ui

import (
	"time"

	"github.com/charmbracelet/bubbles/textinput"
	"github.com/charmbracelet/bubbles/viewport"
	tea "github.com/charmbracelet/bubbletea"

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
)

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

	// Autocomplete state
	suggestions []string
	selectedIdx int
	acContext   autocomplete.Context

	// History state
	historyItems []string
	historyIdx   int

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
	Filename string
	Filepath string
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

	return Model{
		jq:           jqSvc,
		autocomplete: acSvc,
		history:      hist,
		clipboard:    clip,
		filter:       ti,
		filename:     cfg.Filename,
		filepath:     cfg.Filepath,
		mode:         ModeNormal,
	}
}

// Init initializes the model
func (m Model) Init() tea.Cmd {
	return tea.Batch(
		textinput.Blink,
		m.executeFilter(),
	)
}

// executeFilter runs the jq filter and returns a command
func (m Model) executeFilter() tea.Cmd {
	filter := m.filter.Value()
	return func() tea.Msg {
		result := m.jq.Execute(filter)
		return resultMsg{result: result}
	}
}

// Message types
type resultMsg struct {
	result jq.Result
}

type statusClearMsg struct{}

func clearStatusAfter(d time.Duration) tea.Cmd {
	return tea.Tick(d, func(time.Time) tea.Msg {
		return statusClearMsg{}
	})
}

// View renders the UI
func (m Model) View() string {
	if !m.ready {
		return "Loading..."
	}
	return m.renderView()
}
