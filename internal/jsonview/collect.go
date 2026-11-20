package jsonview

import (
	"context"
	"sync/atomic"
	"time"

	"github.com/itchyny/gojq"
)

// Snapshot is an immutable view of collection progress. Values is a
// cap-limited prefix of the collector's buffer: never modify it.
//
// Error semantics: the original implementation turned any error value from
// the iterator into a failed result. That is preserved: when Err != nil the
// result as a whole is an error and Values only holds what arrived before it
// (a caller may choose to show it, but the legacy behaviour is to show
// nothing). Err is also set to ctx.Err() on cancellation.
type Snapshot struct {
	Values []any
	Doc    *Doc // Doc over Values
	Done   bool // iterator exhausted, failed or cancelled
	Err    error
}

// CollectOptions configures Collect.
type CollectOptions struct {
	// FirstScreenLines: Collect returns once this many output lines are
	// available or the iterator ends. Default 50.
	FirstScreenLines int
	// PublishEvery is the minimum interval between background snapshots.
	// Default 30ms.
	PublishEvery time.Duration
}

// Collector pulls values from an iterator in the background.
type Collector struct {
	snap    atomic.Pointer[Snapshot]
	updated chan struct{}
	done    chan struct{}
}

// Snapshot returns the latest published snapshot.
func (c *Collector) Snapshot() *Snapshot { return c.snap.Load() }

// Updated receives a coalesced signal whenever a new snapshot is published.
func (c *Collector) Updated() <-chan struct{} { return c.updated }

// Done is closed after the final snapshot is published and the background
// goroutine has exited (so it also means the iterator is no longer used).
func (c *Collector) Done() <-chan struct{} { return c.done }

// Collect synchronously pulls values until the first screen is filled (or
// the iterator ends), then continues in a goroutine until exhaustion, error
// or ctx cancellation. The iterator must not be used by the caller.
func Collect(ctx context.Context, iter gojq.Iter, opts CollectOptions) *Collector {
	if opts.FirstScreenLines <= 0 {
		opts.FirstScreenLines = 50
	}
	if opts.PublishEvery <= 0 {
		opts.PublishEvery = 30 * time.Millisecond
	}
	c := &Collector{updated: make(chan struct{}, 1), done: make(chan struct{})}

	var values []any
	sh := &shared{}
	mkDoc := func() *Doc {
		n := len(values)
		return &Doc{values: values[:n:n], shared: sh}
	}
	publish := func(done bool, err error) {
		n := len(values)
		c.snap.Store(&Snapshot{Values: values[:n:n], Doc: mkDoc(), Done: done, Err: err})
		select {
		case c.updated <- struct{}{}:
		default:
		}
	}
	// pull returns false when collection is over.
	pull := func() (finished bool, err error) {
		if err := ctx.Err(); err != nil {
			return true, err
		}
		v, ok := iter.Next()
		if !ok {
			return true, ctx.Err()
		}
		if err := ctx.Err(); err != nil {
			return true, err
		}
		if e, isErr := v.(error); isErr {
			return true, e
		}
		values = append(values, v)
		return false, nil
	}

	// Phase 1: first screen, synchronously.
	lines := 0
	for lines < opts.FirstScreenLines {
		fin, err := pull()
		if fin {
			publish(true, err)
			close(c.done)
			return c
		}
		lines += mkDoc().countLines(len(values)-1, opts.FirstScreenLines-lines)
	}
	publish(false, nil)

	// Phase 2: the rest, in the background.
	go func() {
		defer close(c.done)
		last := time.Now()
		for i := 0; ; i++ {
			fin, err := pull()
			if fin {
				publish(true, err)
				return
			}
			if i&255 == 255 && time.Since(last) >= opts.PublishEvery {
				publish(false, nil)
				last = time.Now()
			}
		}
	}()
	return c
}
