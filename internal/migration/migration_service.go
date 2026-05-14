package migration

import (
	"context"
	"encoding/json"
	"fmt"
	"sync"
	"time"

	"github.com/moonlight-box/registry/internal/model"
	"github.com/sirupsen/logrus"
	"gorm.io/gorm"
)

type MigrationWorkerInterface interface {
	Execute(ctx context.Context, task *model.MigrationTask) error
	RetryFailed(ctx context.Context, task *model.MigrationTask) error
}

type RepositoryCreator interface {
	CreateRepo(name, repoType, packageType string, remoteURL string, cacheEnabled bool, cacheTTLSeconds int, storageBackendID *uint) error
	RepoExists(name string) bool
	FindDefaultStorageBackendID() (*uint, error)
}

type MigrationService struct {
	db           *gorm.DB
	worker       MigrationWorkerInterface
	repoCreator  RepositoryCreator
	tasks        map[uint]*MigrationContext
	mu           sync.RWMutex
	nexusClients map[uint]*NexusClient

	// 任务队列相关
	queue         chan uint
	maxConcurrent int
	runningTasks  int
	queueMu       sync.Mutex
	queueStarted  bool

	// 重试任务标识
	retryTaskIDs map[uint]bool
	retryMu      sync.Mutex
}

type MigrationContext struct {
	Task     *model.MigrationTask
	Cancel   context.CancelFunc
	Progress *MigrationProgress
}

type MigrationProgress struct {
	Total     int
	Processed int
	Failed    int
	Logs      []string
	mu        sync.Mutex
}

const (
	// MaxConcurrentTasks 最大并发迁移任务数限制
	MaxConcurrentTasks = 5
)

func NewMigrationService(db *gorm.DB, worker MigrationWorkerInterface, repoCreator RepositoryCreator, maxConcurrent int) *MigrationService {
	if maxConcurrent <= 0 {
		maxConcurrent = 1
	}
	// 限制最大并发数，防止资源耗尽
	if maxConcurrent > MaxConcurrentTasks {
		maxConcurrent = MaxConcurrentTasks
	}
	s := &MigrationService{
		db:            db,
		worker:        worker,
		repoCreator:   repoCreator,
		tasks:         make(map[uint]*MigrationContext),
		nexusClients:  make(map[uint]*NexusClient),
		queue:         make(chan uint, 100),
		maxConcurrent: maxConcurrent,
		retryTaskIDs:  make(map[uint]bool),
	}
	return s
}

type QueueStatus struct {
	QueueLength   int `json:"queue_length"`
	RunningTasks  int `json:"running_tasks"`
	MaxConcurrent int `json:"max_concurrent"`
}

func (s *MigrationService) GetQueueStatus() QueueStatus {
	s.queueMu.Lock()
	defer s.queueMu.Unlock()

	return QueueStatus{
		QueueLength:   len(s.queue),
		RunningTasks:  s.runningTasks,
		MaxConcurrent: s.maxConcurrent,
	}
}

