package ui

import (
	"fmt"
	"os"
	"strings"
	"time"
	"unicode"

	"github.com/charmbracelet/bubbles/viewport"
	tea "github.com/charmbracelet/bubbletea"
)

// Update handles messages
func (m Model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.KeyMsg:
		return m.handleKey(msg)

	case tea.WindowSizeMsg:
		m.width = msg.Width
		m.height = msg.Height
		if !m.ready {
			m.output = newViewport(m.width, m.contentHeight())
			m.ready = true
		} else {
			m.output.Width = m.width - m.suggestWidth() - 5
			m.output.Height = m.contentHeight()
		}
		return m, nil

	case resultMsg:
		m.result = msg.result
		if m.ready {
			// Set raw content for viewport scrolling calculation
			if msg.result.Error != nil {
				m.output.SetContent(msg.result.Error.Error())
			} else {
				m.output.SetContent(msg.result.Raw)
			}
		}
		return m, nil

	case statusClearMsg:
		m.status = ""
		return m, nil
	}

	return m, nil
}

func (m Model) handleKey(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	key := msg.String()

	// Global keys
	switch key {
	case "ctrl+c":
		return m, tea.Quit

	case "esc":
		if m.mode != ModeNormal {
			m.mode = ModeNormal
			m.suggestions = nil
			return m, nil
		}
		return m, tea.Quit

	case "ctrl+y":
		return m.copyOutput()

	case "ctrl+f":
		return m.copyFilter()

	case "ctrl+h":
		m.mode = ModeHistory
		m.historyItems = m.history.Get(m.filepath)
		m.historyIdx = 0
		return m, nil
	}

	// Mode-specific
	switch m.mode {
	case ModeNormal:
		return m.handleNormalKey(msg)
	case ModeAutocomplete:
		return m.handleAutocompleteKey(msg)
	case ModeHistory:
		return m.handleHistoryKey(msg)
	}

	return m, nil
}

func (m Model) handleNormalKey(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	key := msg.String()

	switch key {
	case "enter":
		// Output result and quit
		if m.result.Error == nil && m.result.Raw != "" {
			fmt.Fprint(os.Stdout, m.result.Raw)
			// Save to history
			m.history.Add(m.filepath, m.filter.Value())
			m.history.Save()
		}
		return m, tea.Quit

	case "tab":
		m.mode = ModeAutocomplete
		filter := m.filter.Value()

		// If filter ends with ], append . to drill into sub-keys
		if strings.HasSuffix(filter, "]") {
			filter = filter + "."
			m.filter.SetValue(filter)
			m.filter.SetCursor(len(filter))
		}

		m.suggestions, m.acContext = m.autocomplete.Suggest(filter)
		m.selectedIdx = 0

		// If only suggestion exactly matches what's typed, drill deeper
		if len(m.suggestions) == 1 && m.suggestions[0] == m.acContext.Incomplete && m.acContext.Incomplete != "" {
			newFilter := filter[:m.acContext.StartPos] + m.suggestions[0] + "."
			m.filter.SetValue(newFilter)
			m.filter.SetCursor(len(newFilter))
			m.suggestions, m.acContext = m.autocomplete.Suggest(newFilter)
			m.selectedIdx = 0
		}

		return m, nil

	case "up":
		m.output.LineUp(1)
		return m, nil

	case "down":
		m.output.LineDown(1)
		return m, nil

	case "pgup":
		m.output.HalfViewUp()
		return m, nil

	case "pgdown":
		m.output.HalfViewDown()
		return m, nil

	case "alt+left", "alt+b", "ctrl+left":
		m.moveCursorToPrevWord()
		return m, nil

	case "alt+right", "alt+f", "ctrl+right":
		m.moveCursorToNextWord()
		return m, nil

	case "alt+backspace", "ctrl+backspace", "ctrl+w":
		if m.deletePrevWord() {
			_, m.acContext = m.autocomplete.Suggest(m.filter.Value())
			return m, m.executeFilter()
		}
		return m, nil

	case "alt+delete", "alt+d", "ctrl+delete":
		if m.deleteNextWord() {
			_, m.acContext = m.autocomplete.Suggest(m.filter.Value())
			return m, m.executeFilter()
		}
		return m, nil

	default:
		// Text input
		var cmd tea.Cmd
		m.filter, cmd = m.filter.Update(msg)
		// Update autocomplete context so Available keys panel stays in sync
		_, m.acContext = m.autocomplete.Suggest(m.filter.Value())
		return m, tea.Batch(cmd, m.executeFilter())
	}
}

func (m Model) handleAutocompleteKey(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	key := msg.String()

	switch key {
	case "tab", "down":
		if len(m.suggestions) > 0 {
			m.selectedIdx = (m.selectedIdx + 1) % len(m.suggestions)
		}
		return m, nil

	case "up", "shift+tab":
		if len(m.suggestions) > 0 {
			m.selectedIdx--
			if m.selectedIdx < 0 {
				m.selectedIdx = len(m.suggestions) - 1
			}
		}
		return m, nil

	case "enter":
		if len(m.suggestions) > 0 {
			selected := m.suggestions[m.selectedIdx]
			newFilter := m.autocomplete.Apply(m.filter.Value(), m.acContext, selected)
			m.filter.SetValue(newFilter)
			m.filter.SetCursor(len(newFilter))
		}
		m.mode = ModeNormal
		m.suggestions = nil
		return m, m.executeFilter()

	default:
		// Any other key exits autocomplete and processes normally
		m.mode = ModeNormal
		m.suggestions = nil
		return m.handleNormalKey(msg)
	}
}

