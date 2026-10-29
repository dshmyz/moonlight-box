package repository

import (
	"github.com/dshmyz/moonlight-box/internal/model"
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
	query := applyBlockRuleFilters(r.db.Model(&model.BlockRule{}), filter)

	err := query.Order("created_at DESC").Find(&rules).Error
	return rules, err
}

func (r *BlockRuleRepository) ListWithPage(page, pageSize int, filter map[string]interface{}) ([]model.BlockRule, int64, error) {
	var rules []model.BlockRule
	var total int64
	query := applyBlockRuleFilters(r.db.Model(&model.BlockRule{}), filter)

	err := query.Count(&total).Error
	if err != nil {
		return nil, 0, err
	}

	err = query.Offset((page - 1) * pageSize).Limit(pageSize).
		Order("created_at DESC").
		Find(&rules).Error

	return rules, total, err
}

// applyBlockRuleFilters 应用列表过滤条件。package_name/reason/keyword 由调用方传入带 % 的 LIKE 值。
func applyBlockRuleFilters(query *gorm.DB, filter map[string]interface{}) *gorm.DB {
	if pkgName, ok := filter["package_name"]; ok {
		query = query.Where("package_name LIKE ?", pkgName)
	}
	if pkgType, ok := filter["package_type"]; ok {
		query = query.Where("package_type = ?", pkgType)
	}
	if enabled, ok := filter["enabled"]; ok {
		query = query.Where("enabled = ?", enabled)
	}
	if matchType, ok := filter["match_type"]; ok {
		query = query.Where("match_type = ?", matchType)
	}
	if source, ok := filter["source"]; ok {
		query = query.Where("source = ?", source)
	}
	if reason, ok := filter["reason"]; ok {
		query = query.Where("reason LIKE ?", reason)
	}
	if keyword, ok := filter["keyword"]; ok {
		// 显式加括号：GORM 不会给裸 SQL 片段补括号，OR 会与其他条件按优先级错位组合
		query = query.Where("(package_name LIKE ? OR reason LIKE ?)", keyword, keyword)
	}
	return query
}

func (r *BlockRuleRepository) BulkUpdateEnabled(ids []uint, enabled bool) (int64, error) {
	if len(ids) == 0 {
		return 0, nil
	}
	result := r.db.Model(&model.BlockRule{}).Where("id IN ?", ids).Update("enabled", enabled)
	return result.RowsAffected, result.Error
}

func (r *BlockRuleRepository) BulkDelete(ids []uint) (int64, error) {
	if len(ids) == 0 {
		return 0, nil
	}
	result := r.db.Where("id IN ?", ids).Delete(&model.BlockRule{})
	return result.RowsAffected, result.Error
}

// FindAllEnabledExactRules 查询所有启用的精确匹配规则，排除带条件的规则
// （ConditionType 为空或 NULL 的才算无条件规则）
func (r *BlockRuleRepository) FindAllEnabledExactRules() ([]model.BlockRule, error) {
	var rules []model.BlockRule
	err := r.db.Where("enabled = ? AND match_type = ? AND (condition_type = '' OR condition_type IS NULL)",
		true, model.BlockMatchExact).Find(&rules).Error
	return rules, err
}

// FindAllEnabledWildcardRules 查询所有启用的通配符匹配规则，排除带条件的规则
// （ConditionType 为空或 NULL 的才算无条件规则）
func (r *BlockRuleRepository) FindAllEnabledWildcardRules() ([]model.BlockRule, error) {
	var rules []model.BlockRule
	err := r.db.Where("enabled = ? AND match_type = ? AND (condition_type = '' OR condition_type IS NULL)",
		true, model.BlockMatchWildcard).Find(&rules).Error
	return rules, err
}

// FindAllEnabledRangeRules 查询所有启用的版本范围匹配规则，排除带条件的规则
// （ConditionType 为空或 NULL 的才算无条件规则）
func (r *BlockRuleRepository) FindAllEnabledRangeRules() ([]model.BlockRule, error) {
	var rules []model.BlockRule
	err := r.db.Where("enabled = ? AND match_type = ? AND (condition_type = '' OR condition_type IS NULL)",
		true, model.BlockMatchRange).Find(&rules).Error
	return rules, err
}

// FindAllEnabledConditionalRules 查询所有启用且带条件（ConditionType != ""）的规则
func (r *BlockRuleRepository) FindAllEnabledConditionalRules() ([]model.BlockRule, error) {
	var rules []model.BlockRule
	err := r.db.Where("enabled = ? AND condition_type != ''",
		true).Find(&rules).Error
	return rules, err
}