func (s *MigrationService) recoverInterruptedTasks() []uint {
	var interruptedTasks []model.MigrationTask
	if err := s.db.Where("status IN (?, ?, ?)", model.MigrationRunning, model.MigrationQueued, model.MigrationPending).Find(&interruptedTasks).Error; err != nil {
		logrus.WithError(err).Error("Failed to query interrupted migration tasks")
		return nil
	}

	if len(interruptedTasks) == 0 {
		return nil
	}

	logrus.WithField("module", "migration").Infof("Recovering %d interrupted migration tasks", len(interruptedTasks))

	var recoveredTaskIDs []uint
	for _, task := range interruptedTasks {
		s.resetProcessingItems(task.ID)

		if task.TaskType == model.MigrationTaskSyncConfig {
			s.db.Model(&model.MigrationTask{}).Where("id = ?", task.ID).Updates(map[string]interface{}{
				"status":        model.MigrationQueued,
				"started_at":    nil,
				"error_message": "",
			})
			recoveredTaskIDs = append(recoveredTaskIDs, task.ID)
			continue
		}

		switch task.Phase {
		case model.PhaseScanning:
			s.db.Model(&model.MigrationTask{}).Where("id = ?", task.ID).Updates(map[string]interface{}{
				"status":        model.MigrationQueued,
				"phase":         model.PhaseScanning,
				"started_at":    nil,
				"error_message": "",
			})

		case model.PhaseScanned, model.PhaseMigrating:
			s.db.Model(&model.MigrationTask{}).Where("id = ?", task.ID).Updates(map[string]interface{}{
				"status":        model.MigrationQueued,
				"phase":         model.PhaseMigrating,
				"started_at":    nil,
				"error_message": "",
			})

		default:
			s.db.Model(&model.MigrationTask{}).Where("id = ?", task.ID).Updates(map[string]interface{}{
				"status":        model.MigrationQueued,
				"phase":         model.PhaseScanning,
				"started_at":    nil,
				"error_message": "",
			})
		}

		logrus.WithFields(logrus.Fields{
			"module":     "migration",
			"task_id":    task.ID,
			"task_type":  task.TaskType,
			"phase":      task.Phase,
			"source_url": task.SourceURL,
		}).Info("Task recovered, will be requeued automatically")

		recoveredTaskIDs = append(recoveredTaskIDs, task.ID)
	}

	return recoveredTaskIDs
}

func (s *MigrationService) resetProcessingItems(taskID uint) {
	s.db.Model(&model.MigrationItem{}).
		Where("task_id = ? AND status = ?", taskID, model.MigrationItemProcessing).
		Updates(map[string]interface{}{
			"status":        model.MigrationItemPending,
			"error_message": "",
			"retry_count":   0,
		})
}

// StartQueue 启动任务队列处理器
func (s *MigrationService) StartQueue() {
	s.queueMu.Lock()
	if s.queueStarted {
		s.queueMu.Unlock()
		return
	}
	s.queueStarted = true
	s.queueMu.Unlock()

	go s.processQueue()
}

// StartQueueWithRecovery 启动任务队列处理器并自动恢复中断的任务
func (s *MigrationService) StartQueueWithRecovery() {
	// 先恢复中断的任务
	recoveredTaskIDs := s.recoverInterruptedTasks()

	// 启动队列处理器
	s.StartQueue()

	// 将恢复的任务自动入队
	if len(recoveredTaskIDs) > 0 {
		for _, taskID := range recoveredTaskIDs {
			if err := s.EnqueueTask(taskID); err != nil {
				logrus.WithFields(logrus.Fields{
					"module":  "migration",
					"task_id": taskID,
				}).Error("Failed to enqueue recovered task: " + err.Error())
			} else {
				logrus.WithField("module", "migration").Infof("Recovered task %d has been enqueued", taskID)
			}
		}
	}
}

// EnqueueTask 将任务 ID 加入队列
func (s *MigrationService) EnqueueTask(taskID uint) error {
	// 确保队列处理器已启动，避免任务长期停留在 queued
	s.StartQueue()

	select {
	case s.queue <- taskID:
		return nil
	default:
		return fmt.Errorf("任务队列已满")
	}
}

// processQueue 从队列中取出任务并执行
func (s *MigrationService) processQueue() {
	for taskID := range s.queue {
		// 等待有可用槽位
		s.queueMu.Lock()
		for s.runningTasks >= s.maxConcurrent {
			s.queueMu.Unlock()
			time.Sleep(1 * time.Second)
			s.queueMu.Lock()
		}
		s.runningTasks++
		s.queueMu.Unlock()

		// 启动任务
		go s.runQueuedTask(taskID)
	}
}

