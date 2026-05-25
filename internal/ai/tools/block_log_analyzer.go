package tools

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"sync"
	"time"

	"github.com/dshmyz/moonlight-box/internal/repository"
)

type BlockLogAnalyzerTool struct {
	BaseTool
	auditRepo *repository.AuditRepository
	cache     *sync.Map
	cacheTTL  time.Duration
}

type cacheEntry struct {
	result    string
	expiresAt time.Time
}

func NewBlockLogAnalyzerTool(auditRepo *repository.AuditRepository) *BlockLogAnalyzerTool {
	return &BlockLogAnalyzerTool{
		auditRepo: auditRepo,
		cache:     &sync.Map{},
		cacheTTL:  5 * time.Minute,
	}
}

func (t *BlockLogAnalyzerTool) Name() string {
	return "block_log_analyzer"
}

func (t *BlockLogAnalyzerTool) Description() string {
	return "分析阻断日志，检测异常模式（如高频阻断、集中IP来源、异常时间段等）"
}

func (t *BlockLogAnalyzerTool) Parameters() json.RawMessage {
	return json.RawMessage(`{
		"type": "object",
		"properties": {
			"analysis_type": {
				"type": "string",
				"description": "分析类型",
				"enum": ["overview", "anomaly_detection", "ip_analysis", "time_pattern", "package_analysis"]
			},
			"hours": {
				"type": "integer",
				"description": "分析时间范围（小时），默认24小时",
				"default": 24
			},
			"threshold": {
				"type": "integer",
				"description": "异常检测阈值（阻断次数），默认50",
				"default": 50
			}
		},
		"required": ["analysis_type"]
	}`)
}

func (t *BlockLogAnalyzerTool) Execute(ctx context.Context, params map[string]interface{}) (string, error) {
	analysisType, ok := params["analysis_type"].(string)
	if !ok {
		return "", fmt.Errorf("缺少必需参数: analysis_type")
	}

	hours := 24
	if h, ok := params["hours"].(float64); ok {
		hours = int(h)
	}

	threshold := 50
	if th, ok := params["threshold"].(float64); ok {
		threshold = int(th)
	}

	// 构建缓存 key
	cacheKey := t.buildCacheKey(analysisType, hours)

	// 检查缓存
	if entry, ok := t.cache.Load(cacheKey); ok {
		cacheEntry := entry.(*cacheEntry)
		if time.Now().Before(cacheEntry.expiresAt) {
			return cacheEntry.result, nil
		}
		// 缓存过期，删除
		t.cache.Delete(cacheKey)
	}

	// 执行查询
	var result string
	var err error
	switch analysisType {
	case "overview":
		result, err = t.analyzeOverview(hours)
	case "anomaly_detection":
		result, err = t.detectAnomalies(hours, threshold)
	case "ip_analysis":
		result, err = t.analyzeIPPattern(hours)
	case "time_pattern":
		result, err = t.analyzeTimePattern(hours)
	case "package_analysis":
		result, err = t.analyzePackagePattern(hours)
	default:
		return "", fmt.Errorf("不支持的分析类型: %s", analysisType)
	}

	if err != nil {
		return "", err
	}

	// 写入缓存
	t.cache.Store(cacheKey, &cacheEntry{
		result:    result,
		expiresAt: time.Now().Add(t.cacheTTL),
	})

	return result, nil
}

func (t *BlockLogAnalyzerTool) buildCacheKey(analysisType string, hours int) string {
	return fmt.Sprintf("block_log:%s:%d", analysisType, hours)
}

// ClearCache 清理所有缓存（用于新增阻断记录时调用）
func (t *BlockLogAnalyzerTool) ClearCache() {
	t.cache.Range(func(key, value interface{}) bool {
		t.cache.Delete(key)
		return true
	})
}

func (t *BlockLogAnalyzerTool) analyzeOverview(hours int) (string, error) {
	stats, err := t.auditRepo.GetBlockStats(hours)
	if err != nil {
		return "", fmt.Errorf("获取阻断统计失败: %v", err)
	}

	var sb strings.Builder
	sb.WriteString(fmt.Sprintf("📊 阻断日志概览（过去 %d 小时）\n\n", hours))

	sb.WriteString(fmt.Sprintf("🔢 总阻断次数: **%d**\n", stats.TotalBlocks))
	sb.WriteString(fmt.Sprintf("📦 阻断的唯一包数: **%d**\n", stats.UniquePackages))
	sb.WriteString(fmt.Sprintf("🌐 阻断的唯一IP数: **%d**\n\n", stats.UniqueIPs))

	if len(stats.TopBlockedPkgs) > 0 {
		sb.WriteString("🔝 被阻断最多的包（Top 10）:\n\n")
		for i, pkg := range stats.TopBlockedPkgs {
			sb.WriteString(fmt.Sprintf("%d. **%s** - %d 次\n", i+1, pkg.ResourceName, pkg.Count))
		}
		sb.WriteString("\n")
	}

	if len(stats.TopIPs) > 0 {
		sb.WriteString("🔝 阻断次数最多的IP（Top 10）:\n\n")
		for i, ip := range stats.TopIPs {
			sb.WriteString(fmt.Sprintf("%d. **%s** - %d 次\n", i+1, ip.IPAddress, ip.Count))
		}
		sb.WriteString("\n")
	}

	return sb.String(), nil
}

