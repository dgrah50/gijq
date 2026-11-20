package ui

import (
	"fmt"
	"path/filepath"
	"strings"
	"testing"

	tea "charm.land/bubbletea/v2"

	"github.com/dayangraham/gijq/internal/autocomplete"
	"github.com/dayangraham/gijq/internal/clipboard"
	"github.com/dayangraham/gijq/internal/history"
	"github.com/dayangraham/gijq/internal/jq"
)

// BenchmarkQueryToFrame measures "query -> first frame on screen" of the
// streaming, lazy pipeline: run the command startExecute returns (compile,
// collect the first screen), apply the resultMsg on the "UI goroutine" and
// render a full frame. The 30ms debounce is excluded. The rest of the
// collection and the index build run in the background after the first frame
// and are cancelled (outside the timer) before the next iteration.
//
// Baseline of the previous whole-string pipeline (Execute, split, measure,
// prebuild checkpoints, apply, render; same 160x50 model, Apple M-series):
//
//	55 MB: .  387 ms | .items  380 ms | select .id  66 ms | select .metrics  113 ms | add  33 ms
//	10 MB: .   70 ms | .items   68 ms
var e2eFilters = []string{
	`.`,
	`.items`,
	`.items[0]`,
	`.items | length`,
	`.items[] | select(.status == "ok") | .id`,
	`.items[] | select(.status == "ok") | .metrics`,
	`[.items[] | .metrics.count] | add`,
}

func BenchmarkQueryToFrame(b *testing.B) {
	for _, sizeMB := range []int{10, 55} {
		b.Run(fmt.Sprintf("%dMB", sizeMB), func(b *testing.B) {
			jqSvc := e2eNewJQService(b, sizeMB)

			for _, filter := range e2eFilters {
				b.Run(e2eSanitizeName(filter), func(b *testing.B) {
					m := e2eNewModel(b, jqSvc)
					m.setFilter(filter)

					var lines int

					b.ReportAllocs()
					b.ResetTimer()
					for i := 0; i < b.N; i++ {
						seq := i + 1
						m.query.latest = seq

						// 1. dispatch + first screen (the query command)
						cmd := m.startExecute(seq)
						msg := cmd().(resultMsg)
						if msg.err != nil || msg.snap.Err != nil {
							b.Fatalf("filter %q failed: %v %v", filter, msg.err, msg.snap)
						}
						// 2. apply on the UI goroutine
						next, _ := m.Update(msg)
						m = next.(Model)
						// 3. full frame render
						_ = m.View()

						b.StopTimer()
						lines, _ = m.result.lineCount()
						m.query.stop() // cancel the background work
						<-msg.collector.Done()
						b.StartTimer()
					}
					b.StopTimer()
					b.ReportMetric(float64(lines), "firstScreenLines")
				})
			}
		})
	}
}

func BenchmarkStartupParse(b *testing.B) {
	for _, sizeMB := range []int{10, 55} {
		b.Run(fmt.Sprintf("%dMB", sizeMB), func(b *testing.B) {
			data := e2eSyntheticJSON(sizeMB)

			b.ReportAllocs()
			b.ResetTimer()
			for i := 0; i < b.N; i++ {
				if _, err := jq.NewService(data); err != nil {
					b.Fatalf("parse failed: %v", err)
				}
			}
		})
	}
}

// e2eNewJQService builds the jq service for a synthetic input of sizeMB.
// The build happens outside the timer.
func e2eNewJQService(b *testing.B, sizeMB int) *jq.Service {
	b.Helper()
	b.StopTimer()
	defer b.StartTimer()
	svc, err := jq.NewService(e2eSyntheticJSON(sizeMB))
	if err != nil {
		b.Fatalf("failed to build jq service: %v", err)
	}
	return svc
}

// e2eNewModel builds a Model the same way newTestModel does, sized 160x50.
func e2eNewModel(b *testing.B, jqSvc *jq.Service) Model {
	b.Helper()
	hist, err := history.NewStore(filepath.Join(b.TempDir(), "hist.json"))
	if err != nil {
		b.Fatal(err)
	}
	m := NewModel(jqSvc, autocomplete.NewService(jqSvc), hist, clipboard.NewService(), Config{Filename: "bench.json", Filepath: "/bench.json"})
	next, _ := m.Update(tea.WindowSizeMsg{Width: 160, Height: 50})
	return next.(Model)
}

// e2eSanitizeName turns a filter into a sub-benchmark name: runs of
// non-alphanumeric characters become "_", and "." alone becomes "identity".
func e2eSanitizeName(filter string) string {
	var b strings.Builder
	underscore := true
	for _, r := range filter {
		if (r >= 'a' && r <= 'z') || (r >= 'A' && r <= 'Z') || (r >= '0' && r <= '9') {
			b.WriteRune(r)
			underscore = false
			continue
		}
		if !underscore {
			b.WriteByte('_')
			underscore = true
		}
	}
	name := strings.TrimSuffix(b.String(), "_")
	if name == "" {
		return "identity"
	}
	return name
}

// e2eSyntheticJSON is a copy of syntheticJSON from internal/perf/replay_benchmark_test.go
// (package-private there, so it is duplicated here to keep the input identical).
func e2eSyntheticJSON(sizeMB int) []byte {
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