// runQueuedTask 执行队列中的单个任务
func (s *MigrationService) runQueuedTask(taskID uint) {
	defer func() {
		s.queueMu.Lock()
		s.runningTasks--
		s.queueMu.Unlock()
	}()

	task, err := s.GetTask(taskID)
	if err != nil {
		s.AddLog(taskID, "获取任务失败: "+err.Error())
		return
	}

	if task.Status == model.MigrationCancelled {
		s.AddLog(taskID, "任务已被取消")
		return
	}

	// 更新状态为 running
	now := time.Now()
	s.db.Model(&model.MigrationTask{}).Where("id = ?", taskID).Updates(map[string]interface{}{
		"status":     model.MigrationRunning,
		"started_at": now,
	})
	s.AddLog(taskID, fmt.Sprintf("任务开始执行（从队列）: %s", task.SourceURL))

	// 根据任务类型执行不同逻辑
	if task.TaskType == model.MigrationTaskSyncConfig {
		s.executeSyncConfigTask(taskID, task)
		return
	}

	// 完整迁移任务
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	if s.worker == nil {
		s.AddLog(taskID, "迁移 worker 未初始化")
		s.ClearRetryFlag(taskID)
		return
	}

	isRetry := s.IsRetryTask(taskID)
	defer s.ClearRetryFlag(taskID)

	var execErr error
	if isRetry {
		s.AddLog(taskID, "开始重试失败项目")
		execErr = s.worker.RetryFailed(ctx, task)
	} else {
		execErr = s.worker.Execute(ctx, task)
	}

	if execErr != nil {
		s.AddLog(taskID, "迁移执行出错: "+execErr.Error())
	}
}

func (s *MigrationService) CreateSyncConfigTask(sourceURL, username, password string, selectedRepos []string) (*model.MigrationTask, error) {
	reposJSON, _ := json.Marshal(selectedRepos)

	task := &model.MigrationTask{
		SourceType:    "nexus",
		SourceURL:     sourceURL,
		Username:      username,
		Status:        model.MigrationQueued,
		TaskType:      model.MigrationTaskSyncConfig,
		SelectedRepos: string(reposJSON),
	}

	if err := task.SetPassword(password); err != nil {
		return nil, err
	}

	if err := s.db.Create(task).Error; err != nil {
		return nil, err
	}

	// 加入队列
	if err := s.EnqueueTask(task.ID); err != nil {
		// 入队失败，将任务标记为失败，避免孤儿任务
		s.db.Model(&model.MigrationTask{}).Where("id = ?", task.ID).Updates(map[string]interface{}{
			"status":        model.MigrationFailed,
			"error_message": "任务入队失败: " + err.Error(),
		})
		return nil, err
	}

	return task, nil
}

func (s *MigrationService) executeSyncConfigTask(taskID uint, task *model.MigrationTask) {
	if s.repoCreator == nil {
		s.AddLog(taskID, "仓库创建器未初始化")
		s.db.Model(&model.MigrationTask{}).Where("id = ?", taskID).Updates(map[string]interface{}{
			"status":        model.MigrationFailed,
			"error_message": "仓库创建器未初始化",
			"completed_at":  time.Now(),
		})
		return
	}

	password, _ := task.GetPassword()
	client := NewNexusClient(task.SourceURL, task.Username, password)

	var selectedRepos []string
	if err := json.Unmarshal([]byte(task.SelectedRepos), &selectedRepos); err != nil {
		s.AddLog(taskID, "解析仓库列表失败: "+err.Error())
		s.db.Model(&model.MigrationTask{}).Where("id = ?", taskID).Updates(map[string]interface{}{
			"status":        model.MigrationFailed,
			"error_message": err.Error(),
			"completed_at":  time.Now(),
		})
		return
	}

	ctx := context.Background()
	nexusRepos, err := client.ListRepositories(ctx)
	if err != nil {
		s.AddLog(taskID, "获取仓库列表失败: "+err.Error())
		s.db.Model(&model.MigrationTask{}).Where("id = ?", taskID).Updates(map[string]interface{}{
			"status":        model.MigrationFailed,
			"error_message": err.Error(),
			"completed_at":  time.Now(),
		})
		return
	}

	defaultBackendID, _ := s.repoCreator.FindDefaultStorageBackendID()

	synced := 0
	skipped := 0
	for _, nr := range nexusRepos {
		if len(selectedRepos) > 0 {
			found := false
			for _, name := range selectedRepos {
				if nr.Name == name {
					found = true
					break
				}
			}
			if !found {
				continue
			}
		}

		if nr.Format == "" || nr.Type == "" {
			continue
		}

		if s.repoCreator.RepoExists(nr.Name) {
			s.AddLog(taskID, fmt.Sprintf("仓库已存在，跳过: %s", nr.Name))
			skipped++
			continue
		}

		repoType := s.mapRepoType(nr.Type)
		var remoteURL string
		cacheEnabled := false
		cacheTTL := 86400
		storageID := defaultBackendID

		if nr.Type == "proxy" {
			// 对于代理仓库，需要获取详细信息来获取真正的远程代理地址
			detail, err := client.GetRepositoryDetail(ctx, nr.Name)
			if err != nil {
				s.AddLog(taskID, fmt.Sprintf("获取仓库 %s 详细信息失败: %v", nr.Name, err))
				continue
			}
			if detail.Proxy != nil && detail.Proxy.RemoteURL != "" {
				remoteURL = detail.Proxy.RemoteURL
				cacheEnabled = true
			}
		}

		if nr.Type == "group" {
			cacheEnabled = false
			storageID = nil
		}

		if err := s.repoCreator.CreateRepo(nr.Name, repoType, nr.Format, remoteURL, cacheEnabled, cacheTTL, storageID); err != nil {
			s.AddLog(taskID, fmt.Sprintf("创建仓库失败: %s, 错误: %v", nr.Name, err))
			continue
		}

		s.AddLog(taskID, fmt.Sprintf("同步仓库: %s (%s/%s)", nr.Name, nr.Format, nr.Type))
		synced++
	}

	s.AddLog(taskID, fmt.Sprintf("成功同步 %d 个仓库配置，跳过 %d 个已存在的仓库", synced, skipped))
	s.db.Model(&model.MigrationTask{}).Where("id = ?", taskID).Updates(map[string]interface{}{
		"status":       model.MigrationCompleted,
		"completed_at": time.Now(),
	})
}

