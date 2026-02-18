package perf

import (
	"fmt"
	"strings"
	"testing"

	"github.com/dayangraham/gijq/internal/autocomplete"
	"github.com/dayangraham/gijq/internal/jq"
)

func BenchmarkExecuteFilters(b *testing.B) {
	for _, sizeMB := range []int{10, 55} {
		b.Run(fmt.Sprintf("%dMB", sizeMB), func(b *testing.B) {
			jqSvc, _ := newServicesForBench(b, sizeMB)
			filters := []string{
				".",
				".meta",
				".items[0].name",
				".items | length",
				`.items[] | select(.status == "ok") | .id`,
				`.items[] | select(.metrics.count > 500) | .metrics.ratio`,
			}

			b.ReportAllocs()
			b.ResetTimer()

			for i := 0; i < b.N; i++ {
				result := jqSvc.Execute(filters[i%len(filters)])
				if result.Error != nil {
					b.Fatalf("filter failed: %v", result.Error)
				}
			}
		})
	}
}

func BenchmarkTypingReplay(b *testing.B) {
	filter := `.items[] | select(.status == "ok") | .metrics.count`
	steps := filterPrefixes(filter)

	for _, sizeMB := range []int{10, 55} {
		b.Run(fmt.Sprintf("%dMB", sizeMB), func(b *testing.B) {
			jqSvc, acSvc := newServicesForBench(b, sizeMB)

			b.ReportAllocs()
			b.ResetTimer()

			for i := 0; i < b.N; i++ {
				lastPath := ""
				for _, step := range steps {
					ctx := acSvc.ParseContext(step)
					if ctx.Path != lastPath {
						_, _ = jqSvc.KeysAt(ctx.Path)
						lastPath = ctx.Path
					}
					_ = jqSvc.Execute(step)
				}
			}
		})
	}
}

func newServicesForBench(b *testing.B, sizeMB int) (*jq.Service, *autocomplete.Service) {
	b.Helper()

	jsonData := syntheticJSON(sizeMB)
	jqSvc, err := jq.NewService(jsonData)
	if err != nil {
		b.Fatalf("failed to build jq service: %v", err)
	}
	acSvc := autocomplete.NewService(jqSvc)
	return jqSvc, acSvc
}

func filterPrefixes(filter string) []string {
	parts := make([]string, 0, len(filter))
	var builder strings.Builder
	builder.Grow(len(filter))
	for _, r := range filter {
		builder.WriteRune(r)
		parts = append(parts, builder.String())
	}
	return parts
}

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
