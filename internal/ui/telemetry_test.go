package ui

import (
	"strings"
	"testing"
	"time"
)

func TestPercentiles(t *testing.T) {
	values := []time.Duration{
		10 * time.Millisecond,
		20 * time.Millisecond,
		30 * time.Millisecond,
		40 * time.Millisecond,
		50 * time.Millisecond,
	}

	p50, p95, p99 := percentiles(values)
	if p50 != 30*time.Millisecond {
		t.Fatalf("p50 = %v, want 30ms", p50)
	}
	if p95 != 40*time.Millisecond {
		t.Fatalf("p95 = %v, want 40ms", p95)
	}
	if p99 != 40*time.Millisecond {
		t.Fatalf("p99 = %v, want 40ms", p99)
	}
}

func TestTelemetrySummaryDisabled(t *testing.T) {
	telemetry := newLatencyTelemetry(false)
	if summary, ok := telemetry.Summary(); ok || summary != "" {
		t.Fatalf("Summary() = (%q, %v), want empty false", summary, ok)
	}
}

func TestTelemetrySummaryNoSamples(t *testing.T) {
	telemetry := newLatencyTelemetry(true)
	summary, ok := telemetry.Summary()
	if !ok {
		t.Fatal("Summary() should be available when enabled")
	}
	if !strings.Contains(summary, "no completed samples") {
		t.Fatalf("unexpected summary: %q", summary)
	}
}