func (t *BlockLogAnalyzerTool) detectAnomalies(hours int, threshold int) (string, error) {
	stats, err := t.auditRepo.GetBlockStats(hours)
	if err != nil {
		return "", fmt.Errorf("获取阻断统计失败: %v", err)
	}

	var anomalies []string
	var sb strings.Builder

	sb.WriteString(fmt.Sprintf("🔍 异常检测报告（过去 %d 小时，阈值: %d 次）\n\n", hours, threshold))

	avgPerHour := float64(stats.TotalBlocks) / float64(hours)
	if avgPerHour > float64(threshold)/24 {
		anomalies = append(anomalies, fmt.Sprintf("⚠️ 平均每小时阻断 %.1f 次，高于正常水平", avgPerHour))
	}

	for _, pkg := range stats.TopBlockedPkgs {
		if pkg.Count >= int64(threshold) {
			anomalies = append(anomalies, fmt.Sprintf("🔴 包 %s 被阻断 %d 次，超过阈值 %d", pkg.ResourceName, pkg.Count, threshold))
		}
	}

	for _, ip := range stats.TopIPs {
		if ip.Count >= int64(threshold) {
			anomalies = append(anomalies, fmt.Sprintf("🔴 IP %s 触发阻断 %d 次，超过阈值 %d", ip.IPAddress, ip.Count, threshold))
		}
	}

	if len(stats.BlocksByHour) > 0 {
		maxCount := int64(0)
		maxHour := ""
		for _, h := range stats.BlocksByHour {
			if h.Count > maxCount {
				maxCount = h.Count
				maxHour = h.Hour
			}
		}
		if maxCount > int64(float64(avgPerHour)*3) {
			anomalies = append(anomalies, fmt.Sprintf("🔴 时间段 %s 阻断 %d 次，是平均值的 %.1f 倍", maxHour, maxCount, float64(maxCount)/avgPerHour))
		}
	}

	if len(anomalies) == 0 {
		sb.WriteString("✅ 未检测到异常阻断模式\n")
		sb.WriteString(fmt.Sprintf("\n📊 统计信息:\n"))
		sb.WriteString(fmt.Sprintf("- 总阻断次数: %d\n", stats.TotalBlocks))
		sb.WriteString(fmt.Sprintf("- 平均每小时: %.1f 次\n", avgPerHour))
		sb.WriteString(fmt.Sprintf("- 涉及包数: %d\n", stats.UniquePackages))
		sb.WriteString(fmt.Sprintf("- 涉及IP数: %d\n", stats.UniqueIPs))
	} else {
		sb.WriteString(fmt.Sprintf("⚠️ 检测到 %d 个异常模式:\n\n", len(anomalies)))
		for i, anomaly := range anomalies {
			sb.WriteString(fmt.Sprintf("%d. %s\n", i+1, anomaly))
		}
	}

	sb.WriteString("\n\n💡 建议:\n")
	if len(anomalies) > 0 {
		sb.WriteString("- 检查高频阻断的包是否有误配置\n")
		sb.WriteString("- 分析异常IP是否存在恶意行为\n")
		sb.WriteString("- 审查阻断规则是否需要调整\n")
	} else {
		sb.WriteString("- 继续保持当前阻断规则配置\n")
		sb.WriteString("- 定期监控阻断日志变化趋势\n")
	}

	return sb.String(), nil
}

