package migration

import (
	"context"

	"github.com/moonlight-box/registry/internal/model"
)

type ComponentQueue struct {
	queue  chan model.MigrationItem
	ctx    context.Context
	cancel context.CancelFunc
}

func NewComponentQueue(size int) *ComponentQueue {
	ctx, cancel := context.WithCancel(context.Background())
	return &ComponentQueue{
		queue:  make(chan model.MigrationItem, size),
		ctx:    ctx,
		cancel: cancel,
	}
}

func (q *ComponentQueue) TryPush(item model.MigrationItem) bool {
	select {
	case q.queue <- item:
		return true
	default:
		return false
	}
}

func (q *ComponentQueue) Push(item model.MigrationItem) bool {
	select {
	case q.queue <- item:
		return true
	case <-q.ctx.Done():
		return false
	}
}

func (q *ComponentQueue) Pop() (model.MigrationItem, bool) {
	select {
	case item, ok := <-q.queue:
		return item, ok
	case <-q.ctx.Done():
		return model.MigrationItem{}, false
	}
}

func (q *ComponentQueue) Close() {
	q.cancel()
	close(q.queue)
}

func (q *ComponentQueue) Len() int {
	return len(q.queue)
}

func (q *ComponentQueue) Cap() int {
	return cap(q.queue)
}
