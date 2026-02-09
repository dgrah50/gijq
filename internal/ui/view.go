package ui

import (
	"fmt"
	"regexp"
	"strings"

	"github.com/charmbracelet/lipgloss"
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
	help := helpStyle.Render("tab: autocomplete | ctrl+h: history | ctrl+y: copy output | ctrl+f: copy filter | enter: quit")

	var status string
	if m.status != "" {
		status = statusStyle.Render(m.status)
	} else if m.result.Error != nil {
		status = errorStyle.Render("Error: " + m.result.Error.Error())
	}

	return fmt.Sprintf("%s  %s\n%s\n", title, help, status)
}

func (m Model) renderContent() string {
	outputWidth := m.width - m.suggestWidth() - 5

	// Get the raw content and colorize it
	var content string
	if m.result.Error != nil {
		content = errorStyle.Render(m.result.Error.Error())
	} else {
		content = colorizeJSON(m.result.Raw)
	}

	// Apply scrolling manually based on viewport position
	lines := strings.Split(content, "\n")
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
	for _, line := range visibleLines {
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
	outputPane := borderStyle.Render(visibleContent)

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
	keys, _ := m.jq.KeysAt(m.currentPath())
	if len(keys) == 0 {
		return labelStyle.Render("No keys")
	}

	var lines []string
	lines = append(lines, labelStyle.Render("Available keys:"))
	for i, k := range keys {
		lines = append(lines, suggestionStyle.Render("  "+k))
		if i >= m.contentHeight()-2 {
			lines = append(lines, helpStyle.Render(fmt.Sprintf("  ...+%d more", len(keys)-i-1)))
			break
		}
	}
	return strings.Join(lines, "\n")
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

	return fmt.Sprintf("\n%s%s\n%s%s", filterLabel, filter, fileLabel, file)
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
