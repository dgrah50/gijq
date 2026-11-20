package ui

import (
	"context"
	"strconv"
	"strings"
	"testing"

	"github.com/dayangraham/gijq/internal/jsonview"
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

// benchModel is a ready Model showing values as a complete, indexed result.
func benchModel(b *testing.B, width, height int, values ...any) Model {
	b.Helper()
	doc := jsonview.NewDoc(values)
	ix, err := doc.BuildIndex(context.Background(), 0, nil)
	if err != nil {
		b.Fatal(err)
	}
	r := newResultView()
	r.snap = &jsonview.Snapshot{Values: values, Doc: doc, Done: true}
	r.index = ix
	return Model{result: r, width: width, height: height, ready: true}
}

func syntheticValue(items int) any {
	arr := make([]any, items)
	for i := range arr {
		arr[i] = map[string]any{
			"id": i, "name": "item-" + intToString(i), "ok": i%2 == 0,
			"ratio": float64(i%1000) / 1000,
		}
	}
	return map[string]any{"items": arr}
}

func BenchmarkRenderContentWindow(b *testing.B) {
	newModel := func() Model { return benchModel(b, 180, 60, syntheticValue(120000)) }
	probe := newModel()
	total, _ := probe.result.lineCount()

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
		maxOffset := max(total-m.contentHeight(), 1)
		b.ReportAllocs()
		b.ResetTimer()
		for i := 0; i < b.N; i++ {
			m.yOffset = i % maxOffset
			_ = m.renderContent()
		}
	})

	b.Run("cold-cache", func(b *testing.B) {
		m := newModel()
		b.ReportAllocs()
		b.ResetTimer()
		for i := 0; i < b.N; i++ {
			m.result.cache = &visibleCache{}
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

func intToString(v int) string {
	return strconv.Itoa(v)
}

func BenchmarkHandleResult(b *testing.B) {
	values := []any{syntheticValue(120000)}
	snap := &jsonview.Snapshot{Values: values, Doc: jsonview.NewDoc(values), Done: true}
	m := Model{width: 180, height: 60, ready: true, result: newResultView(), telemetry: newLatencyTelemetry(false)}
	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		m.query.active, m.query.phase = i+1, queryStreaming
		next, _ := m.Update(resultMsg{seq: i + 1, snap: snap})
		m = next.(Model)
	}
}

func longLineModel(b *testing.B) Model {
	return benchModel(b, 180, 60, strings.Repeat("abcdefghij", 500000))
}

func BenchmarkRenderLongLine(b *testing.B) {
	m := longLineModel(b)
	m.outputXOffset = 1000
	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_ = m.renderView()
	}
}

func BenchmarkRenderLongLineDeep(b *testing.B) {
	m := longLineModel(b)
	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		m.outputXOffset = 4000000 + i%200
		_ = m.renderView()
	}
}
