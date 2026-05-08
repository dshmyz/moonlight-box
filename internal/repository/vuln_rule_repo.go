package repository

import (
	"github.com/moonlight-box/registry/internal/model"
	"gorm.io/gorm"
)

type VulnRuleRepository struct {
	db *gorm.DB
}

func NewVulnRuleRepository(db *gorm.DB) *VulnRuleRepository {
	return &VulnRuleRepository{db: db}
}

func (r *VulnRuleRepository) Create(rule *model.VulnRule) error {
	return r.db.Create(rule).Error
}

func (r *VulnRuleRepository) Update(id uint, updates map[string]interface{}) error {
	return r.db.Model(&model.VulnRule{}).Where("id = ?", id).Updates(updates).Error
}

func (r *VulnRuleRepository) Delete(id uint) error {
	return r.db.Delete(&model.VulnRule{}, id).Error
}

func (r *VulnRuleRepository) FindByID(id uint) (*model.VulnRule, error) {
	var rule model.VulnRule
	err := r.db.First(&rule, id).Error
	if err != nil {
		return nil, err
	}
	return &rule, nil
}

func (r *VulnRuleRepository) List(page, pageSize int, source, severity, pkgType, keyword string) ([]model.VulnRule, int64, error) {
	var rules []model.VulnRule
	var total int64

	query := r.db.Model(&model.VulnRule{})

	if source != "" {
		query = query.Where("source = ?", source)
	}
	if severity != "" {
		query = query.Where("severity = ?", severity)
	}
	if pkgType != "" {
		query = query.Where("package_type = ?", pkgType)
	}
	if keyword != "" {
		query = query.Where("package_pattern LIKE ? OR cve LIKE ? OR title LIKE ?", "%"+keyword+"%", "%"+keyword+"%", "%"+keyword+"%")
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

func (r *VulnRuleRepository) ListAllEnabled() ([]model.VulnRule, error) {
	var rules []model.VulnRule
	err := r.db.Where("enabled = ?", true).Find(&rules).Error
	return rules, err
}

func (r *VulnRuleRepository) UpsertByCVE(cve string, rule *model.VulnRule) error {
	return r.db.Where("cve = ?", cve).
		Assign(rule).
		FirstOrCreate(rule).Error
}

func (r *VulnRuleRepository) DeleteBySourceAndExternalIDs(source model.VulnRuleSource, externalIDs []string) error {
	if len(externalIDs) == 0 {
		return nil
	}
	return r.db.Where("source = ? AND external_id NOT IN ?", source, externalIDs).Delete(&model.VulnRule{}).Error
}
