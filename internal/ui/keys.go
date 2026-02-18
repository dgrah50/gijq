package ui

import (
	"context"
	"errors"
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
			m.output.Width = m.outputContentWidth()
			m.output.Height = m.contentHeight()
		}
		m.clampOutputXOffset()
		return m, nil

	case resultMsg:
		m.telemetry.OnResult(msg.seq, msg.result.Error, msg.seq == m.activeQuerySeq)
		if msg.seq != m.activeQuerySeq {
			return m, nil
		}
		m.queryRunning = false
		m.queryCancel = nil
		if errors.Is(msg.result.Error, context.Canceled) {
			return m, nil
		}
		m.result = msg.result
		if msg.result.Error != nil {
			m.lines = strings.Split(msg.result.Error.Error(), "\n")
		} else {
			m.lines = strings.Split(msg.result.Raw, "\n")
		}
		m.maxLineWidth = maxDisplayLineWidth(m.lines)
		m.clampOutputXOffset()
		if m.ready {
			// Set raw content for viewport scrolling calculation
			if msg.result.Error != nil {
				m.output.SetContent(msg.result.Error.Error())
			} else {
				m.output.SetContent(msg.result.Raw)
			}
		}
		return m, nil

	case executeQueryMsg:
		if msg.seq != m.querySeq {
			m.telemetry.OnDebounceDropped(msg.seq)
			return m, nil
		}
		return m, m.startExecute(msg.seq)

	case keysMsg:
		if m.keysInFlight == msg.path {
			m.keysInFlight = ""
		}
		if msg.path != m.currentPath() {
			return m, nil
		}
		m.keysPath = msg.path
		if msg.err != nil {
			m.availableKeys = nil
			return m, nil
		}
		m.availableKeys = msg.keys
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

	case "?", "ctrl+/":
		if m.mode == ModeHelp {
			m.mode = ModeNormal
		} else {
			m.mode = ModeHelp
		}
		return m, nil

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
	case ModeHelp:
		return m.handleHelpKey(msg)
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

	case "shift+up":
		m.output.LineUp(8)
		return m, nil

	case "shift+down":
		m.output.LineDown(8)
		return m, nil

	case "shift+left":
		m.scrollHorizontal(-8)
		return m, nil

	case "shift+right":
		m.scrollHorizontal(8)
		return m, nil

	case "home":
		m.outputXOffset = 0
		return m, nil

	case "end":
		m.outputXOffset = m.maxHorizontalOffset()
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
			m.acContext = m.autocomplete.ParseContext(m.filter.Value())
			return m, tea.Batch(m.queueExecute(), m.maybeFetchKeys())
		}
		return m, nil

	case "alt+delete", "alt+d", "ctrl+delete":
		if m.deleteNextWord() {
			m.acContext = m.autocomplete.ParseContext(m.filter.Value())
			return m, tea.Batch(m.queueExecute(), m.maybeFetchKeys())
		}
		return m, nil

	default:
		// Text input
		var cmd tea.Cmd
		m.filter, cmd = m.filter.Update(msg)
		// Update autocomplete context so Available keys panel stays in sync
		m.acContext = m.autocomplete.ParseContext(m.filter.Value())
		return m, tea.Batch(cmd, m.queueExecute(), m.maybeFetchKeys())
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
		m.acContext = m.autocomplete.ParseContext(m.filter.Value())
		return m, tea.Batch(m.executeNow(), m.maybeFetchKeys())

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
		m.acContext = m.autocomplete.ParseContext(m.filter.Value())
		return m, tea.Batch(m.executeNow(), m.maybeFetchKeys())

	default:
		m.mode = ModeNormal
		return m, nil
	}
}

func (m Model) handleHelpKey(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	switch msg.String() {
	case "enter", " ":
		m.mode = ModeNormal
		return m, nil
	default:
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

func (m *Model) maybeFetchKeys() tea.Cmd {
	path := m.currentPath()
	if path == "" {
		path = "."
	}
	if path == m.keysPath || path == m.keysInFlight {
		return nil
	}
	if path != m.keysPath {
		m.availableKeys = nil
	}
	m.keysInFlight = path
	return m.fetchKeys(path)
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
	const minSuggestWidth = 24
	const maxSuggestWidth = 32
	if m.width <= 0 {
		return minSuggestWidth
	}
	w := m.width / 4
	if w < minSuggestWidth {
		w = minSuggestWidth
	}
	if w > maxSuggestWidth {
		w = maxSuggestWidth
	}
	return w
}

func newViewport(width, height int) viewport.Model {
	vp := viewport.New(width-30, height) // Account for suggestion panel + borders
	vp.SetContent("")
	return vp
}

func (m *Model) scrollHorizontal(delta int) {
	m.outputXOffset += delta
	m.clampOutputXOffset()
}

func (m *Model) clampOutputXOffset() {
	if m.outputXOffset < 0 {
		m.outputXOffset = 0
		return
	}
	maxOffset := m.maxHorizontalOffset()
	if m.outputXOffset > maxOffset {
		m.outputXOffset = maxOffset
	}
}

func (m Model) maxHorizontalOffset() int {
	outputWidth := m.outputContentWidth()
	if outputWidth <= 0 {
		return 0
	}
	maxOffset := m.maxLineWidth - outputWidth
	if maxOffset < 0 {
		return 0
	}
	return maxOffset
}

func (m Model) showSuggestionPane() bool {
	// Keep the right pane from compressing the output pane below a usable width.
	const minOutputWidth = 40
	return m.width >= minOutputWidth+m.suggestWidth()+5
}

func (m Model) outputContentWidth() int {
	gutter := 5
	width := m.width - gutter
	if m.showSuggestionPane() {
		width -= m.suggestWidth()
	}
	if width < 10 {
		width = 10
	}
	return width
}
