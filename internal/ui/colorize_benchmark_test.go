package ui

import (
	"strconv"
	"strings"
	"testing"

	"github.com/dayangraham/gijq/internal/jq"
)

func BenchmarkHighlightWindow(b *testing.B) {
	lines := strings.Split(syntheticPrettyJSON(120000), "\n")
	const rows, width = 52, 180

	b.Run("offset-0", func(b *testing.B) {
		b.ReportAllocs()
		b.ResetTimer()
		for i := 0; i < b.N; i++ {
			for _, l := range lines[1000 : 1000+rows] {
				_, _, _, _ = highlightWindow(l, 0, width, true, nil)
			}
		}
	})

	b.Run("h-scrolling", func(b *testing.B) {
		b.ReportAllocs()
		b.ResetTimer()
		for i := 0; i < b.N; i++ {
			x := i % 120
			for _, l := range lines[1000 : 1000+rows] {
				_, _, _, _ = highlightWindow(l, x, width, true, nil)
			}
		}
	})
}

func BenchmarkRenderContentWindow(b *testing.B) {
	raw := syntheticPrettyJSON(120000)
	lines := strings.Split(raw, "\n")

	newModel := func() Model {
		return Model{
			result: jq.Result{Raw: raw},
			lines:  lines,
			width:  180,
			height: 60,
			ready:  true,
			hl:     newHighlightCache(),
		}
	}

	b.Run("fixed-offset", func(b *testing.B) {
		m := newModel()
		b.ReportAllocs()
		b.ResetTimer()
		for i := 0; i < b.N; i++ {
			_ = m.renderContent()
		}
	})

	b.Run("scrolling", func(b *testing.B) {
		m := newModel()
		maxOffset := len(lines) - m.contentHeight()
		if maxOffset < 1 {
			maxOffset = 1
		}

		b.ReportAllocs()
		b.ResetTimer()
		for i := 0; i < b.N; i++ {
			m.yOffset = i % maxOffset
			_ = m.renderContent()
		}
	})

	b.Run("cold-cache", func(b *testing.B) {
		b.ReportAllocs()
		b.ResetTimer()
		for i := 0; i < b.N; i++ {
			m := newModel()
			m.yOffset = i % 1000
			_ = m.renderContent()
		}
	})
}

func syntheticPrettyJSON(items int) string {
	var b strings.Builder
	b.Grow(items * 64)
	b.WriteString("{\n  \"items\": [\n")
	for i := 0; i < items; i++ {
		if i > 0 {
			b.WriteString(",\n")
		}
		b.WriteString("    {\"id\": ")
		b.WriteString(intToString(i))
		b.WriteString(", \"name\": \"item-")
		b.WriteString(intToString(i))
		b.WriteString("\", \"ok\": ")
		if i%2 == 0 {
			b.WriteString("true")
		} else {
			b.WriteString("false")
		}
		b.WriteString(", \"ratio\": 0.")
		b.WriteString(intToString(i % 1000))
		b.WriteString("}")
	}
	b.WriteString("\n  ]\n}")
	return b.String()
}

func visibleWindow(s string, start, maxLines int) string {
	lines := strings.Split(s, "\n")
	if start >= len(lines) {
		return ""
	}
	end := start + maxLines
	if end > len(lines) {
		end = len(lines)
	}
	return strings.Join(lines[start:end], "\n")
}

func intToString(v int) string {
	return strconv.Itoa(v)
}

func BenchmarkHandleResult(b *testing.B) {
	raw := syntheticPrettyJSON(120000)
	result := jq.Result{Raw: raw}

	b.Run("background-postprocess", func(b *testing.B) {
		b.ReportAllocs()
		for i := 0; i < b.N; i++ {
			_, _ = processResult(result)
		}
	})

	b.Run("ui-goroutine-update", func(b *testing.B) {
		lines, w := processResult(result)
		m := Model{width: 180, height: 60, ready: true, hl: newHighlightCache(), telemetry: newLatencyTelemetry(false)}
		b.ReportAllocs()
		b.ResetTimer()
		for i := 0; i < b.N; i++ {
			m.activeQuerySeq = i + 1
			next, _ := m.Update(resultMsg{seq: i + 1, result: result, lines: lines, maxLineWidth: w})
			m = next.(Model)
		}
	})
}

func BenchmarkRenderLongLine(b *testing.B) {
	raw := `"` + strings.Repeat("abcdefghij", 500000) + `"`
	m := Model{
		result: jq.Result{Raw: raw},
		lines:  []string{raw},
		width:  180,
		height: 60,
		ready:  true,
		hl:     newHighlightCache(),
	}
	m.maxLineWidth = len(raw)
	m.outputXOffset = 1000
	_ = m.renderView() // build checkpoints (one-off cost)
	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_ = m.renderView()
	}
}

func BenchmarkRenderLongLineDeep(b *testing.B) {
	raw := `"` + strings.Repeat("abcdefghij", 500000) + `"`
	m := Model{
		result: jq.Result{Raw: raw},
		lines:  []string{raw},
		width:  180,
		height: 60,
		ready:  true,
		hl:     newHighlightCache(),
	}
	m.maxLineWidth = len(raw)
	m.outputXOffset = 4000000
	_ = m.renderView() // build checkpoints (one-off cost)
	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		m.outputXOffset = 4000000 + i%200
		_ = m.renderView()
	}
}
