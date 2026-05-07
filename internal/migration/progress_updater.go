package migration

import (
	"context"
	"sync"
	"time"

	"github.com/moonlight-box/registry/internal/model"
	"gorm.io/gorm"
)

type ProgressUpdater struct {
	taskID       uint
	db           *gorm.DB
	updateTicker *time.Ticker
	buffer       struct {
		processed int
		failed    int
		mu        sync.Mutex
	}
}

func NewProgressUpdater(taskID uint, db *gorm.DB, updateInterval time.Duration) *ProgressUpdater {
	return &ProgressUpdater{
		taskID:       taskID,
		db:           db,
		updateTicker: time.NewTicker(updateInterval),
	}
}

func (p *ProgressUpdater) IncrementProcessed() {
	p.buffer.mu.Lock()
	p.buffer.processed++
	p.buffer.mu.Unlock()
}

func (p *ProgressUpdater) IncrementFailed() {
	p.buffer.mu.Lock()
	p.buffer.failed++
	p.buffer.mu.Unlock()
}

func (p *ProgressUpdater) Start(ctx context.Context) {
	defer p.updateTicker.Stop()

	for {
		select {
		case <-p.updateTicker.C:
			p.flush()
		case <-ctx.Done():
			p.flush()
			return
		}
	}
}

func (p *ProgressUpdater) flush() {
	p.buffer.mu.Lock()
	processed := p.buffer.processed
	failed := p.buffer.failed
	p.buffer.processed = 0
	p.buffer.failed = 0
	p.buffer.mu.Unlock()

	if processed > 0 || failed > 0 {
		p.db.Model(&model.MigrationTask{}).
			Where("id = ?", p.taskID).
			Updates(map[string]interface{}{
				"processed_items": gorm.Expr("processed_items + ?", processed),
				"failed_items":    gorm.Expr("failed_items + ?", failed),
			})
	}
}

func (p *ProgressUpdater) Stop() {
	p.flush()
	p.updateTicker.Stop()
}