func (s *MigrationService) mapRepoType(nexusType string) string {
	switch nexusType {
	case "proxy":
		return "proxy"
	case "hosted":
		return "local"
	case "group":
		return "virtual"
	default:
		return "proxy"
	}
}

func (s *MigrationService) CreateTask(sourceURL, username, password string, selectedRepos []string, targetRepoID uint, targetRepoName string, workerCount, maxRetries, batchSize int) (*model.MigrationTask, error) {
	reposJSON, _ := json.Marshal(selectedRepos)

	if workerCount <= 0 {
		workerCount = 10
	}
	if maxRetries <= 0 {
		maxRetries = 3
	}
	if batchSize <= 0 {
		batchSize = 50
	}

	task := &model.MigrationTask{
		SourceType:         "nexus",
		SourceURL:          sourceURL,
		Username:           username,
		Status:             model.MigrationQueued,
		TaskType:           model.MigrationTaskFull,
		Phase:              model.PhaseScanning,
		SelectedRepos:      string(reposJSON),
		TargetRepositoryID: targetRepoID,
		TargetRepository:   targetRepoName,
		WorkerCount:        workerCount,
		MaxRetries:         maxRetries,
		BatchSize:          batchSize,
	}

	if err := task.SetPassword(password); err != nil {
		return nil, err
	}

	if err := s.db.Create(task).Error; err != nil {
		return nil, err
	}

	// 加入队列
	if err := s.EnqueueTask(task.ID); err != nil {
		// 与 CreateSyncConfigTask 行为对齐，避免遗留 queued 孤儿任务
		s.db.Model(&model.MigrationTask{}).Where("id = ?", task.ID).Updates(map[string]interface{}{
			"status":        model.MigrationFailed,
			"error_message": "任务入队失败: " + err.Error(),
		})
		return nil, err
	}

	return task, nil
}

func (s *MigrationService) GetTask(id uint) (*model.MigrationTask, error) {
	var task model.MigrationTask
	if err := s.db.First(&task, id).Error; err != nil {
		return nil, err
	}
	return &task, nil
}

func (s *MigrationService) ListTasks() ([]model.MigrationTask, error) {
	var tasks []model.MigrationTask
	err := s.db.Order("created_at DESC").Find(&tasks).Error
	return tasks, err
}

