package ui

import (
	"context"
	"strings"
)

type keysStatus int

const (
	keysIdle keysStatus = iota
	keysLoading
	keysLoaded
	keysFailed
)

// keysState tracks the keys pane data for exactly one path. All transitions
// go through request and receive.
type keysState struct {
	status keysStatus
	path   string
	keys   []string
	lower  []string // lowercased keys, parallel to keys
	ctx    context.Context
	cancel context.CancelFunc
	seq    uint64
}

// request moves the state to loading path. It returns false when the path is
// already loading, loaded or failed (no fetch needed).
func (k *keysState) request(path string) bool {
	if k.status != keysIdle && k.path == path {
		return false
	}
	if k.cancel != nil {
		k.cancel()
	}
	seq := k.seq + 1
	ctx, cancel := context.WithCancel(context.Background())
	*k = keysState{status: keysLoading, path: path, ctx: ctx, cancel: cancel, seq: seq}
	return true
}

func (k *keysState) stop() {
	if k.cancel != nil {
		k.cancel()
	}
}

// receive applies a fetch result. Stale results (for a path other than the
// one most recently requested) are ignored.
func (k *keysState) receive(path string, keys []string, err error, requestSeq ...uint64) {
	if k.status != keysLoading || k.path != path || len(requestSeq) > 0 && k.seq != requestSeq[0] {
		return
	}
	if k.cancel != nil {
		k.cancel()
		k.cancel = nil
		k.ctx = nil
	}
	if err != nil {
		k.status = keysFailed
		k.keys, k.lower = nil, nil
		return
	}
	k.status = keysLoaded
	k.keys = keys
	k.lower = make([]string, len(keys))
	for i, key := range keys {
		k.lower[i] = strings.ToLower(key)
	}
}

// loadingFor reports whether keys for path are still being fetched.
func (k *keysState) loadingFor(path string) bool {
	return k.status == keysLoading && k.path == path
}

// filtered returns the keys whose lowercase form starts with the lowercase
// prefix, without re-lowercasing keys.
func (k *keysState) filtered(incomplete string) []string {
	if incomplete == "" || len(k.keys) == 0 {
		return k.keys
	}
	prefix := strings.ToLower(incomplete)
	matches := make([]string, 0, len(k.keys))
	for i, lk := range k.lower {
		if strings.HasPrefix(lk, prefix) {
			matches = append(matches, k.keys[i])
		}
	}
	return matches
}
