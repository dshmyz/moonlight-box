package migration

import (
	"context"
	"sync"
	"time"

	"github.com/moonlight-box/registry/internal/model"
	"github.com/moonlight-box/registry/internal/repository"
	"github.com/sirupsen/logrus"
	"gorm.io/gorm"
)

type ProgressUpdater struct {
	taskID       uint
	db           *gorm.DB
	itemRepo     *repository.MigrationItemRepository
	updateTicker *time.Ticker
	buffer       struct {
		processed int
		failed    int
		mu        sync.Mutex
	}
	itemBuffer struct {
		updates []repository.ItemStatusUpdate
		mu      sync.Mutex
	}
}

func NewProgressUpdater(taskID uint, db *gorm.DB, itemRepo *repository.MigrationItemRepository, updateInterval time.Duration) *ProgressUpdater {
	return &ProgressUpdater{
		taskID:       taskID,
		db:           db,
		itemRepo:     itemRepo,
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

func (p *ProgressUpdater) UpdateItemStatus(itemID uint, status model.MigrationItemStatus, errMsg string) {
	p.itemBuffer.mu.Lock()
	p.itemBuffer.updates = append(p.itemBuffer.updates, repository.ItemStatusUpdate{
		ItemID:   itemID,
		Status:   status,
		ErrorMsg: errMsg,
	})
	p.itemBuffer.mu.Unlock()
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

	p.itemBuffer.mu.Lock()
	updates := p.itemBuffer.updates
	p.itemBuffer.updates = nil
	p.itemBuffer.mu.Unlock()

	if len(updates) > 0 && p.itemRepo != nil {
		if err := p.itemRepo.BatchUpdateStatus(updates); err != nil {
			logrus.WithFields(logrus.Fields{
				"module":  "migration",
				"task_id": p.taskID,
				"error":   err,
				"count":   len(updates),
			}).Error("Failed to batch update migration item status")
		}
	}
}

func (p *ProgressUpdater) Stop() {
	p.flush()
	p.updateTicker.Stop()
}
