package repository

import (
	"github.com/moonlight-box/registry/internal/model"
	"gorm.io/gorm"
)

type VulnDataSourceRepository struct {
	db *gorm.DB
}

func NewVulnDataSourceRepository(db *gorm.DB) *VulnDataSourceRepository {
	return &VulnDataSourceRepository{db: db}
}

func (r *VulnDataSourceRepository) Create(ds *model.VulnDataSource) error {
	return r.db.Create(ds).Error
}

func (r *VulnDataSourceRepository) Update(id uint, updates map[string]interface{}) error {
	return r.db.Model(&model.VulnDataSource{}).Where("id = ?", id).Updates(updates).Error
}

func (r *VulnDataSourceRepository) Delete(id uint) error {
	return r.db.Delete(&model.VulnDataSource{}, id).Error
}

func (r *VulnDataSourceRepository) FindByID(id uint) (*model.VulnDataSource, error) {
	var ds model.VulnDataSource
	err := r.db.First(&ds, id).Error
	if err != nil {
		return nil, err
	}
	return &ds, nil
}

func (r *VulnDataSourceRepository) List() ([]model.VulnDataSource, error) {
	var dsList []model.VulnDataSource
	err := r.db.Order("created_at DESC").Find(&dsList).Error
	return dsList, err
}

func (r *VulnDataSourceRepository) ListEnabled() ([]model.VulnDataSource, error) {
	var dsList []model.VulnDataSource
	err := r.db.Where("enabled = ?", true).Find(&dsList).Error
	return dsList, err
}

func (r *VulnDataSourceRepository) UpdateSyncStatus(id uint, status, errMsg string) error {
	return r.db.Model(&model.VulnDataSource{}).Where("id = ?", id).Updates(map[string]interface{}{
		"last_status": status,
		"last_error":  errMsg,
	}).Error
}
