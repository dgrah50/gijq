package ui

import (
	"fmt"
	"regexp"
	"strings"

	"github.com/charmbracelet/lipgloss"
	"github.com/mattn/go-runewidth"
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

var (
	jsonStringRe = regexp.MustCompile(`"(?:[^"\\]|\\.)*"`)
	jsonNumberRe = regexp.MustCompile(`:\s*(-?\d+(?:\.\d+)?(?:[eE][+-]?\d+)?)`)
	jsonBoolRe   = regexp.MustCompile(`:\s*(true|false)`)
	jsonNullRe   = regexp.MustCompile(`:\s*(null)`)
	jsonKeyRe    = regexp.MustCompile(`"([^"]+)"(\s*:)`)
)

// renderView renders the full UI
func (m Model) renderView() string {
	header := m.renderHeader()
	if m.mode == ModeHelp {
		content := m.renderHelpContent()
		footer := m.renderFooter()
		return lipgloss.JoinVertical(lipgloss.Left, header, content, footer)
	}

	content := m.renderContent()
	footer := m.renderFooter()

	view := lipgloss.JoinVertical(lipgloss.Left, header, content, footer)

	// Overlay history if in history mode
	if m.mode == ModeHistory {
		view = m.overlayHistory(view)
	}

	return view
}

func (m Model) renderHeader() string {
	title := titleStyle.Render("gijq")
	help := helpStyle.Render(m.compactHelpText())

	var status string
	if m.status != "" {
		status = statusStyle.Render(m.status)
	} else if m.queryRunning {
		status = helpStyle.Render("Running...")
	} else if m.result.Error != nil {
		status = errorStyle.Render("Error: " + m.result.Error.Error())
	}

	return fmt.Sprintf("%s  %s\n%s\n", title, help, status)
}

func (m Model) renderContent() string {
	outputWidth := m.outputContentWidth()

	lines := m.lines
	if len(lines) == 0 {
		lines = []string{""}
	}

	startLine := m.output.YOffset
	endLine := startLine + m.contentHeight()
	if endLine > len(lines) {
		endLine = len(lines)
	}
	if startLine > len(lines) {
		startLine = len(lines)
	}

	visibleLines := lines[startLine:endLine]

	// Manually pad each line to width (preserves ANSI codes)
	var paddedLines []string
	for _, rawLine := range visibleLines {
		clippedRaw, leftCut, rightCut := clipRawLine(rawLine, m.outputXOffset, outputWidth)
		clippedRaw = withEllipsis(clippedRaw, outputWidth, leftCut, rightCut)

		line := rawLine
		if m.result.Error != nil {
			line = errorStyle.Render(clippedRaw)
		} else {
			line = m.colorCache.Colorize(clippedRaw)
		}

		displayWidth := lipgloss.Width(line)
		if displayWidth < outputWidth {
			line += strings.Repeat(" ", outputWidth-displayWidth)
		}
		paddedLines = append(paddedLines, line)
	}

	// Pad to fill height
	emptyLine := strings.Repeat(" ", outputWidth)
	for len(paddedLines) < m.contentHeight() {
		paddedLines = append(paddedLines, emptyLine)
	}

	visibleContent := strings.Join(paddedLines, "\n")

	// Only use borderStyle for border chrome, not Width/Height constraints
	outputPane := borderStyle.Width(outputWidth).Height(m.contentHeight()).Render(visibleContent)

	if !m.showSuggestionPane() {
		return outputPane
	}

	suggestPane := m.renderSuggestions()
	return lipgloss.JoinHorizontal(lipgloss.Top,
		outputPane,
		" ",
		borderStyle.Width(m.suggestWidth()).Height(m.contentHeight()).Render(suggestPane),
	)
}

// colorizeJSON applies syntax highlighting to JSON using raw ANSI codes
func colorizeJSON(s string) string {
	// Color brackets/braces FIRST in a single pass (individual ReplaceAll calls
	// would corrupt each other since ANSI codes contain [ and ] characters)
	bracketReplacer := strings.NewReplacer(
		"{", ansiWhite+"{"+ansiReset,
		"}", ansiWhite+"}"+ansiReset,
		"[", ansiWhite+"["+ansiReset,
		"]", ansiWhite+"]"+ansiReset,
	)
	result := bracketReplacer.Replace(s)

	// Color keys (ANSI codes go inside quotes so the string regex detects and skips them)
	result = jsonKeyRe.ReplaceAllStringFunc(result, func(match string) string {
		parts := jsonKeyRe.FindStringSubmatch(match)
		if len(parts) >= 3 {
			return "\"" + ansiCyan + parts[1] + ansiReset + "\"" + parts[2]
		}
		return match
	})

	// Color string values (not keys - they're already colored)
	result = jsonStringRe.ReplaceAllStringFunc(result, func(match string) string {
		if strings.Contains(match, "\x1b[") {
			return match
		}
		return ansiGreen + match + ansiReset
	})

	// Color numbers (values after colon)
	result = jsonNumberRe.ReplaceAllStringFunc(result, func(match string) string {
		parts := jsonNumberRe.FindStringSubmatch(match)
		if len(parts) >= 2 {
			return ": " + ansiYellow + parts[1] + ansiReset
		}
		return match
	})

	// Color booleans
	result = jsonBoolRe.ReplaceAllStringFunc(result, func(match string) string {
		parts := jsonBoolRe.FindStringSubmatch(match)
		if len(parts) >= 2 {
			return ": " + ansiMagenta + parts[1] + ansiReset
		}
		return match
	})

	// Color null
	result = jsonNullRe.ReplaceAllStringFunc(result, func(match string) string {
		parts := jsonNullRe.FindStringSubmatch(match)
		if len(parts) >= 2 {
			return ": " + ansiGray + parts[1] + ansiReset
		}
		return match
	})

	return result
}

func (m Model) renderSuggestions() string {
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
		return strings.Join(lines, "\n")
	}

	// Show current path keys when not in autocomplete
	allKeys := m.availableKeys
	if m.keysInFlight == m.currentPath() && len(allKeys) == 0 {
		return labelStyle.Render("Loading keys...")
	}
	keys := filterKeysByPrefix(allKeys, m.acContext.Incomplete)
	if len(keys) == 0 {
		if m.acContext.Incomplete != "" {
			return labelStyle.Render("No matches")
		}
		return labelStyle.Render("No keys")
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
	return strings.Join(lines, "\n")
}

func filterKeysByPrefix(keys []string, incomplete string) []string {
	if incomplete == "" || len(keys) == 0 {
		return keys
	}

	prefix := strings.ToLower(incomplete)
	matches := make([]string, 0, len(keys))
	for _, key := range keys {
		if strings.HasPrefix(strings.ToLower(key), prefix) {
			matches = append(matches, key)
		}
	}
	return matches
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
		"?: help",
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
		helpStyle.Render("esc or ?: close"),
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
	overlayWidth := lipgloss.Width(overlay)

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
		// Pad base line if needed
		if len(baseLine) < startX {
			baseLine += strings.Repeat(" ", startX-len(baseLine))
		}
		// Replace portion with overlay
		newLine := baseLine[:startX] + line
		if len(baseLine) > startX+lipgloss.Width(line) {
			newLine += baseLine[startX+lipgloss.Width(line):]
		}
		baseLines[y] = newLine
	}

	return strings.Join(baseLines, "\n")
}

