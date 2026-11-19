package ui

import (
	"context"
	"errors"
	"strings"
	"unicode"

	tea "charm.land/bubbletea/v2"
)

// Update handles messages
func (m Model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.KeyPressMsg:
		return m.handleKey(msg)

	case tea.WindowSizeMsg:
		m.width = msg.Width
		m.height = msg.Height
		m.ready = true
		// Keep the footer filter line within the terminal width
		// ("filter: " label + "> " prompt + cursor cell).
		if w := m.width - len("filter: ") - 4; w >= 1 {
			m.filter.SetWidth(w)
		} else {
			m.filter.SetWidth(1)
		}
		m.helpLine = m.compactHelpText()
		m.clampOutputXOffset()
		m.clampYOffset()
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
		m.lines = msg.lines
		if len(m.lines) == 0 {
			m.lines = []string{""}
		}
		m.maxLineWidth = msg.maxLineWidth
		m.hl.resetWith(msg.checkpoints)
		m.clampOutputXOffset()
		m.clampYOffset()
		return m, nil

	case executeQueryMsg:
		if msg.seq != m.querySeq {
			m.telemetry.OnDebounceDropped(msg.seq)
			return m, nil
		}
		return m, m.startExecute(msg.seq)

	case keysMsg:
		m.keys.receive(msg.path, msg.keys, msg.err)
		return m, nil

	case statusClearMsg:
		if msg.seq == m.statusSeq {
			m.status = ""
		}
		return m, nil
	}

	return m, nil
}

// syncContext re-derives the autocomplete context from the filter and
// fetches keys for the resulting path if needed.
func (m *Model) syncContext() tea.Cmd {
	m.acContext = m.autocomplete.ParseContext(m.filter.Value())
	return m.maybeFetchKeys()
}

// leaveToNormal exits any non-normal mode, drops mode data and re-syncs the
// keys pane with the filter.
func (m *Model) leaveToNormal() tea.Cmd {
	m.mode = ModeNormal
	m.suggestions = nil
	m.selectedIdx = 0
	return m.syncContext()
}

// enterMode switches to help or history, discarding autocomplete data.
func (m *Model) enterMode(mode Mode) {
	m.mode = mode
	m.suggestions = nil
	m.selectedIdx = 0
}

func (m Model) handleKey(msg tea.KeyPressMsg) (tea.Model, tea.Cmd) {
	key := msg.String()

	// Global keys
	switch key {
	case "ctrl+c":
		return m, tea.Quit

	case "f1", "ctrl+/", "ctrl+_":
		if m.mode == ModeHelp {
			return m, m.leaveToNormal()
		}
		m.enterMode(ModeHelp)
		return m, nil

	case "esc":
		if m.mode != ModeNormal {
			return m, m.leaveToNormal()
		}
		return m, tea.Quit

	case "ctrl+y":
		return m.copyOutput()

	case "ctrl+f":
		return m.copyFilter()

	case "ctrl+h":
		m.enterMode(ModeHistory)
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

func (m Model) handleNormalKey(msg tea.KeyPressMsg) (tea.Model, tea.Cmd) {
	key := msg.String()

	switch key {
	case "enter":
		// Record the result and quit; main prints it after the TUI has
		// released the terminal.
		if m.result.Error == nil && m.result.Raw != "" {
			m.output = m.result.Raw
			m.confirmed = true
			// Save to history
			m.history.Add(m.filepath, m.filter.Value())
			m.history.Save()
		}
		return m, tea.Quit

	case "tab":
		m.mode = ModeAutocomplete
		original := m.filter.Value()
		filter := original

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

		if m.filter.Value() != original {
			return m, tea.Batch(m.queueExecute(), m.maybeFetchKeys())
		}
		return m, nil

	case "up":
		m.scrollY(-1)
		return m, nil

	case "down":
		m.scrollY(1)
		return m, nil

	case "shift+up":
		m.scrollY(-8)
		return m, nil

	case "shift+down":
		m.scrollY(8)
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
		m.scrollY(-m.halfPage())
		return m, nil

	case "pgdown":
		m.scrollY(m.halfPage())
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

func (m Model) handleAutocompleteKey(msg tea.KeyPressMsg) (tea.Model, tea.Cmd) {
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
		fetch := m.leaveToNormal()
		return m, tea.Batch(m.executeNow(), fetch)

	default:
		// Any other key exits autocomplete and processes normally
		m.mode = ModeNormal
		m.suggestions = nil
		m.selectedIdx = 0
		return m.handleNormalKey(msg)
	}
}

func (m Model) handleHistoryKey(msg tea.KeyPressMsg) (tea.Model, tea.Cmd) {
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
		fetch := m.leaveToNormal()
		return m, tea.Batch(m.executeNow(), fetch)

	default:
		// Leave history and process the key as a normal key.
		m.mode = ModeNormal
		return m.handleNormalKey(msg)
	}
}

func (m Model) handleHelpKey(msg tea.KeyPressMsg) (tea.Model, tea.Cmd) {
	switch msg.String() {
	case "?", "enter", "space":
		return m, m.leaveToNormal()
	default:
		return m, nil
	}
}

func (m Model) copyOutput() (tea.Model, tea.Cmd) {
	if m.result.Raw == "" {
		return m, m.setStatus("Nothing to copy")
	}
	if err := m.clipboard.Copy(m.result.Raw); err != nil {
		return m, m.setStatus("Copy failed: " + err.Error())
	}
	return m, m.setStatus("Copied output to clipboard")
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
	if !m.keys.request(path) {
		return nil
	}
	return m.fetchKeys(path)
}

func (m Model) copyFilter() (tea.Model, tea.Cmd) {
	filter := m.filter.Value()
	if filter == "" {
		return m, m.setStatus("Nothing to copy")
	}
	if err := m.clipboard.Copy(filter); err != nil {
		return m, m.setStatus("Copy failed: " + err.Error())
	}
	return m, m.setStatus("Copied filter to clipboard")
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

func (m Model) halfPage() int {
	if h := m.contentHeight() / 2; h > 1 {
		return h
	}
	return 1
}

func (m Model) maxYOffset() int {
	n := len(m.lines) - m.contentHeight()
	if n < 0 {
		return 0
	}
	return n
}

func (m *Model) clampYOffset() {
	if max := m.maxYOffset(); m.yOffset > max {
		m.yOffset = max
	}
	if m.yOffset < 0 {
		m.yOffset = 0
	}
}

func (m *Model) scrollY(delta int) {
	m.yOffset += delta
	m.clampYOffset()
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
