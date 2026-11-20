package ui

// Layout and scrolling geometry for the output and suggestion panes.

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

// maxYOffset is the largest scroll offset. Until the index is complete it is
// derived from the lines known so far (a lower bound), so the user can scroll
// further as the index grows.
func (m Model) maxYOffset() int {
	lines, _ := m.result.lineCount()
	n := lines - m.contentHeight()
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
	m.observeVisibleWidth()
}

// observeVisibleWidth records the widths of the lines on screen as a lower
// bound for horizontal scrolling (see resultView.seenWidth).
func (m *Model) observeVisibleWidth() {
	if m.result.hasError() {
		return
	}
	for _, w := range m.result.window(m.yOffset, m.contentHeight()).widths {
		m.result.observe(w)
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
	width, _ := m.result.maxWidth()
	maxOffset := width - outputWidth
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
