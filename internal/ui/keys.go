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
		m.filter.SetWidth(max(1, m.width-len("filter: ")-4))
		m.helpLine = m.compactHelpText()
		m.clampOutputXOffset()
		m.clampYOffset()
		return m, nil

	case executeQueryMsg:
		if !m.query.isLatest(msg.seq) {
			m.telemetry.OnDebounceDropped(msg.seq) // superseded while debouncing
			return m, nil
		}
		return m, m.startExecute(msg.seq)

	case resultMsg:
		accepted := m.query.finish(msg.seq)
		m.telemetry.OnResult(msg.seq, msg.result.Error, accepted)
		if !accepted || errors.Is(msg.result.Error, context.Canceled) {
			return m, nil
		}
		m.result = msg.result
		m.resultFilter = msg.filter
		m.lines = msg.lines
		m.maxLineWidth = msg.maxLineWidth
		m.checkpoints = msg.checkpoints
		m.clampOutputXOffset()
		m.clampYOffset()
		return m, nil

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

// handleKey dispatches a key press, then applies the follow-up every filter
// edit needs. Handlers only edit state; they never queue queries or keys
// fetches themselves, so no edit path can forget to.
func (m Model) handleKey(msg tea.KeyPressMsg) (tea.Model, tea.Cmd) {
	before := m.filter.Value()
	m, cmd := m.dispatchKey(msg)
	if m.quitting {
		return m, cmd
	}
	var queued tea.Cmd
	if m.filter.Value() != before {
		queued = m.queueExecute()
	}
	return m, tea.Batch(cmd, queued, m.fetchKeysIfNeeded())
}

// quit moves the model to its terminal state and returns the quit command.
func (m *Model) quit() tea.Cmd {
	m.quitting = true
	return tea.Quit
}

func (m Model) dispatchKey(msg tea.KeyPressMsg) (Model, tea.Cmd) {
	// Global keys
	switch msg.String() {
	case "ctrl+c":
		return m, m.quit()

	case "f1", "ctrl+/", "ctrl+_":
		if m.mode == ModeHelp {
			m.setMode(ModeNormal)
		} else {
			m.setMode(ModeHelp)
		}
		return m, nil

	case "esc":
		if m.mode == ModeNormal {
			return m, m.quit()
		}
		m.setMode(ModeNormal)
		return m, nil

	case "ctrl+y":
		return m.copyToClipboard(m.result.Raw, "output")

	case "ctrl+f":
		return m.copyToClipboard(m.filter.Value(), "filter")

	case "ctrl+h":
		m.setMode(ModeHistory)
		m.historyItems = m.history.Get(m.filepath)
		return m, nil
	}

	switch m.mode {
	case ModeAutocomplete:
		return m.handleAutocompleteKey(msg)
	case ModeHistory:
		return m.handleHistoryKey(msg)
	case ModeHelp:
		return m.handleHelpKey(msg)
	default:
		return m.handleNormalKey(msg)
	}
}

func (m Model) handleNormalKey(msg tea.KeyPressMsg) (Model, tea.Cmd) {
	switch msg.String() {
	case "enter":
		return m.confirm()

	case "tab":
		return m.startAutocomplete(), nil

	case "up":
		m.scrollY(-1)
	case "down":
		m.scrollY(1)
	case "shift+up":
		m.scrollY(-8)
	case "shift+down":
		m.scrollY(8)
	case "pgup":
		m.scrollY(-m.halfPage())
	case "pgdown":
		m.scrollY(m.halfPage())
	case "shift+left":
		m.scrollHorizontal(-8)
	case "shift+right":
		m.scrollHorizontal(8)
	case "home":
		m.outputXOffset = 0
	case "end":
		m.outputXOffset = m.maxHorizontalOffset()

	case "alt+left", "alt+b", "ctrl+left":
		m.filter.SetCursor(prevWordStart(m.filter.Value(), m.filter.Position()))
	case "alt+right", "alt+f", "ctrl+right":
		m.filter.SetCursor(nextWordStart(m.filter.Value(), m.filter.Position()))
	case "alt+backspace", "ctrl+backspace", "ctrl+w":
		m.deletePrevWord()
	case "alt+delete", "alt+d", "ctrl+delete":
		m.deleteNextWord()

	default:
		var cmd tea.Cmd
		m.filter, cmd = m.filter.Update(msg)
		return m, cmd
	}
	return m, nil
}