func (t *BlockLogAnalyzerTool) analyzeIPPattern(hours int) (string, error) {
	stats, err := t.auditRepo.GetBlockStats(hours)
	if err != nil {
		return "", fmt.Errorf("获取阻断统计失败: %v", err)
	}

	var sb strings.Builder
	sb.WriteString(fmt.Sprintf("🌐 IP 阻断模式分析（过去 %d 小时）\n\n", hours))

	sb.WriteString(fmt.Sprintf("🔢 涉及IP总数: **%d**\n\n", stats.UniqueIPs))

	if len(stats.TopIPs) == 0 {
		sb.WriteString("📭 暂无IP阻断记录\n")
		return sb.String(), nil
	}

	totalFromTop := int64(0)
	for _, ip := range stats.TopIPs {
		totalFromTop += ip.Count
	}

	sb.WriteString("📊 IP阻断分布:\n\n")
	for i, ip := range stats.TopIPs {
		percentage := float64(ip.Count) / float64(stats.TotalBlocks) * 100
		sb.WriteString(fmt.Sprintf("%d. **%s**\n", i+1, ip.IPAddress))
		sb.WriteString(fmt.Sprintf("   阻断次数: %d (%.1f%%)\n", ip.Count, percentage))

		if ip.Count > int64(stats.TotalBlocks)/3 {
			sb.WriteString("   ⚠️ 此IP触发了超过 33% 的阻断，建议重点关注\n")
		}
		sb.WriteString("\n")
	}

	if stats.TotalBlocks > 0 && len(stats.TopIPs) > 0 {
		topPercentage := float64(totalFromTop) / float64(stats.TotalBlocks) * 100
		sb.WriteString(fmt.Sprintf("\n📈 Top 10 IP 占总阻断次数的 %.1f%%\n", topPercentage))

		if topPercentage > 80 {
			sb.WriteString("⚠️ 阻断高度集中在少数IP，可能存在定向攻击\n")
		}
	}

	return sb.String(), nil
}

func (t *BlockLogAnalyzerTool) analyzeTimePattern(hours int) (string, error) {
	stats, err := t.auditRepo.GetBlockStats(hours)
	if err != nil {
		return "", fmt.Errorf("获取阻断统计失败: %v", err)
	}

	var sb strings.Builder
	sb.WriteString(fmt.Sprintf("⏰ 时间段阻断模式分析（过去 %d 小时）\n\n", hours))

	if len(stats.BlocksByHour) == 0 {
		sb.WriteString("📭 暂无阻断记录\n")
		return sb.String(), nil
	}

	sb.WriteString("📊 每小时阻断次数:\n\n")

	maxCount := int64(0)
	for _, h := range stats.BlocksByHour {
		if h.Count > maxCount {
			maxCount = h.Count
		}
	}

	for _, h := range stats.BlocksByHour {
		bar := ""
		if maxCount > 0 {
			barLen := int(float64(h.Count) / float64(maxCount) * 20)
			bar = strings.Repeat("█", barLen) + strings.Repeat("░", 20-barLen)
		}
		sb.WriteString(fmt.Sprintf("%s |%s| %d\n", h.Hour, bar, h.Count))
	}

	avgPerHour := float64(stats.TotalBlocks) / float64(len(stats.BlocksByHour))
	sb.WriteString(fmt.Sprintf("\n📈 平均每小时: %.1f 次\n", avgPerHour))

	for _, h := range stats.BlocksByHour {
		if h.Count > int64(avgPerHour*2) {
			sb.WriteString(fmt.Sprintf("⚠️ %s 阻断 %d 次，是平均值的 %.1f 倍\n", h.Hour, h.Count, float64(h.Count)/avgPerHour))
		}
	}

	return sb.String(), nil
}

func (t *BlockLogAnalyzerTool) analyzePackagePattern(hours int) (string, error) {
	stats, err := t.auditRepo.GetBlockStats(hours)
	if err != nil {
		return "", fmt.Errorf("获取阻断统计失败: %v", err)
	}

	var sb strings.Builder
	sb.WriteString(fmt.Sprintf("📦 包阻断模式分析（过去 %d 小时）\n\n", hours))

	sb.WriteString(fmt.Sprintf("🔢 被阻断的唯一包数: **%d**\n\n", stats.UniquePackages))

	if len(stats.TopBlockedPkgs) == 0 {
		sb.WriteString("📭 暂无包阻断记录\n")
		return sb.String(), nil
	}

	sb.WriteString("📊 被阻断最多的包:\n\n")
	for i, pkg := range stats.TopBlockedPkgs {
		percentage := float64(pkg.Count) / float64(stats.TotalBlocks) * 100
		sb.WriteString(fmt.Sprintf("%d. **%s**\n", i+1, pkg.ResourceName))
		sb.WriteString(fmt.Sprintf("   阻断次数: %d (%.1f%%)\n", pkg.Count, percentage))
		sb.WriteString("\n")
	}

	totalFromTop := int64(0)
	for _, pkg := range stats.TopBlockedPkgs {
		totalFromTop += pkg.Count
	}

	if stats.TotalBlocks > 0 {
		topPercentage := float64(totalFromTop) / float64(stats.TotalBlocks) * 100
		sb.WriteString(fmt.Sprintf("\n📈 Top 10 包占总阻断次数的 %.1f%%\n", topPercentage))

		if topPercentage > 90 {
			sb.WriteString("⚠️ 阻断高度集中在少数包，建议审查这些包的安全性\n")
		}
	}

	return sb.String(), nil
}
