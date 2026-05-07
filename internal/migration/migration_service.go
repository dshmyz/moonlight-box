package migration

import (
	"context"
	"encoding/json"
	"fmt"
	"sync"

	"github.com/moonlight-box/registry/internal/model"
	"gorm.io/gorm"
)

type MigrationService struct {
	db           *gorm.DB
	tasks        map[uint]*MigrationContext
	mu           sync.RWMutex
	nexusClients map[uint]*NexusClient
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

func NewMigrationService(db *gorm.DB) *MigrationService {
	return &MigrationService{
		db:           db,
		tasks:        make(map[uint]*MigrationContext),
		nexusClients: make(map[uint]*NexusClient),
	}
}

func (s *MigrationService) CreateTask(sourceURL, username, password string, selectedRepos []string, targetRepoID uint, targetRepoName string) (*model.MigrationTask, error) {
	reposJSON, _ := json.Marshal(selectedRepos)

	task := &model.MigrationTask{
		SourceType:         "nexus",
		SourceURL:          sourceURL,
		Username:           username,
		Password:           password,
		Status:             model.MigrationPending,
		SelectedRepos:      string(reposJSON),
		TargetRepositoryID: targetRepoID,
		TargetRepository:   targetRepoName,
	}

	if err := s.db.Create(task).Error; err != nil {
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

	if !ok {
		return fmt.Errorf("task not found")
	}

	ctx.Cancel()

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
