package ui

import (
	"fmt"
	"strings"
	"sync"

	"charm.land/lipgloss/v2"
	"github.com/charmbracelet/x/ansi"
)

// Raw ANSI color codes for JSON syntax highlighting
// Using raw codes instead of lipgloss to preserve colors through the rendering pipeline
const (
	ansiReset   = "\x1b[0m"
	ansiCyan    = "\x1b[36m" // Keys
	ansiGreen   = "\x1b[32m" // Strings
	ansiYellow  = "\x1b[33m" // Numbers
	ansiMagenta = "\x1b[35m" // Booleans
	ansiGray    = "\x1b[90m" // Null
	ansiWhite   = "\x1b[37m" // Brackets
)

// renderView renders the full UI
func (m Model) renderView() string {
	header := m.renderHeader()
	if m.mode == ModeHelp {
		return header + "\n" + m.renderHelpContent() + "\n" + m.renderFooter()
	}

	view := header + "\n" + m.renderContent() + "\n" + m.renderFooter()

	// Overlay history if in history mode
	if m.mode == ModeHistory {
		view = m.overlayHistory(view)
	}

	return view
}

func (m Model) renderHeader() string {
	title := titleStyle.Render("gijq")
	helpText := m.helpLine
	if helpText == "" {
		helpText = m.compactHelpText()
	}
	help := helpStyle.Render(helpText)

	var status string
	if m.status != "" {
		status = statusStyle.Render(m.fitStatus(m.status))
	} else if m.queryRunning {
		status = helpStyle.Render("Running...")
	} else if m.result.Error != nil {
		status = errorStyle.Render(m.fitStatus("Error: " + m.result.Error.Error()))
	}

	return fmt.Sprintf("%s  %s\n%s\n", title, help, status)
}

var (
	errWrapOnce   sync.Once
	errPre, errSu string
)

// errorWrap returns the escape prefix/suffix errorStyle puts around text.
func errorWrap() (string, string) {
	errWrapOnce.Do(func() {
		pre, suf, _ := strings.Cut(errorStyle.Render("x"), "x")
		errPre, errSu = pre, suf
	})
	return errPre, errSu
}

// borderGlyphStyle colours border glyphs the same way borderStyle does.
var borderGlyphStyle = lipgloss.NewStyle().Foreground(lipgloss.Color("8"))

// bar is the pre-rendered vertical border glyph.
var bar = borderGlyphStyle.Render("│")

type borderKey struct {
	bottom bool
	w      int
}

var (
	borderMu   sync.Mutex
	borderMemo = map[borderKey]string{}
)

const borderMemoLimit = 64

func borderLine(bottom bool, w int) string {
	k := borderKey{bottom, w}
	borderMu.Lock()
	defer borderMu.Unlock()
	if s, ok := borderMemo[k]; ok {
		return s
	}
	l, r := "╭", "╮"
	if bottom {
		l, r = "╰", "╯"
	}
	s := borderGlyphStyle.Render(l + strings.Repeat("─", w) + r)
	if len(borderMemo) >= borderMemoLimit {
		clear(borderMemo)
	}
	borderMemo[k] = s
	return s
}

func borderTop(w int) string    { return borderLine(false, w) }
func borderBottom(w int) string { return borderLine(true, w) }

// fitStatus reduces text to its first line, truncated to the terminal width.
func (m Model) fitStatus(text string) string {
	if i := strings.IndexByte(text, '\n'); i >= 0 {
		text = text[:i]
	}
	if m.width > 0 {
		text = ansi.Truncate(text, m.width, "…")
	}
	return text
}

