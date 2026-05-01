package migration

import (
	"context"
	"encoding/json"
	"fmt"
	"sync"
	"time"

	"github.com/moonlight-box/registry/internal/model"
)

type MigrationWorker struct {
	service     *MigrationService
	concurrency int
}

func NewMigrationWorker(service *MigrationService, concurrency int) *MigrationWorker {
	return &MigrationWorker{
		service:     service,
		concurrency: concurrency,
	}
}

func (w *MigrationWorker) Execute(ctx context.Context, task *model.MigrationTask) error {
	ctx, cancel := context.WithCancel(ctx)
	defer cancel()

	w.service.RegisterContext(task.ID, ctx, cancel)

	client := NewNexusClient(task.SourceURL, task.Username, task.Password)
	w.service.RegisterNexusClient(task.ID, client)

	// 更新状态为 running
	now := time.Now()
	startedAt := &now
	w.service.db.Model(&model.MigrationTask{}).Where("id = ?", task.ID).Updates(map[string]interface{}{
		"status":     model.MigrationRunning,
		"started_at": startedAt,
	})

	// 解析选中的仓库
	var selectedRepos []string
	if err := json.Unmarshal([]byte(task.SelectedRepos), &selectedRepos); err != nil {
		return w.failTask(task.ID, fmt.Sprintf("failed to parse selected repos: %v", err))
	}

	semaphore := make(chan struct{}, w.concurrency)
	var wg sync.WaitGroup

	for _, repoName := range selectedRepos {
		select {
		case <-ctx.Done():
			return w.cancelTask(task.ID)
		default:
		}

		w.service.AddLog(task.ID, fmt.Sprintf("开始迁移仓库: %s", repoName))

		components, err := client.ListComponents(ctx, repoName)
		if err != nil {
			w.service.AddLog(task.ID, fmt.Sprintf("获取 %s 组件列表失败: %v", repoName, err))
			continue
		}

		w.updateTotal(task.ID, len(components))

		for _, comp := range components {
			wg.Add(1)
			semaphore <- struct{}{}

			go func(c NexusComponent) {
				defer func() { <-semaphore; wg.Done() }()

				select {
				case <-ctx.Done():
					return
				default:
				}

				if err := w.migrateComponent(task.ID, client, c); err != nil {
					w.service.AddLog(task.ID, fmt.Sprintf("迁移 %s 失败: %v", c.Name, err))
					w.incrementFailed(task.ID)
				} else {
					w.incrementProcessed(task.ID)
				}
			}(comp)
		}
	}

	// 等待所有 goroutine 完成
	wg.Wait()

	w.completeTask(task.ID)
	w.service.AddLog(task.ID, "迁移任务完成")
	return nil
}

func (w *MigrationWorker) migrateComponent(taskID uint, client *NexusClient, comp NexusComponent) error {
	for _, asset := range comp.Assets {
		if asset.DownloadURL == "" {
			continue
		}

		reader, contentType, size, err := client.DownloadAsset(context.Background(), asset.DownloadURL)
		if err != nil {
			return fmt.Errorf("download asset failed: %w", err)
		}
		defer reader.Close()

		_ = contentType
		_ = size
	}

	return nil
}

func (w *MigrationWorker) updateTotal(taskID uint, total int) {
	w.service.db.Model(&model.MigrationTask{}).Where("id = ?", taskID).Update("total_items", total)
}

func (w *MigrationWorker) incrementProcessed(taskID uint) {
	w.service.db.Model(&model.MigrationTask{}).Where("id = ?", taskID).
		Update("processed_items", w.service.db.Raw("processed_items + 1"))
}

func (w *MigrationWorker) incrementFailed(taskID uint) {
	w.service.db.Model(&model.MigrationTask{}).Where("id = ?", taskID).
		Update("failed_items", w.service.db.Raw("failed_items + 1"))
}

func (w *MigrationWorker) failTask(taskID uint, errMsg string) error {
	return w.service.db.Model(&model.MigrationTask{}).Where("id = ?", taskID).Updates(map[string]interface{}{
		"status":        model.MigrationFailed,
		"error_message": errMsg,
		"completed_at":  time.Now(),
	}).Error
}

func (w *MigrationWorker) cancelTask(taskID uint) error {
	return w.service.db.Model(&model.MigrationTask{}).Where("id = ?", taskID).Updates(map[string]interface{}{
		"status":       model.MigrationCancelled,
		"completed_at": time.Now(),
	}).Error
}

func (w *MigrationWorker) completeTask(taskID uint) {
	w.service.db.Model(&model.MigrationTask{}).Where("id = ?", taskID).Updates(map[string]interface{}{
		"status":       model.MigrationCompleted,
		"completed_at": time.Now(),
	})
}
