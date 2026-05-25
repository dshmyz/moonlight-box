package migration

import (
	"context"
	"sync"

	"github.com/dshmyz/moonlight-box/internal/model"
)

type ComponentQueue struct {
	queue    chan model.MigrationItem
	ctx      context.Context
	cancel   context.CancelFunc
	closed   sync.Once
	isClosed bool
	closedMu sync.RWMutex
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
	q.closedMu.RLock()
	closed := q.isClosed
	q.closedMu.RUnlock()

	if closed {
		return false
	}

	select {
	case q.queue <- item:
		return true
	default:
		return false
	}
}

func (q *ComponentQueue) Push(item model.MigrationItem) bool {
	q.closedMu.RLock()
	closed := q.isClosed
	q.closedMu.RUnlock()

	if closed {
		return false
	}

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
	q.closed.Do(func() {
		q.closedMu.Lock()
		q.isClosed = true
		q.closedMu.Unlock()
		// 只关闭channel，不取消context
		// 这样消费者可以继续读取队列中已有的数据
		close(q.queue)
	})
}

func (q *ComponentQueue) Cancel() {
	q.cancel()
}

func (q *ComponentQueue) Len() int {
	return len(q.queue)
}

func (q *ComponentQueue) Cap() int {
	return cap(q.queue)
}
