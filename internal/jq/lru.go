package jq

import "container/list"

// cacheCap bounds each of the per-service caches.
const cacheCap = 256

type lruEntry[V any] struct {
	key string
	val V
}

// lru is a small bounded LRU cache. It is not safe for concurrent use;
// callers must hold an exclusive lock for Get and Put (Get updates recency).
type lru[V any] struct {
	cap   int
	ll    *list.List
	items map[string]*list.Element
}

func newLRU[V any](capacity int) *lru[V] {
	return &lru[V]{cap: capacity, ll: list.New(), items: make(map[string]*list.Element, capacity)}
}

func (c *lru[V]) Get(key string) (V, bool) {
	if el, ok := c.items[key]; ok {
		c.ll.MoveToFront(el)
		return el.Value.(*lruEntry[V]).val, true
	}
	var zero V
	return zero, false
}

func (c *lru[V]) Put(key string, val V) {
	if el, ok := c.items[key]; ok {
		el.Value.(*lruEntry[V]).val = val
		c.ll.MoveToFront(el)
		return
	}
	c.items[key] = c.ll.PushFront(&lruEntry[V]{key: key, val: val})
	for c.ll.Len() > c.cap {
		last := c.ll.Back()
		c.ll.Remove(last)
		delete(c.items, last.Value.(*lruEntry[V]).key)
	}
}

func (c *lru[V]) Len() int { return c.ll.Len() }
