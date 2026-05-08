package repository

import (
	"github.com/moonlight-box/registry/internal/model"
	"gorm.io/gorm"
)

type BlockRuleRepository struct {
	db *gorm.DB
}

func NewBlockRuleRepository(db *gorm.DB) *BlockRuleRepository {
	return &BlockRuleRepository{db: db}
}

func (r *BlockRuleRepository) Create(rule *model.BlockRule) error {
	return r.db.Create(rule).Error
}

func (r *BlockRuleRepository) Update(id uint, updates map[string]interface{}) error {
	return r.db.Model(&model.BlockRule{}).Where("id = ?", id).Updates(updates).Error
}

func (r *BlockRuleRepository) Delete(id uint) error {
	return r.db.Delete(&model.BlockRule{}, id).Error
}

func (r *BlockRuleRepository) GetByID(id uint) (*model.BlockRule, error) {
	var rule model.BlockRule
	err := r.db.First(&rule, id).Error
	if err != nil {
		return nil, err
	}
	return &rule, nil
}

func (r *BlockRuleRepository) List(filter map[string]interface{}) ([]model.BlockRule, error) {
	var rules []model.BlockRule
	query := r.db.Model(&model.BlockRule{})

	if pkgName, ok := filter["package_name"]; ok {
		query = query.Where("package_name LIKE ?", pkgName)
	}
	if pkgType, ok := filter["package_type"]; ok {
		query = query.Where("package_type = ?", pkgType)
	}
	if enabled, ok := filter["enabled"]; ok {
		query = query.Where("enabled = ?", enabled)
	}

	err := query.Order("created_at DESC").Find(&rules).Error
	return rules, err
}

func (r *BlockRuleRepository) ListWithPage(page, pageSize int, filter map[string]interface{}) ([]model.BlockRule, int64, error) {
	var rules []model.BlockRule
	var total int64
	query := r.db.Model(&model.BlockRule{})

	if pkgName, ok := filter["package_name"]; ok {
		query = query.Where("package_name LIKE ?", pkgName)
	}
	if pkgType, ok := filter["package_type"]; ok {
		query = query.Where("package_type = ?", pkgType)
	}
	if enabled, ok := filter["enabled"]; ok {
		query = query.Where("enabled = ?", enabled)
	}

	err := query.Count(&total).Error
	if err != nil {
		return nil, 0, err
	}

	err = query.Offset((page - 1) * pageSize).Limit(pageSize).
		Order("created_at DESC").
		Find(&rules).Error

	return rules, total, err
}

func (r *BlockRuleRepository) FindEnabledByPackageType(pkgType string) ([]model.BlockRule, error) {
	var rules []model.BlockRule
	err := r.db.Where("package_type = ? AND enabled = ?", pkgType, true).Find(&rules).Error
	return rules, err
}

func (r *BlockRuleRepository) FindEnabledExactRules(pkgType, pkgName, version string) ([]model.BlockRule, error) {
	var rules []model.BlockRule
	err := r.db.Where("package_type = ? AND enabled = ? AND match_type = ? AND package_name = ? AND version = ?",
		pkgType, true, model.BlockMatchExact, pkgName, version).Find(&rules).Error
	return rules, err
}

func (r *BlockRuleRepository) FindEnabledWildcardRules(pkgType string) ([]model.BlockRule, error) {
	var rules []model.BlockRule
	err := r.db.Where("package_type = ? AND enabled = ? AND match_type = ?",
		pkgType, true, model.BlockMatchWildcard).Find(&rules).Error
	return rules, err
}

func (r *BlockRuleRepository) FindAllEnabledExactRules() ([]model.BlockRule, error) {
	var rules []model.BlockRule
	err := r.db.Where("enabled = ? AND match_type = ?", true, model.BlockMatchExact).Find(&rules).Error
	return rules, err
}

func (r *BlockRuleRepository) FindAllEnabledWildcardRules() ([]model.BlockRule, error) {
	var rules []model.BlockRule
	err := r.db.Where("enabled = ? AND match_type = ?", true, model.BlockMatchWildcard).Find(&rules).Error
	return rules, err
}