func (m Model) handleHistoryKey(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	key := msg.String()

	switch key {
	case "down", "tab":
		if len(m.historyItems) > 0 {
			m.historyIdx = (m.historyIdx + 1) % len(m.historyItems)
		}
		return m, nil

	case "up", "shift+tab":
		if len(m.historyItems) > 0 {
			m.historyIdx--
			if m.historyIdx < 0 {
				m.historyIdx = len(m.historyItems) - 1
			}
		}
		return m, nil

	case "enter":
		if len(m.historyItems) > 0 {
			m.filter.SetValue(m.historyItems[m.historyIdx])
			m.filter.SetCursor(len(m.historyItems[m.historyIdx]))
		}
		m.mode = ModeNormal
		return m, m.executeFilter()

	default:
		m.mode = ModeNormal
		return m, nil
	}
}

func (m Model) copyOutput() (tea.Model, tea.Cmd) {
	if m.result.Raw == "" {
		m.status = "Nothing to copy"
		return m, clearStatusAfter(3 * time.Second)
	}
	if err := m.clipboard.Copy(m.result.Raw); err != nil {
		m.status = "Copy failed: " + err.Error()
	} else {
		m.status = "Copied output to clipboard"
	}
	return m, clearStatusAfter(3 * time.Second)
}

func (m *Model) moveCursorToPrevWord() {
	pos := m.filter.Position()
	nextPos := prevWordStart(m.filter.Value(), pos)
	m.filter.SetCursor(nextPos)
}

func (m *Model) moveCursorToNextWord() {
	pos := m.filter.Position()
	nextPos := nextWordStart(m.filter.Value(), pos)
	m.filter.SetCursor(nextPos)
}

func (m *Model) deletePrevWord() bool {
	value := m.filter.Value()
	pos := m.filter.Position()
	start := prevWordStart(value, pos)
	if start == pos {
		return false
	}

	r := []rune(value)
	newValue := string(append(r[:start], r[pos:]...))
	m.filter.SetValue(newValue)
	m.filter.SetCursor(start)
	return true
}

func (m *Model) deleteNextWord() bool {
	value := m.filter.Value()
	pos := m.filter.Position()
	end := nextWordDeleteEnd(value, pos)
	if end == pos {
		return false
	}

	r := []rune(value)
	newValue := string(append(r[:pos], r[end:]...))
	m.filter.SetValue(newValue)
	m.filter.SetCursor(pos)
	return true
}

func prevWordStart(value string, pos int) int {
	r := []rune(value)
	if pos <= 0 {
		return 0
	}
	if pos > len(r) {
		pos = len(r)
	}

	i := pos
	for i > 0 && !isWordRune(r[i-1]) {
		i--
	}
	for i > 0 && isWordRune(r[i-1]) {
		i--
	}
	return i
}

func nextWordStart(value string, pos int) int {
	r := []rune(value)
	if pos < 0 {
		return 0
	}
	if pos >= len(r) {
		return len(r)
	}

	i := pos
	for i < len(r) && isWordRune(r[i]) {
		i++
	}
	for i < len(r) && !isWordRune(r[i]) {
		i++
	}
	return i
}

func nextWordDeleteEnd(value string, pos int) int {
	r := []rune(value)
	if pos < 0 {
		return 0
	}
	if pos >= len(r) {
		return len(r)
	}

	i := pos
	if isWordRune(r[i]) {
		for i < len(r) && isWordRune(r[i]) {
			i++
		}
		return i
	}

	for i < len(r) && !isWordRune(r[i]) {
		i++
	}
	for i < len(r) && isWordRune(r[i]) {
		i++
	}
	return i
}

func isWordRune(r rune) bool {
	return unicode.IsLetter(r) || unicode.IsDigit(r) || r == '_'
}

func (m Model) copyFilter() (tea.Model, tea.Cmd) {
	filter := m.filter.Value()
	if filter == "" {
		m.status = "Nothing to copy"
		return m, clearStatusAfter(3 * time.Second)
	}
	if err := m.clipboard.Copy(filter); err != nil {
		m.status = "Copy failed: " + err.Error()
	} else {
		m.status = "Copied filter to clipboard"
	}
	return m, clearStatusAfter(3 * time.Second)
}

func (m Model) contentHeight() int {
	// Total height minus header (3 lines), footer (3 lines), and borders (2 lines)
	return m.height - 8
}

func (m Model) suggestWidth() int {
	return 25
}

func newViewport(width, height int) viewport.Model {
	vp := viewport.New(width-30, height) // Account for suggestion panel + borders
	vp.SetContent("")
	return vp
}