func (m Model) renderContent() string {
	outputWidth := m.outputContentWidth()
	height := m.contentHeight()
	if height < 0 {
		height = 0
	}

	lines := m.lines
	if len(lines) == 0 {
		lines = []string{""}
	}

	startLine := m.yOffset
	if startLine > len(lines) {
		startLine = len(lines)
	}
	endLine := startLine + height
	if endLine > len(lines) {
		endLine = len(lines)
	}
	visibleLines := lines[startLine:endLine]

	// Render each output row, already padded to outputWidth.
	rows := make([]string, 0, height)
	errPre, errSu := errorWrap()
	isErr := m.result.Error != nil
	var sb strings.Builder
	for i, rawLine := range visibleLines {
		text, w, _, _ := highlightWindow(rawLine, m.outputXOffset, outputWidth, !isErr, m.hl.forLine(startLine+i, rawLine))

		sb.Reset()
		if isErr && text != "" {
			sb.WriteString(errPre)
			sb.WriteString(text)
			sb.WriteString(errSu)
		} else {
			sb.WriteString(text)
		}
		if w < outputWidth {
			writeSpaces(&sb, outputWidth-w)
		}
		rows = append(rows, sb.String())
	}
	emptyLine := strings.Repeat(" ", outputWidth)
	for len(rows) < height {
		rows = append(rows, emptyLine)
	}

	showSuggest := m.showSuggestionPane()
	var suggestRows []string
	suggestWidth := m.suggestWidth()
	if showSuggest {
		suggestRows = m.renderSuggestions()
	}

	out := make([]string, 0, height+2)
	top := borderTop(outputWidth)
	bottom := borderBottom(outputWidth)
	if showSuggest {
		top += " " + borderTop(suggestWidth)
		bottom += " " + borderBottom(suggestWidth)
	}
	out = append(out, top)
	for i := 0; i < height; i++ {
		sb.Reset()
		sb.WriteString(bar)
		sb.WriteString(rows[i])
		sb.WriteString(bar)
		if showSuggest {
			sb.WriteByte(' ')
			sb.WriteString(bar)
			sb.WriteString(fitToWidth(suggestRows, i, suggestWidth))
			sb.WriteString(bar)
		}
		out = append(out, sb.String())
	}
	out = append(out, bottom)
	return strings.Join(out, "\n")
}

func writeSpaces(sb *strings.Builder, n int) {
	const spaces = "                                                                "
	for n > len(spaces) {
		sb.WriteString(spaces)
		n -= len(spaces)
	}
	sb.WriteString(spaces[:n])
}

// fitToWidth returns rows[i] truncated/padded to exactly width columns.
func fitToWidth(rows []string, i, width int) string {
	line := ""
	if i < len(rows) {
		line = rows[i]
	}
	w := ansi.StringWidth(line)
	if w > width {
		line = ansi.Truncate(line, width, "")
		w = ansi.StringWidth(line)
	}
	if w < width {
		line += strings.Repeat(" ", width-w)
	}
	return line
}

func (m Model) renderSuggestions() []string {
	if m.mode == ModeAutocomplete && len(m.suggestions) > 0 {
		var lines []string
		lines = append(lines, labelStyle.Render("Keys:"))
		for i, s := range m.suggestions {
			if i == m.selectedIdx {
				lines = append(lines, selectedStyle.Render("→ "+s))
			} else {
				lines = append(lines, suggestionStyle.Render("  "+s))
			}
			if i >= m.contentHeight()-2 {
				lines = append(lines, helpStyle.Render(fmt.Sprintf("  ...+%d more", len(m.suggestions)-i-1)))
				break
			}
		}
		return lines
	}

	// Show current path keys when not in autocomplete
	if m.keys.loadingFor(m.currentPath()) {
		return []string{labelStyle.Render("Loading keys...")}
	}
	keys := m.keys.filtered(m.acContext.Incomplete)
	if len(keys) == 0 {
		if m.acContext.Incomplete != "" {
			return []string{labelStyle.Render("No matches")}
		}
		return []string{labelStyle.Render("No keys")}
	}

	var lines []string
	if m.acContext.Incomplete != "" {
		lines = append(lines, labelStyle.Render("Matching keys:"))
	} else {
		lines = append(lines, labelStyle.Render("Available keys:"))
	}
	for i, k := range keys {
		lines = append(lines, suggestionStyle.Render("  "+k))
		if i >= m.contentHeight()-2 {
			lines = append(lines, helpStyle.Render(fmt.Sprintf("  ...+%d more", len(keys)-i-1)))
			break
		}
	}
	return lines
}

func (m Model) currentPath() string {
	ctx := m.acContext
	if ctx.Path == "" {
		ctx.Path = "."
	}
	return ctx.Path
}

func (m Model) renderFooter() string {
	filterLabel := labelStyle.Render("filter: ")
	filter := m.filter.View()
	fileLabel := labelStyle.Render("file: ")
	file := m.filename
	scrollLabel := ""
	if m.maxHorizontalOffset() > 0 {
		scrollLabel = labelStyle.Render(fmt.Sprintf(" x:%d/%d", m.outputXOffset, m.maxHorizontalOffset()))
	}

	return fmt.Sprintf("\n%s%s\n%s%s%s", filterLabel, filter, fileLabel, file, scrollLabel)
}

