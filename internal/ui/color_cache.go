package ui

// lineColorCache stores a bounded mapping of raw lines to colorized lines.
// This avoids repeatedly running regex colorization for lines that stay in view.
type lineColorCache struct {
	maxEntries int
	lines      map[string]string
	order      []string
}

func newLineColorCache(maxEntries int) *lineColorCache {
	if maxEntries < 1 {
		maxEntries = 1
	}
	return &lineColorCache{
		maxEntries: maxEntries,
		lines:      make(map[string]string, maxEntries),
		order:      make([]string, 0, maxEntries),
	}
}

func (c *lineColorCache) Colorize(line string) string {
	if line == "" {
		return ""
	}
	if colored, ok := c.lines[line]; ok {
		return colored
	}

	colored := colorizeJSON(line)
	c.lines[line] = colored
	c.order = append(c.order, line)

	if len(c.order) > c.maxEntries {
		evicted := c.order[0]
		c.order = c.order[1:]
		delete(c.lines, evicted)
	}

	return colored
}
