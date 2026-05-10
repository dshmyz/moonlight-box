package cache

import (
	"container/list"
	"sync"
)

type LRUCache struct {
	capacity int
	items    map[string]*list.Element
	evictList *list.List
	mu       sync.RWMutex
}

type lruEntry struct {
	key   string
	value interface{}
}

func NewLRUCache(capacity int) *LRUCache {
	if capacity <= 0 {
		capacity = 1000
	}
	return &LRUCache{
		capacity:  capacity,
		items:     make(map[string]*list.Element, capacity),
		evictList: list.New(),
	}
}

func (c *LRUCache) Get(key string) (interface{}, bool) {
	c.mu.Lock()
	defer c.mu.Unlock()

	if elem, ok := c.items[key]; ok {
		c.evictList.MoveToFront(elem)
		return elem.Value.(*lruEntry).value, true
	}
	return nil, false
}

func (c *LRUCache) Set(key string, value interface{}) {
	c.mu.Lock()
	defer c.mu.Unlock()

	if elem, ok := c.items[key]; ok {
		c.evictList.MoveToFront(elem)
		elem.Value.(*lruEntry).value = value
		return
	}

	if c.evictList.Len() >= c.capacity {
		c.evictOldest()
	}

	entry := &lruEntry{
		key:   key,
		value: value,
	}
	elem := c.evictList.PushFront(entry)
	c.items[key] = elem
}

func (c *LRUCache) Delete(key string) {
	c.mu.Lock()
	defer c.mu.Unlock()

	if elem, ok := c.items[key]; ok {
		c.evictList.Remove(elem)
		delete(c.items, key)
	}
}

func (c *LRUCache) Len() int {
	c.mu.RLock()
	defer c.mu.RUnlock()
	return len(c.items)
}

func (c *LRUCache) Clear() {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.items = make(map[string]*list.Element, c.capacity)
	c.evictList = list.New()
}

func (c *LRUCache) evictOldest() {
	if elem := c.evictList.Back(); elem != nil {
		c.evictList.Remove(elem)
		delete(c.items, elem.Value.(*lruEntry).key)
	}
}