func (s *MigrationService) CancelTask(id uint) error {
	s.mu.RLock()
	ctx, ok := s.tasks[id]
	s.mu.RUnlock()

	if ok {
		ctx.Cancel()
	}

	task := &model.MigrationTask{Status: model.MigrationCancelled}
	return s.db.Model(&model.MigrationTask{}).Where("id = ?", id).Updates(task).Error
}

func (s *MigrationService) AddLog(taskID uint, message string) {
	s.mu.RLock()
	mc, ok := s.tasks[taskID]
	s.mu.RUnlock()

	if ok {
		mc.Progress.mu.Lock()
		mc.Progress.Logs = append(mc.Progress.Logs, message)
		mc.Progress.mu.Unlock()
	} else {
		logrus.WithFields(logrus.Fields{
			"module":  "migration",
			"task_id": taskID,
		}).Info(message)
	}
}

func (s *MigrationService) GetProgress(taskID uint) *MigrationProgress {
	s.mu.RLock()
	mc, ok := s.tasks[taskID]
	s.mu.RUnlock()

	if !ok {
		return nil
	}
	return mc.Progress
}

func (s *MigrationService) RegisterContext(taskID uint, ctx context.Context, cancel context.CancelFunc) {
	s.mu.Lock()
	s.tasks[taskID] = &MigrationContext{
		Cancel:   cancel,
		Progress: &MigrationProgress{},
	}
	s.mu.Unlock()
}

func (s *MigrationService) GetNexusClient(taskID uint) *NexusClient {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.nexusClients[taskID]
}

func (s *MigrationService) RegisterNexusClient(taskID uint, client *NexusClient) {
	s.mu.Lock()
	s.nexusClients[taskID] = client
	s.mu.Unlock()
}

func (s *MigrationService) RetryFailedTask(task *model.MigrationTask) {
	s.db.Model(&model.MigrationTask{}).Where("id = ?", task.ID).Updates(map[string]interface{}{
		"status":     model.MigrationQueued,
		"phase":      model.PhaseMigrating,
		"started_at": nil,
	})

	s.retryMu.Lock()
	s.retryTaskIDs[task.ID] = true
	s.retryMu.Unlock()

	if err := s.EnqueueTask(task.ID); err != nil {
		s.retryMu.Lock()
		delete(s.retryTaskIDs, task.ID)
		s.retryMu.Unlock()
		s.AddLog(task.ID, "重试任务入队失败: "+err.Error())
		return
	}

	s.AddLog(task.ID, "重试任务已加入队列")
}

func (s *MigrationService) StartTask(taskID uint) error {
	task, err := s.GetTask(taskID)
	if err != nil {
		return fmt.Errorf("任务不存在: %v", err)
	}

	if task.Status == model.MigrationRunning {
		return fmt.Errorf("任务正在运行中")
	}

	s.db.Model(&model.MigrationTask{}).Where("id = ?", taskID).Updates(map[string]interface{}{
		"status":     model.MigrationQueued,
		"started_at": nil,
	})

	if err := s.EnqueueTask(taskID); err != nil {
		s.AddLog(taskID, "任务入队失败: "+err.Error())
		return fmt.Errorf("任务入队失败: %v", err)
	}

	s.AddLog(taskID, "任务已加入队列")
	return nil
}

func (s *MigrationService) IsRetryTask(taskID uint) bool {
	s.retryMu.Lock()
	defer s.retryMu.Unlock()
	return s.retryTaskIDs[taskID]
}

func (s *MigrationService) ClearRetryFlag(taskID uint) {
	s.retryMu.Lock()
	delete(s.retryTaskIDs, taskID)
	s.retryMu.Unlock()
}

func (s *MigrationService) ListItems(taskID uint, page, pageSize int) ([]model.MigrationItem, int64, error) {
	var items []model.MigrationItem
	var total int64

	query := s.db.Model(&model.MigrationItem{}).Where("task_id = ?", taskID)

	if err := query.Count(&total).Error; err != nil {
		return nil, 0, err
	}

	if page > 0 && pageSize > 0 {
		offset := (page - 1) * pageSize
		query = query.Offset(offset).Limit(pageSize)
	}

	err := query.Order("created_at ASC").Find(&items).Error
	return items, total, err
}