// confirm records the result of the current filter for printing after exit
// and quits. The shown result may belong to an older filter (query still
// debouncing or running); in that case the current filter is evaluated now so
// the printed output always matches the filter on screen.
func (m Model) confirm() (Model, tea.Cmd) {
	filter := m.filter.Value()
	result := m.result
	if m.resultFilter != filter {
		m.query.stop()
		result = m.jq.Execute(filter)
	}
	if result.Error == nil && result.Raw != "" {
		m.output = result.Raw
		m.history.Add(m.filepath, filter)
		_ = m.history.Save() // best effort: never block exiting on history
	}
	return m, m.quit()
}

// startAutocomplete completes the key under the cursor. A filter ending in
// "]" drills into the element; a single exact match is accepted and drilled
// into. With suggestions available it enters ModeAutocomplete.
func (m Model) startAutocomplete() Model {
	filter := m.filter.Value()
	if strings.HasSuffix(filter, "]") {
		filter += "."
	}
	suggestions, ctx := m.autocomplete.Suggest(filter)
	if len(suggestions) == 1 && ctx.Incomplete != "" && suggestions[0] == ctx.Incomplete {
		filter = filter[:ctx.StartPos] + suggestions[0] + "."
		suggestions, _ = m.autocomplete.Suggest(filter)
	}
	if filter != m.filter.Value() {
		m.setFilter(filter)
	}
	if len(suggestions) > 0 {
		m.setMode(ModeAutocomplete)
		m.suggestions = suggestions
	}
	return m
}

func (m Model) handleAutocompleteKey(msg tea.KeyPressMsg) (Model, tea.Cmd) {
	switch msg.String() {
	case "tab", "down":
		m.selectedIdx = wrapIndex(m.selectedIdx+1, len(m.suggestions))
		return m, nil

	case "up", "shift+tab":
		m.selectedIdx = wrapIndex(m.selectedIdx-1, len(m.suggestions))
		return m, nil

	case "enter":
		if len(m.suggestions) > 0 {
			selected := m.suggestions[m.selectedIdx]
			m.setFilter(m.autocomplete.Apply(m.filter.Value(), m.acContext(), selected))
		}
		m.setMode(ModeNormal)
		return m, nil

	default:
		// Any other key leaves autocomplete and is handled normally.
		m.setMode(ModeNormal)
		return m.handleNormalKey(msg)
	}
}

func (m Model) handleHistoryKey(msg tea.KeyPressMsg) (Model, tea.Cmd) {
	switch msg.String() {
	case "down", "tab":
		m.historyIdx = wrapIndex(m.historyIdx+1, len(m.historyItems))
		return m, nil

	case "up", "shift+tab":
		m.historyIdx = wrapIndex(m.historyIdx-1, len(m.historyItems))
		return m, nil

	case "enter":
		if len(m.historyItems) > 0 {
			m.setFilter(m.historyItems[m.historyIdx])
		}
		m.setMode(ModeNormal)
		return m, nil

	default:
		// Any other key leaves history and is handled normally.
		m.setMode(ModeNormal)
		return m.handleNormalKey(msg)
	}
}

func (m Model) handleHelpKey(msg tea.KeyPressMsg) (Model, tea.Cmd) {
	switch msg.String() {
	case "?", "enter", "space":
		m.setMode(ModeNormal)
	}
	return m, nil
}

// wrapIndex wraps i into [0, n); it returns 0 when n is 0.
func wrapIndex(i, n int) int {
	if n <= 0 {
		return 0
	}
	return ((i % n) + n) % n
}

func (m Model) copyToClipboard(text, what string) (Model, tea.Cmd) {
	if text == "" {
		return m, m.setStatus("Nothing to copy")
	}
	if err := m.clipboard.Copy(text); err != nil {
		return m, m.setStatus("Copy failed: " + err.Error())
	}
	return m, m.setStatus("Copied " + what + " to clipboard")
}

// deletePrevWord deletes from the start of the previous word to the cursor.
func (m *Model) deletePrevWord() {
	value, pos := m.filter.Value(), m.filter.Position()
	start := prevWordStart(value, pos)
	if start == pos {
		return
	}
	r := []rune(value)
	m.filter.SetValue(string(r[:start]) + string(r[pos:]))
	m.filter.SetCursor(start)
}

// deleteNextWord deletes from the cursor to the end of the next word.
func (m *Model) deleteNextWord() {
	value, pos := m.filter.Value(), m.filter.Position()
	end := nextWordDeleteEnd(value, pos)
	if end == pos {
		return
	}
	r := []rune(value)
	m.filter.SetValue(string(r[:pos]) + string(r[end:]))
	m.filter.SetCursor(pos)
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
