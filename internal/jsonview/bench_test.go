package jsonview

import (
	"context"
	"encoding/json"
	"fmt"
	"github.com/dayangraham/gijq/internal/jsontest"
	"strings"
	"sync"
	"testing"
)

// syntheticJSON is copied from internal/perf/replay_benchmark_test.go.
func syntheticJSON(sizeMB int) []byte {
	targetBytes := sizeMB * 1024 * 1024
	if targetBytes < 1024 {
		targetBytes = 1024
	}

	payload := strings.Repeat("abcdefghijklmnopqrstuvwxyz0123456789", 4)
	statuses := []string{"ok", "warn", "error"}

	var b strings.Builder
	b.Grow(targetBytes + 4096)
	b.WriteString(`{"items":[`)

	for i := 0; b.Len() < targetBytes-1024; i++ {
		if i > 0 {
			b.WriteByte(',')
		}
		fmt.Fprintf(
			&b,
			`{"id":%d,"name":"item-%06d","status":"%s","metrics":{"count":%d,"ratio":%.3f},"tags":["alpha","beta","gamma"],"payload":"%s"}`,
			i,
			i,
			statuses[i%len(statuses)],
			i%1000,
			float64(i%1000)/1000.0,
			payload,
		)
	}

	b.WriteString(`],"meta":{"generated":true,"seed":"deterministic-v1"}}`)
	return []byte(b.String())
}

var (
	benchOnce sync.Once
	benchData any
	benchRoot []any // result of `.`
	benchItem []any // result of `.items`
	benchDoc  *Doc  // doc over `.`
	benchIdx  *Index
)

func benchSetup(b *testing.B) {
	benchOnce.Do(func() {
		if err := json.Unmarshal(syntheticJSON(55), &benchData); err != nil {
			panic(err)
		}
		benchRoot = []any{benchData}
		benchItem = []any{benchData.(map[string]any)["items"]}
		benchDoc = NewDoc(benchRoot)
		benchIdx, _ = benchDoc.BuildIndex(context.Background(), DefaultStep, nil)
	})
	b.ReportAllocs()
	b.ResetTimer()
}

const screen = 50

// 1. Reference: full formatting.
func BenchmarkReferenceFormatRoot(b *testing.B) {
	benchSetup(b)
	for i := 0; i < b.N; i++ {
		_ = jsontest.ReferenceFormat(benchRoot)
	}
}

func BenchmarkReferenceFormatItems(b *testing.B) {
	benchSetup(b)
	for i := 0; i < b.N; i++ {
		_ = jsontest.ReferenceFormat(benchItem)
	}
}

// 2. Lazy first screen, no index. NewDoc is included (fresh state each time,
// so the first-use classification scan is measured, which is the worst case).
func BenchmarkLazyFirstScreenRoot(b *testing.B) {
	benchSetup(b)
	for i := 0; i < b.N; i++ {
		_ = NewDoc(benchRoot).Lines(nil, 0, screen)
	}
}

func BenchmarkLazyFirstScreenItems(b *testing.B) {
	benchSetup(b)
	for i := 0; i < b.N; i++ {
		_ = NewDoc(benchItem).Lines(nil, 0, screen)
	}
}

// Same, once the value has been classified already (scroll/redraw cost).
func BenchmarkLazyLinesWarm(b *testing.B) {
	benchSetup(b)
	benchDoc.Lines(nil, 0, 1)
	for i := 0; i < b.N; i++ {
		_ = benchDoc.Lines(nil, 0, screen)
	}
}

// 3. Index build.
func BenchmarkIndexBuildRoot(b *testing.B) {
	benchSetup(b)
	var ix *Index
	for i := 0; i < b.N; i++ {
		ix, _ = NewDoc(benchRoot).BuildIndex(context.Background(), DefaultStep, nil)
	}
	b.ReportMetric(float64(ix.Lines()), "lines")
	b.ReportMetric(float64(ix.MaxWidth()), "maxwidth")
}

// Index build when value classification is already cached.
func BenchmarkIndexBuildRootWarm(b *testing.B) {
	benchSetup(b)
	for i := 0; i < b.N; i++ {
		_, _ = benchDoc.BuildIndex(context.Background(), DefaultStep, nil)
	}
}

// 4. Deep random access.
func BenchmarkLinesDeepIndexed(b *testing.B) {
	benchSetup(b)
	for i := 0; i < b.N; i++ {
		_ = benchDoc.Lines(benchIdx, 3_000_000, screen)
	}
}

func BenchmarkLinesDeepNoIndex(b *testing.B) {
	benchSetup(b)
	for i := 0; i < b.N; i++ {
		_ = benchDoc.Lines(nil, 3_000_000, screen)
	}
}

func BenchmarkWriteToRoot(b *testing.B) {
	benchSetup(b)
	for i := 0; i < b.N; i++ {
		_, _ = benchDoc.WriteTo(discard{})
	}
}

type discard struct{}

func (discard) Write(p []byte) (int, error) { return len(p), nil }

// 5. Streaming.
const streamQuery = `.items[] | select(.status == "ok") | .id`

func benchQuery(b *testing.B) *gojqCode {
	benchSetup(b)
	return compileBench(b, streamQuery)
}

func BenchmarkStreamFirstScreen(b *testing.B) {
	code := benchQuery(b)
	for i := 0; i < b.N; i++ {
		ctx, cancel := context.WithCancel(context.Background())
		c := Collect(ctx, code.run(ctx, benchData), CollectOptions{FirstScreenLines: screen})
		_ = c.Snapshot().Doc.Lines(nil, 0, screen)
		cancel()
		<-c.Done()
	}
}

func BenchmarkStreamDrainAndReferenceFormat(b *testing.B) {
	code := benchQuery(b)
	for i := 0; i < b.N; i++ {
		vals, _ := drain(code.run(context.Background(), benchData))
		_ = jsontest.ReferenceFormat(vals)
	}
}

func BenchmarkStreamDrainOnly(b *testing.B) {
	code := benchQuery(b)
	for i := 0; i < b.N; i++ {
		_, _ = drain(code.run(context.Background(), benchData))
	}
}

// Full streaming collection to completion (background cost).
func BenchmarkStreamCollectAll(b *testing.B) {
	code := benchQuery(b)
	for i := 0; i < b.N; i++ {
		c := Collect(context.Background(), code.run(context.Background(), benchData), CollectOptions{FirstScreenLines: screen})
		<-c.Done()
	}
}
