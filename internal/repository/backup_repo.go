package repository

import (
	"github.com/moonlight-box/registry/internal/model"
	"gorm.io/gorm"
)

type BackupRepository struct {
	db *gorm.DB
}

func NewBackupRepository(db *gorm.DB) *BackupRepository {
	return &BackupRepository{db: db}
}

func (r *BackupRepository) Create(backup *model.Backup) error {
	return r.db.Create(backup).Error
}

func (r *BackupRepository) Update(backup *model.Backup) error {
	return r.db.Save(backup).Error
}

func (r *BackupRepository) Delete(id uint) error {
	return r.db.Delete(&model.Backup{}, id).Error
}

func (r *BackupRepository) GetByID(id uint) (*model.Backup, error) {
	var backup model.Backup
	err := r.db.First(&backup, id).Error
	if err != nil {
		return nil, err
	}
	return &backup, nil
}

func (r *BackupRepository) GetByName(name string) (*model.Backup, error) {
	var backup model.Backup
	err := r.db.Where("name = ?", name).First(&backup).Error
	if err != nil {
		return nil, err
	}
	return &backup, nil
}

func (r *BackupRepository) List(page, pageSize int) ([]model.Backup, int64, error) {
	var backups []model.Backup
	var total int64

	offset := (page - 1) * pageSize

	if err := r.db.Model(&model.Backup{}).Count(&total).Error; err != nil {
		return nil, 0, err
	}

	err := r.db.Order("created_at DESC").
		Offset(offset).
		Limit(pageSize).
		Find(&backups).Error

	return backups, total, err
}

func (r *BackupRepository) ListByStatus(status model.BackupStatus) ([]model.Backup, error) {
	var backups []model.Backup
	err := r.db.Where("status = ?", status).Order("created_at DESC").Find(&backups).Error
	return backups, err
}