func clipRawLine(line string, xOffset, maxWidth int) (string, bool, bool) {
	if maxWidth <= 0 {
		return "", xOffset > 0, len(line) > 0
	}

	runes := []rune(line)
	if len(runes) == 0 {
		return "", xOffset > 0, false
	}

	if xOffset < 0 {
		xOffset = 0
	}

	totalWidth := 0
	for _, r := range runes {
		w := runewidth.RuneWidth(r)
		if w < 1 {
			w = 1
		}
		totalWidth += w
	}

	if xOffset > totalWidth {
		xOffset = totalWidth
	}

	startIdx := len(runes)
	widthSoFar := 0
	for i, r := range runes {
		w := runewidth.RuneWidth(r)
		if w < 1 {
			w = 1
		}
		if widthSoFar+w > xOffset {
			startIdx = i
			break
		}
		widthSoFar += w
	}

	endIdx := startIdx
	visibleWidth := 0
	for i := startIdx; i < len(runes); i++ {
		w := runewidth.RuneWidth(runes[i])
		if w < 1 {
			w = 1
		}
		if visibleWidth+w > maxWidth {
			break
		}
		visibleWidth += w
		endIdx = i + 1
	}

	leftCut := xOffset > 0
	rightCut := (xOffset + visibleWidth) < totalWidth
	return string(runes[startIdx:endIdx]), leftCut, rightCut
}

func withEllipsis(line string, maxWidth int, leftCut, rightCut bool) string {
	if maxWidth <= 0 {
		return ""
	}

	if !leftCut && !rightCut {
		return line
	}

	runes := []rune(line)
	runes = trimToDisplayWidth(runes, maxWidth)

	if leftCut && rightCut && maxWidth >= 2 {
		runes = trimToDisplayWidth(runes, maxWidth-2)
		return "…" + string(runes) + "…"
	}

	if leftCut {
		if maxWidth == 1 {
			return "…"
		}
		runes = trimToDisplayWidth(runes, maxWidth-1)
		return "…" + string(runes)
	}

	// rightCut only
	if maxWidth == 1 {
		return "…"
	}
	runes = trimToDisplayWidth(runes, maxWidth-1)
	return string(runes) + "…"
}

func trimToDisplayWidth(runes []rune, maxWidth int) []rune {
	if maxWidth <= 0 || len(runes) == 0 {
		return []rune{}
	}
	width := 0
	end := 0
	for i, r := range runes {
		w := runewidth.RuneWidth(r)
		if w < 1 {
			w = 1
		}
		if width+w > maxWidth {
			break
		}
		width += w
		end = i + 1
	}
	return runes[:end]
}