func (m Model) overlayHistory(base string) string {
	if len(m.historyItems) == 0 {
		content := "No history for this file"
		overlay := historyOverlayStyle.Render(content)
		return placeOverlay(base, overlay, m.width, m.height)
	}

	var lines []string
	lines = append(lines, titleStyle.Render("Query History"))
	lines = append(lines, helpStyle.Render("↑/↓: navigate | enter: select | esc: close"))
	lines = append(lines, "")

	maxShow := 10
	for i, item := range m.historyItems {
		if i >= maxShow {
			lines = append(lines, helpStyle.Render(fmt.Sprintf("...+%d more", len(m.historyItems)-maxShow)))
			break
		}
		if i == m.historyIdx {
			lines = append(lines, selectedStyle.Render("→ "+item))
		} else {
			lines = append(lines, suggestionStyle.Render("  "+item))
		}
	}

	content := strings.Join(lines, "\n")
	overlay := historyOverlayStyle.Render(content)
	return placeOverlay(base, overlay, m.width, m.height)
}

func (m Model) compactHelpText() string {
	items := []string{
		"f1: help",
		"tab: autocomplete",
		"enter: quit",
		"shift+up/down: fast scroll",
		"shift+left/right: h-scroll",
		"ctrl+h: history",
		"ctrl+y: copy out",
	}

	if m.width <= 0 {
		return items[0]
	}

	const reserveForTitle = 28
	maxWidth := m.width - reserveForTitle
	if maxWidth < 12 {
		return items[0]
	}

	sep := " | "
	line := ""
	for _, item := range items {
		candidate := item
		if line != "" {
			candidate = line + sep + item
		}
		if lipgloss.Width(candidate) > maxWidth {
			break
		}
		line = candidate
	}

	if line == "" {
		return items[0]
	}
	return line
}

func (m Model) renderHelpContent() string {
	maxWidth := m.width - 8
	if maxWidth < 44 {
		maxWidth = m.width - 2
	}
	if maxWidth > 88 {
		maxWidth = 88
	}
	if maxWidth < 24 {
		maxWidth = 24
	}

	rows := []string{
		titleStyle.Render("Keyboard Shortcuts"),
		helpStyle.Render("esc, ? or F1: close"),
		"",
		labelStyle.Render("Navigation"),
		m.helpRow("Up/Down", "Scroll output"),
		m.helpRow("Shift+Up/Down", "Fast scroll"),
		m.helpRow("PgUp/PgDn", "Half-page scroll"),
		m.helpRow("Shift+Left/Right", "Horizontal scroll"),
		m.helpRow("Home/End", "Jump horizontal start/end"),
		"",
		labelStyle.Render("Editing"),
		m.helpRow("Tab", "Autocomplete keys"),
		m.helpRow("Alt/Ctrl+Left", "Prev word"),
		m.helpRow("Alt/Ctrl+Right", "Next word"),
		m.helpRow("Alt/Ctrl+Backspace", "Delete prev word"),
		"",
		labelStyle.Render("Actions"),
		m.helpRow("Enter", "Output result and quit"),
		m.helpRow("Ctrl+Y", "Copy output"),
		m.helpRow("Ctrl+F", "Copy filter"),
		m.helpRow("Ctrl+H", "Query history"),
		m.helpRow("F1 / Ctrl+/", "Toggle this help"),
		m.helpRow("Esc/Ctrl+C", "Quit"),
	}

	panel := historyOverlayStyle.Width(maxWidth).Render(strings.Join(rows, "\n"))
	return lipgloss.Place(m.width, m.contentHeight(), lipgloss.Center, lipgloss.Center, panel)
}

func (m Model) helpRow(key, desc string) string {
	return suggestionStyle.Render(fmt.Sprintf("  %-18s %s", key, desc))
}

func placeOverlay(base, overlay string, width, height int) string {
	// Center the overlay
	overlayLines := strings.Split(overlay, "\n")
	baseLines := strings.Split(base, "\n")

	overlayHeight := len(overlayLines)
	overlayWidth := 0
	for _, l := range overlayLines {
		if w := ansi.StringWidth(l); w > overlayWidth {
			overlayWidth = w
		}
	}

	startY := (height - overlayHeight) / 2
	startX := (width - overlayWidth) / 2

	if startY < 0 {
		startY = 0
	}
	if startX < 0 {
		startX = 0
	}

	for i, line := range overlayLines {
		y := startY + i
		if y >= len(baseLines) {
			break
		}
		baseLine := baseLines[y]
		left := ansi.Truncate(baseLine, startX, "")
		leftW := ansi.StringWidth(left)
		if leftW < startX {
			left += strings.Repeat(" ", startX-leftW)
		}
		lineW := ansi.StringWidth(line)
		right := ansi.TruncateLeft(baseLine, startX+lineW, "")

		var sb strings.Builder
		if strings.Contains(left, "\x1b") {
			left += ansiReset
		}
		sb.WriteString(left)
		sb.WriteString(line)
		if right != "" {
			sb.WriteString(ansiReset)
			sb.WriteString(right)
		}
		baseLines[y] = sb.String()
	}

	return strings.Join(baseLines, "\n")
}
