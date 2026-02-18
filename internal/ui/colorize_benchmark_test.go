package ui

import (
	"strconv"
	"strings"
	"testing"

	"github.com/dayangraham/gijq/internal/jq"
)

func BenchmarkColorizeJSON(b *testing.B) {
	large := syntheticPrettyJSON(120000)
	visible := visibleWindow(large, 0, 220)

	b.Run("full", func(b *testing.B) {
		b.ReportAllocs()
		b.ResetTimer()
		for i := 0; i < b.N; i++ {
			_ = colorizeJSON(large)
		}
	})

	b.Run("visible-window", func(b *testing.B) {
		b.ReportAllocs()
		b.ResetTimer()
		for i := 0; i < b.N; i++ {
			_ = colorizeJSON(visible)
		}
	})
}

func BenchmarkRenderContentWindow(b *testing.B) {
	raw := syntheticPrettyJSON(120000)
	lines := strings.Split(raw, "\n")

	newModel := func() Model {
		return Model{
			result:     jq.Result{Raw: raw},
			lines:      lines,
			width:      180,
			height:     60,
			ready:      true,
			colorCache: newLineColorCache(4096),
			output:     newViewport(180, 52),
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
			m.output.YOffset = i % maxOffset
			_ = m.renderContent()
		}
	})

	b.Run("cold-cache", func(b *testing.B) {
		b.ReportAllocs()
		b.ResetTimer()
		for i := 0; i < b.N; i++ {
			m := newModel()
			m.output.YOffset = i % 1000
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
