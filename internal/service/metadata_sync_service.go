package service

import (
	"context"
	"fmt"
	"sync"
	"time"

	"github.com/moonlight-box/registry/internal/model"
	"github.com/moonlight-box/registry/internal/repository"
	"github.com/moonlight-box/registry/internal/types"
	"github.com/sirupsen/logrus"
	"gorm.io/gorm"
)

// MetadataSyncService 元数据同步服务
type MetadataSyncService struct {
	db          *gorm.DB
	taskRepo    *repository.MetadataSyncTaskRepository
	repoRepo    *repository.RepositoryRepository
	pkgRepo     *repository.PackageRepository
	adapters    map[string]types.MetadataSyncer
	runningTask map[uint]context.CancelFunc
	mu          sync.RWMutex
}

// NewMetadataSyncService 创建元数据同步服务
func NewMetadataSyncService(
	db *gorm.DB,
	taskRepo *repository.MetadataSyncTaskRepository,
	repoRepo *repository.RepositoryRepository,
	pkgRepo *repository.PackageRepository,
) *MetadataSyncService {
	return &MetadataSyncService{
		db:          db,
		taskRepo:    taskRepo,
		repoRepo:    repoRepo,
		pkgRepo:     pkgRepo,
		adapters:    make(map[string]types.MetadataSyncer),
		runningTask: make(map[uint]context.CancelFunc),
	}
}

// RegisterAdapter 注册适配器
func (s *MetadataSyncService) RegisterAdapter(pkgType string, syncer types.MetadataSyncer) {
	s.adapters[pkgType] = syncer
}

// TriggerManualSync 手动触发同步
func (s *MetadataSyncService) TriggerManualSync(repoID uint, userID uint) (*model.MetadataSyncTask, error) {
	repo, err := s.repoRepo.FindByID(repoID)
	if err != nil {
		return nil, err
	}

	if repo.Type != model.RepoTypeProxy {
		return nil, fmt.Errorf("only proxy repository supports metadata sync")
	}

	runningTask, _ := s.taskRepo.GetRunningTaskByRepoID(repoID)
	if runningTask != nil {
		return nil, fmt.Errorf("a sync task is already running for this repository")
	}

	now := time.Now()
	task := &model.MetadataSyncTask{
		RepositoryID: repoID,
		Status:       "pending",
		TriggerType:  "manual",
		TriggeredBy:  &userID,
		StartedAt:    &now,
	}

	if err := s.taskRepo.Create(task); err != nil {
		return nil, err
	}

	go s.executeSync(task.ID, repo)

	return task, nil
}

// executeSync 执行同步
func (s *MetadataSyncService) executeSync(taskID uint, repo *model.Repository) {
	ctx, cancel := context.WithCancel(context.Background())

	s.mu.Lock()
	s.runningTask[taskID] = cancel
	s.mu.Unlock()

	defer func() {
		s.mu.Lock()
		delete(s.runningTask, taskID)
		s.mu.Unlock()
	}()

	task, _ := s.taskRepo.GetByID(taskID)
	task.Status = "running"
	s.taskRepo.Update(task)

	syncer, ok := s.adapters[repo.PackageType]
	if !ok {
		s.failTask(task, fmt.Sprintf("unsupported package type: %s", repo.PackageType))
		return
	}

	result, err := syncer.SyncMetadata(ctx, repo)
	if err != nil {
		s.failTask(task, err.Error())
		return
	}

	now := time.Now()
	task.Status = "completed"
	task.CompletedAt = &now
	task.TotalPackages = result.Total
	task.SyncedPackages = result.Synced
	task.FailedPackages = result.Failed
	task.SkippedPackages = result.Skipped
	s.taskRepo.Update(task)

	s.repoRepo.Update(repo.Name, map[string]interface{}{
		"last_metadata_sync_at": &now,
		"last_sync_status":      "success",
		"last_sync_error":       "",
	})

	logrus.WithFields(logrus.Fields{
		"task_id": taskID,
		"repo_id": repo.ID,
		"total":   result.Total,
		"synced":  result.Synced,
		"failed":  result.Failed,
	}).Info("Metadata sync completed")
}

// failTask 标记任务失败
func (s *MetadataSyncService) failTask(task *model.MetadataSyncTask, errMsg string) {
	now := time.Now()
	task.Status = "failed"
	task.CompletedAt = &now
	task.ErrorMessage = errMsg
	s.taskRepo.Update(task)

	s.repoRepo.Update(task.Repository.Name, map[string]interface{}{
		"last_sync_status": "failed",
		"last_sync_error":  errMsg,
	})

	logrus.WithFields(logrus.Fields{
		"task_id": task.ID,
		"error":   errMsg,
	}).Error("Metadata sync failed")
}

// GetTaskStatus 获取任务状态
func (s *MetadataSyncService) GetTaskStatus(taskID uint) (*model.MetadataSyncTask, error) {
	return s.taskRepo.GetByID(taskID)
}

// GetRepositorySyncHistory 获取仓库同步历史
func (s *MetadataSyncService) GetRepositorySyncHistory(repoID uint, limit int) ([]model.MetadataSyncTask, error) {
	if limit <= 0 {
		limit = 20
	}
	return s.taskRepo.GetByRepositoryID(repoID, limit)
}

// CancelTask 取消任务
func (s *MetadataSyncService) CancelTask(taskID uint) error {
	s.mu.RLock()
	cancel, ok := s.runningTask[taskID]
	s.mu.RUnlock()

	if !ok {
		return fmt.Errorf("task not running")
	}

	cancel()

	task, err := s.taskRepo.GetByID(taskID)
	if err != nil {
		return err
	}

	now := time.Now()
	task.Status = "cancelled"
	task.CompletedAt = &now
	return s.taskRepo.Update(task)
}

// SyncRepositoryMetadata 同步仓库元数据（用于定时任务）
func (s *MetadataSyncService) SyncRepositoryMetadata(repoID uint) error {
	repo, err := s.repoRepo.FindByID(repoID)
	if err != nil {
		return err
	}

	if !repo.MetadataSyncEnabled {
		return nil
	}

	runningTask, _ := s.taskRepo.GetRunningTaskByRepoID(repoID)
	if runningTask != nil {
		return fmt.Errorf("a sync task is already running")
	}

	now := time.Now()
	task := &model.MetadataSyncTask{
		RepositoryID: repoID,
		Status:       "pending",
		TriggerType:  "scheduled",
		StartedAt:    &now,
	}

	if err := s.taskRepo.Create(task); err != nil {
		return err
	}

	go s.executeSync(task.ID, repo)

	return nil
}
