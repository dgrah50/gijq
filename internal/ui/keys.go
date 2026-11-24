package ui

import (
	"context"
	"errors"
	"strings"
	"unicode"

	tea "charm.land/bubbletea/v2"

	"github.com/dayangraham/gijq/internal/jq"
	"github.com/dayangraham/gijq/internal/jsonview"
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
		return m.handleResult(msg)

	case resultProgressMsg:
		// Stale or finished queries are dropped and not re-armed.
		if !m.query.live(msg.seq) || m.query.phase != queryStreaming {
			return m, nil
		}
		return m.applySnapshot(msg.seq, msg.collector, msg.snap)

	case indexMsg:
		return m.handleIndex(msg)

	case keysMsg:
		m.keys.receive(msg.path, msg.keys, msg.err, msg.seq)
		return m, nil

	case statusClearMsg:
		if msg.seq == m.statusSeq {
			m.status = ""
		}
		return m, nil
	}

	return m, nil
}

// handleResult applies the first message of a query: it replaces the shown
// result. Messages of superseded queries are dropped.
func (m Model) handleResult(msg resultMsg) (Model, tea.Cmd) {
	accepted := m.query.live(msg.seq)
	err := msg.err
	if msg.snap != nil {
		err = msg.snap.Err
	}
	m.telemetry.OnResult(msg.seq, err, accepted)
	if !accepted {
		return m, nil
	}
	if msg.err != nil {
		m.result.begin(msg.filter)
		m.result.setError(msg.err)
		m.query.stop()
		m.clampOutputXOffset()
		m.clampYOffset()
		return m, nil
	}
	m.result.begin(msg.filter)
	return m.applySnapshot(msg.seq, msg.collector, msg.snap)
}

// applySnapshot adopts a collector snapshot of the live query and decides
// what comes next: wait for more values (streaming), build the index
// (collected), or stop (failed).
func (m Model) applySnapshot(seq int, c *jsonview.Collector, snap *jsonview.Snapshot) (Model, tea.Cmd) {
	var cmd tea.Cmd
	switch {
	case errors.Is(snap.Err, context.Canceled):
		// Only cancellation produces this, and cancellation goes through
		// query.stop, which makes the query not live. Be defensive anyway.
		m.query.stop()
		return m, nil
	case snap.Err != nil:
		m.result.setError(snap.Err)
		m.query.stop()
	case !snap.Done:
		m.result.setSnapshot(snap, m.firstScreenLines())
		cmd = waitProgress(m.query.ctx, seq, c)
	default:
		m.result.setSnapshot(snap, m.firstScreenLines())
		m.query.collected()
		cmd = startIndex(m.query.ctx, seq, snap.Doc)
	}
	m.clampOutputXOffset()
	m.clampYOffset()
	return m, cmd
}

// handleIndex applies an index snapshot of the document being shown.
func (m Model) handleIndex(msg indexMsg) (Model, tea.Cmd) {
	if !m.query.live(msg.seq) || m.query.phase != queryIndexing ||
		m.result.snap == nil || msg.doc != m.result.snap.Doc {
		return m, nil
	}
	if msg.index == nil { // builder ended without a result
		m.query.stop()
		return m, nil
	}
	m.result.setIndex(msg.index)
	var cmd tea.Cmd
	if msg.index.Complete() {
		m.query.stop() // indexing done: back to idle
	} else {
		cmd = waitIndex(m.query.ctx, msg.seq, msg.doc, msg.ch)
	}
	m.clampOutputXOffset()
	m.clampYOffset()
	return m, cmd
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
	m.query.stop() // no query goroutine outlives the program
	m.keys.stop()
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
		return m.copyOutput()

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

// currentDoc returns the complete result of the filter on screen. It uses
// the shown result when it is this filter's and fully collected; otherwise
// (stale filter, or still streaming) it evaluates the filter to completion on
// the calling goroutine, leaving the background query untouched. ok is false
// when the filter's result is an error.
func (m Model) currentDoc() (doc *jsonview.Doc, ok bool) {
	filter := m.filter.Value()
	if m.result.filter == filter {
		if m.result.hasError() {
			return nil, false
		}
		if m.result.complete() {
			return m.result.snap.Doc, true
		}
	}
	d, err := evalDoc(m.jq, filter)
	return d, err == nil
}

// evalDoc evaluates filter to completion on the calling goroutine. Any error
// value fails the whole result, as in the shown result.
func evalDoc(svc *jq.Service, filter string) (*jsonview.Doc, error) {
	iter, err := svc.Iter(context.Background(), filter)
	if err != nil {
		return nil, err
	}
	var values []any
	for {
		v, ok := iter.Next()
		if !ok {
			return jsonview.NewDoc(values), nil
		}
		if err, isErr := v.(error); isErr {
			return nil, err
		}
		values = append(values, v)
	}
}

// confirm records the result of the current filter for printing after exit
// and quits. The shown result may belong to an older filter, or still be
// streaming; in that case the current filter is evaluated now so the printed
// output always matches the filter on screen and is complete.
func (m Model) confirm() (Model, tea.Cmd) {
	m.query.stop() // exiting: nothing in the background matters any more
	if doc, ok := m.currentDoc(); ok && doc.NumValues() > 0 {
		m.output = doc
		m.history.Add(m.filepath, m.filter.Value())
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

// resultText builds the full text of the current filter's result for the
// clipboard, on demand from the complete document. When there is nothing to
// copy it returns the status message explaining why instead.
func (m Model) resultText() (text, problem string) {
	doc, ok := m.currentDoc()
	if !ok || doc.NumValues() == 0 {
		return "", "Nothing to copy"
	}
	var sb strings.Builder
	if _, err := doc.WriteTo(&sb); err != nil {
		return "", "Copy failed: " + err.Error()
	}
	return sb.String(), ""
}

// copyOutput copies the full result text of the current filter.
func (m Model) copyOutput() (Model, tea.Cmd) {
	text, problem := m.resultText()
	if problem != "" {
		return m, m.setStatus(problem)
	}
	return m.copyToClipboard(text, "output")
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
