package repository

import (
	"time"

	"github.com/dshmyz/moonlight-box/internal/model"
	"gorm.io/gorm"
)

type AuditRepository struct {
	db *gorm.DB
}

func NewAuditRepository(db *gorm.DB) *AuditRepository {
	return &AuditRepository{db: db}
}

func (r *AuditRepository) Create(log *model.AuditLog) error {
	return r.db.Create(log).Error
}

func (r *AuditRepository) List(page, pageSize int, ipAddress, resourceType string, action *model.AuditAction) ([]model.AuditLog, int64, error) {
	var logs []model.AuditLog
	var total int64

	query := r.db.Model(&model.AuditLog{})

	if ipAddress != "" {
		query = query.Where("ip_address = ?", ipAddress)
	}
	if resourceType != "" {
		query = query.Where("resource_type = ?", resourceType)
	}
	if action != nil {
		query = query.Where("action = ?", *action)
	}

	query.Count(&total)

	offset := (page - 1) * pageSize
	result := query.Offset(offset).Limit(pageSize).Order("created_at DESC").Find(&logs)

	return logs, total, result.Error
}

func (r *AuditRepository) GetByID(id uint) (*model.AuditLog, error) {
	var log model.AuditLog
	err := r.db.First(&log, id).Error
	if err != nil {
		return nil, err
	}
	return &log, nil
}

func (r *AuditRepository) CleanOldLogs(maxAge time.Duration) error {
	cutoff := time.Now().Add(-maxAge)
	return r.db.Where("created_at < ?", cutoff).Delete(&model.AuditLog{}).Error
}

// GetLastAIToolLog 获取最后一条 AI 相关审计日志（工具调用或提示词变更），用于哈希链头。
// 表为空时 out.ID == 0 且返回 nil（Find 不产生 ErrRecordNotFound 日志噪音）。
func (r *AuditRepository) GetLastAIToolLog(out *model.AuditLog) error {
	return r.db.Model(&model.AuditLog{}).
		Where("action IN ?", []model.AuditAction{model.ActionAIToolCall, model.ActionAIPromptChange}).
		Order("id DESC").
		Limit(1).
		Find(out).Error
}

// CleanOldAIAndToolLogs 按保留策略清理过期 AI 工具审计日志。
func (r *AuditRepository) CleanOldAIAndToolLogs(maxAge time.Duration) error {
	cutoff := time.Now().Add(-maxAge)
	return r.db.Where("action IN ? AND created_at < ?",
		[]model.AuditAction{model.ActionAIToolCall, model.ActionAIPromptChange}, cutoff).
		Delete(&model.AuditLog{}).Error
}

// ListAIAndToolLogs 按条件过滤查询 AI 工具审计日志。
func (r *AuditRepository) ListAIAndToolLogs(toolName, username string, success *bool, startTime, endTime *time.Time, limit, offset int) ([]model.AuditLog, int64, error) {
	var logs []model.AuditLog
	var total int64

	query := r.db.Model(&model.AuditLog{}).
		Where("action IN ?", []model.AuditAction{model.ActionAIToolCall, model.ActionAIPromptChange})

	if toolName != "" {
		query = query.Where("tool_name = ?", toolName)
	}
	if username != "" {
		query = query.Where("details LIKE ?", "%"+username+"%")
	}
	if success != nil {
		if *success {
			query = query.Where("response_status != 500 OR response_status IS NULL")
		} else {
			query = query.Where("response_status = 500")
		}
	}
	if startTime != nil {
		query = query.Where("created_at >= ?", *startTime)
	}
	if endTime != nil {
		query = query.Where("created_at <= ?", *endTime)
	}

	query.Count(&total)

	if limit <= 0 {
		limit = 100
	}
	result := query.Offset(offset).Limit(limit).Order("id DESC").Find(&logs)
	return logs, total, result.Error
}

// VerifyAIChain 校验 AI 审计日志哈希链（仅 AI 工具调用/提示词变更日志，详见 model.VerifyAuditChain）。
// 返回被篡改的日志 ID 列表，nil 表示链路完整。
func (r *AuditRepository) VerifyAIChain(earliestID uint) ([]uint, error) {
	return model.VerifyAuditChain(r.db, earliestID)
}

type BlockStats struct {
	TotalBlocks    int64             `json:"total_blocks"`
	UniquePackages int64             `json:"unique_packages"`
	TopBlockedPkgs []TopBlockedItem  `json:"top_blocked_pkgs"`
	BlocksByHour   []HourlyBlockItem `json:"blocks_by_hour"`
	UniqueIPs      int64             `json:"unique_ips"`
	TopIPs         []TopIPItem       `json:"top_ips"`
}

type TopBlockedItem struct {
	ResourceName string `json:"resource_name"`
	Count        int64  `json:"count"`
}

type HourlyBlockItem struct {
	Hour  string `json:"hour"`
	Count int64  `json:"count"`
}

type TopIPItem struct {
	IPAddress string `json:"ip_address"`
	Count     int64  `json:"count"`
}

func (r *AuditRepository) GetBlockStats(hours int) (*BlockStats, error) {
	stats := &BlockStats{}

	cutoff := time.Now().Add(-time.Duration(hours) * time.Hour)

	baseQuery := r.db.Model(&model.AuditLog{}).
		Where("action = ? AND created_at >= ?", model.ActionBlock, cutoff)

	baseQuery.Count(&stats.TotalBlocks)

	r.db.Model(&model.AuditLog{}).
		Where("action = ? AND created_at >= ?", model.ActionBlock, cutoff).
		Distinct("resource_name").
		Count(&stats.UniquePackages)

	r.db.Model(&model.AuditLog{}).
		Where("action = ? AND created_at >= ?", model.ActionBlock, cutoff).
		Distinct("ip_address").
		Count(&stats.UniqueIPs)

	var topPkgs []TopBlockedItem
	baseQuery.Select("resource_name, COUNT(*) as count").
		Group("resource_name").
		Order("count DESC").
		Limit(10).
		Find(&topPkgs)
	stats.TopBlockedPkgs = topPkgs

	var hourlyBlocks []HourlyBlockItem
	baseQuery.Select("DATE_FORMAT(created_at, '%Y-%m-%d %H:00:00') as hour, COUNT(*) as count").
		Group("hour").
		Order("hour ASC").
		Find(&hourlyBlocks)
	stats.BlocksByHour = hourlyBlocks

	var topIPs []TopIPItem
	baseQuery.Select("ip_address, COUNT(*) as count").
		Where("ip_address != ''").
		Group("ip_address").
		Order("count DESC").
		Limit(10).
		Find(&topIPs)
	stats.TopIPs = topIPs

	return stats, nil
}
