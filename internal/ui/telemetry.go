package ui

import (
	"context"
	"errors"
	"fmt"
	"slices"
	"time"
)

type latencyTelemetry struct {
	enabled bool

	pending map[int]latencySpan

	keyToFrame []time.Duration
	keyToStart []time.Duration
	runTime    []time.Duration

	droppedDebounce int
	staleResults    int
	canceledResults int
}

type latencySpan struct {
	queuedAt   time.Time
	dispatched time.Time
}

func newLatencyTelemetry(enabled bool) *latencyTelemetry {
	return &latencyTelemetry{
		enabled: enabled,
		pending: map[int]latencySpan{},
	}
}

func (t *latencyTelemetry) OnQueued(seq int) {
	if !t.enabled {
		return
	}
	t.pending[seq] = latencySpan{queuedAt: time.Now()}
}

func (t *latencyTelemetry) OnDispatch(seq int) {
	if !t.enabled {
		return
	}
	span, ok := t.pending[seq]
	if !ok {
		span = latencySpan{queuedAt: time.Now()}
	}
	span.dispatched = time.Now()
	t.pending[seq] = span
	if !span.queuedAt.IsZero() {
		t.keyToStart = append(t.keyToStart, span.dispatched.Sub(span.queuedAt))
	}
}

func (t *latencyTelemetry) OnDebounceDropped(seq int) {
	if !t.enabled {
		return
	}
	if _, ok := t.pending[seq]; ok {
		delete(t.pending, seq)
		t.droppedDebounce++
	}
}

func (t *latencyTelemetry) OnResult(seq int, err error, accepted bool) {
	if !t.enabled {
		return
	}

	span, ok := t.pending[seq]
	if !ok {
		if errorsIsCanceled(err) {
			t.canceledResults++
		}
		if !accepted {
			t.staleResults++
		}
		return
	}
	delete(t.pending, seq)

	now := time.Now()
	if accepted && !errorsIsCanceled(err) {
		if !span.queuedAt.IsZero() {
			t.keyToFrame = append(t.keyToFrame, now.Sub(span.queuedAt))
		}
		if !span.dispatched.IsZero() {
			t.runTime = append(t.runTime, now.Sub(span.dispatched))
		}
	}
	if errorsIsCanceled(err) {
		t.canceledResults++
	}
	if !accepted {
		t.staleResults++
	}
}

func (t *latencyTelemetry) Summary() (string, bool) {
	if !t.enabled {
		return "", false
	}
	if len(t.keyToFrame) == 0 {
		return "telemetry: no completed samples yet", true
	}

	keyP50, keyP95, keyP99 := percentiles(t.keyToFrame)
	startP50, startP95, startP99 := percentiles(t.keyToStart)
	runP50, runP95, runP99 := percentiles(t.runTime)

	return fmt.Sprintf(
		"telemetry keypress->frame samples=%d p50=%s p95=%s p99=%s | keypress->dispatch p50=%s p95=%s p99=%s | execute p50=%s p95=%s p99=%s | dropped(debounce)=%d stale=%d canceled=%d",
		len(t.keyToFrame),
		keyP50, keyP95, keyP99,
		startP50, startP95, startP99,
		runP50, runP95, runP99,
		t.droppedDebounce, t.staleResults, t.canceledResults,
	), true
}

func percentiles(values []time.Duration) (time.Duration, time.Duration, time.Duration) {
	if len(values) == 0 {
		return 0, 0, 0
	}

	cpy := make([]time.Duration, len(values))
	copy(cpy, values)
	slices.Sort(cpy)

	return percentile(cpy, 50), percentile(cpy, 95), percentile(cpy, 99)
}

func percentile(sorted []time.Duration, p int) time.Duration {
	if len(sorted) == 0 {
		return 0
	}
	if p <= 0 {
		return sorted[0]
	}
	if p >= 100 {
		return sorted[len(sorted)-1]
	}

	idx := int(float64(len(sorted)-1) * (float64(p) / 100.0))
	return sorted[idx]
}

func errorsIsCanceled(err error) bool {
	return errors.Is(err, context.Canceled)
}
