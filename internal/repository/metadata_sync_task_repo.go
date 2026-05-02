package repository

import (
	"github.com/moonlight-box/registry/internal/model"
	"gorm.io/gorm"
)

// MetadataSyncTaskRepository 元数据同步任务数据访问层
type MetadataSyncTaskRepository struct {
	db *gorm.DB
}

func NewMetadataSyncTaskRepository(db *gorm.DB) *MetadataSyncTaskRepository {
	return &MetadataSyncTaskRepository{db: db}
}

// Create 创建同步任务
func (r *MetadataSyncTaskRepository) Create(task *model.MetadataSyncTask) error {
	return r.db.Create(task).Error
}

// GetByID 根据ID获取同步任务
func (r *MetadataSyncTaskRepository) GetByID(id uint) (*model.MetadataSyncTask, error) {
	var task model.MetadataSyncTask
	err := r.db.First(&task, id).Error
	return &task, err
}

// Update 更新同步任务
func (r *MetadataSyncTaskRepository) Update(task *model.MetadataSyncTask) error {
	return r.db.Save(task).Error
}

// GetByRepositoryID 根据仓库ID获取同步任务列表
func (r *MetadataSyncTaskRepository) GetByRepositoryID(repoID uint, limit int) ([]model.MetadataSyncTask, error) {
	var tasks []model.MetadataSyncTask
	err := r.db.Where("repository_id = ?", repoID).
		Order("created_at DESC").
		Limit(limit).
		Find(&tasks).Error
	return tasks, err
}

// GetRunningTaskByRepoID 获取指定仓库正在运行的同步任务
func (r *MetadataSyncTaskRepository) GetRunningTaskByRepoID(repoID uint) (*model.MetadataSyncTask, error) {
	var task model.MetadataSyncTask
	err := r.db.Where("repository_id = ? AND status = ?", repoID, "running").First(&task).Error
	return &task, err
}
