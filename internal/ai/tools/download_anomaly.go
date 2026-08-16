package tools

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"sync"
	"time"

	"github.com/dshmyz/moonlight-box/internal/model"
	"gorm.io/gorm"
)

// DownloadAnomalyTool 下载行为异常检测。
//
// 基于 download_logs / download_daily_stats 分析：
//   - overview：下载概览（总量/唯一包/IP/Top N）；
//   - spike_detection：单包日下载量相对基线骤增（可能为投毒/恶意拉取）；
//   - new_package：近期首次出现的包（新包投毒风险）；
//   - ip_focus：下载 IP 集中度（单一来源占比过高）；
//   - failed_spike：失败率骤增（探测/攻击特征）。
//
// 只读分析，带 5 分钟结果缓存。
type DownloadAnomalyTool struct {
	BaseTool
	cache    *sync.Map
	cacheTTL time.Duration
}

type downloadAnomalyCacheEntry struct {
	result    string
	expiresAt time.Time
}

func NewDownloadAnomalyTool() *DownloadAnomalyTool {
	return &DownloadAnomalyTool{
		cache:    &sync.Map{},
		cacheTTL: 5 * time.Minute,
	}
}

func (t *DownloadAnomalyTool) Name() string {
	return "download_anomaly"
}

func (t *DownloadAnomalyTool) Description() string {
	return "下载行为异常检测：单包下载骤增、新包出现、IP集中、失败率异常等。" +
		"可用于识别供应链投毒与恶意拉取。只读分析。"
}

func (t *DownloadAnomalyTool) Parameters() json.RawMessage {
	return json.RawMessage(`{
		"type": "object",
		"properties": {
			"analysis_type": {
				"type": "string",
				"description": "分析类型",
				"enum": ["overview", "spike_detection", "new_package", "ip_focus", "failed_spike"]
			},
			"hours": {
				"type": "integer",
				"description": "分析时间范围（小时），默认 24",
				"default": 24
			},
			"threshold": {
				"type": "integer",
				"description": "骤增判定倍数（相对基线），默认 5",
				"default": 5
			},
			"limit": {
				"type": "integer",
				"description": "返回条目上限，默认 20",
				"default": 20
			}
		},
		"required": ["analysis_type"]
	}`)
}

func (t *DownloadAnomalyTool) Execute(ctx context.Context, params map[string]interface{}) (string, error) {
	analysisType, ok := params["analysis_type"].(string)
	if !ok {
		return "", fmt.Errorf("缺少必需参数: analysis_type")
	}
	hours := 24
	if h, ok := params["hours"].(float64); ok {
		hours = int(h)
	}
	threshold := 5
	if th, ok := params["threshold"].(float64); ok {
		threshold = int(th)
	}
	limit := 20
	if l, ok := params["limit"].(float64); ok {
		limit = int(l)
	}

	toolCtx := t.Context()
	if toolCtx == nil || toolCtx.DB == nil {
		return "", fmt.Errorf("工具上下文未配置 DB")
	}
	db := toolCtx.DB

	cacheKey := fmt.Sprintf("download_anomaly:%s:%d:%d:%d", analysisType, hours, threshold, limit)
	if entry, ok := t.cache.Load(cacheKey); ok {
		ce := entry.(*downloadAnomalyCacheEntry)
		if time.Now().Before(ce.expiresAt) {
			return ce.result, nil
		}
		t.cache.Delete(cacheKey)
	}

	var result string
	var err error
	switch analysisType {
	case "overview":
		result, err = t.analyzeOverview(db, hours, limit)
	case "spike_detection":
		result, err = t.detectSpikes(db, hours, threshold, limit)
	case "new_package":
		result, err = t.detectNewPackages(db, hours, limit)
	case "ip_focus":
		result, err = t.analyzeIPFocus(db, hours, limit)
	case "failed_spike":
		result, err = t.detectFailedSpike(db, hours, threshold, limit)
	default:
		return "", fmt.Errorf("不支持的分析类型: %s", analysisType)
	}
	if err != nil {
		return "", err
	}

	t.cache.Store(cacheKey, &downloadAnomalyCacheEntry{
		result:    result,
		expiresAt: time.Now().Add(t.cacheTTL),
	})
	return result, nil
}

// ClearCache 清理缓存（下载日志更新时调用）。
func (t *DownloadAnomalyTool) ClearCache() {
	t.cache.Range(func(key, value interface{}) bool {
		t.cache.Delete(key)
		return true
	})
}

// downloadRow 聚合查询结果行。
type downloadRow struct {
	PackageName string
	PackageType string
	Count       int64
	FailedCount int64
	IPAddress   string
}

// analyzeOverview 下载概览。
func (t *DownloadAnomalyTool) analyzeOverview(db *gorm.DB, hours, limit int) (string, error) {
	cutoff := time.Now().Add(-time.Duration(hours) * time.Hour)

	var total, failed int64
	db.Model(&model.DownloadLog{}).Where("created_at >= ?", cutoff).Count(&total)
	db.Model(&model.DownloadLog{}).
		Where("created_at >= ? AND status = ?", cutoff, model.DownloadStatusFailed).Count(&failed)

	var uniquePkgs, uniqueIPs int64
	db.Model(&model.DownloadLog{}).Where("created_at >= ?", cutoff).
		Distinct("package_name").Count(&uniquePkgs)
	db.Model(&model.DownloadLog{}).
		Where("created_at >= ? AND ip_address != ''", cutoff).
		Distinct("ip_address").Count(&uniqueIPs)

	var topPkgs []downloadRow
	db.Model(&model.DownloadLog{}).
		Select("package_name, COUNT(*) as count").
		Where("created_at >= ?", cutoff).
		Group("package_name").
		Order("count DESC").
		Limit(limit).
		Scan(&topPkgs)

	var topIPs []downloadRow
	db.Model(&model.DownloadLog{}).
		Select("ip_address, COUNT(*) as count").
		Where("created_at >= ? AND ip_address != ''", cutoff).
		Group("ip_address").
		Order("count DESC").
		Limit(limit).
		Scan(&topIPs)

	var sb strings.Builder
	sb.WriteString(fmt.Sprintf("📊 下载概览（过去 %d 小时）\n\n", hours))
	sb.WriteString(fmt.Sprintf("🔢 总下载(非缓存): **%d**\n", total))
	sb.WriteString(fmt.Sprintf("❌ 失败: **%d** (%.1f%%)\n", failed, pct(failed, total)))
	sb.WriteString(fmt.Sprintf("📦 唯一包数: **%d**\n", uniquePkgs))
	sb.WriteString(fmt.Sprintf("🌐 唯一IP数: **%d**\n\n", uniqueIPs))

	if len(topPkgs) > 0 {
		sb.WriteString("🔝 下载最多的包（Top N）:\n\n")
		for i, p := range topPkgs {
			sb.WriteString(fmt.Sprintf("%d. **%s** (%s) - %d 次\n", i+1, p.PackageName, p.PackageType, p.Count))
		}
		sb.WriteString("\n")
	}
	if len(topIPs) > 0 {
		sb.WriteString("🌐 下载最多的IP（Top N）:\n\n")
		for i, ip := range topIPs {
			sb.WriteString(fmt.Sprintf("%d. **%s** - %d 次\n", i+1, ip.IPAddress, ip.Count))
		}
	}
	return sb.String(), nil
}

// detectSpikes 单包日下载量骤增检测。
// 基线 = 目标包在观察期前的日均下载量；最近 24h 下载量 > baseline*threshold 判定为异常。
func (t *DownloadAnomalyTool) detectSpikes(db *gorm.DB, hours, threshold, limit int) (string, error) {
	cutoff := time.Now().Add(-time.Duration(hours) * time.Hour)
	// 基线期 = 观察期再往前同长时段
	baselineCutoff := cutoff.Add(-time.Duration(hours) * time.Hour)

	type pkgAgg struct {
		baseCount int64
	}
	agg := make(map[string]*pkgAgg)

	var rows []downloadRow
	db.Model(&model.DownloadLog{}).
		Select("package_name, COUNT(*) as count").
		Where("created_at >= ? AND created_at < ?", baselineCutoff, cutoff).
		Group("package_name").
		Limit(5000).
		Scan(&rows)
	for _, r := range rows {
		key := r.PackageName
		if _, ok := agg[key]; !ok {
			agg[key] = &pkgAgg{}
		}
		agg[key].baseCount = r.Count
	}

	var recent []downloadRow
	db.Model(&model.DownloadLog{}).
		Select("package_name, COUNT(*) as count").
		Where("created_at >= ?", cutoff).
		Group("package_name").
		Order("count DESC").
		Limit(limit * 5).
		Scan(&recent)

	var spikes []string
	var sb strings.Builder
	sb.WriteString(fmt.Sprintf("🔍 下载骤增检测（过去 %d 小时 vs 之前 %d 小时基线，阈值 %.0fx）\n\n", hours, hours, float64(threshold)))

	for _, r := range recent {
		base, ok := agg[r.PackageName]
		if !ok {
			continue // 无基线数据（可能是新包），交给 new_package 检测
		}
		baseDays := float64(hours) / 24.0
		recentPerDay := float64(r.Count) / baseDays
		basePerDay := float64(base.baseCount) / baseDays
		if basePerDay >= 1 && recentPerDay >= basePerDay*float64(threshold) && recentPerDay-basePerDay >= 5 {
			spikes = append(spikes, fmt.Sprintf("🔴 **%s**: 近%dh %d次，基线日均 %.1f → 当前日均 %.1f (%.1fx)",
				r.PackageName, hours, r.Count, basePerDay, recentPerDay, recentPerDay/basePerDay))
			if len(spikes) >= limit {
				break
			}
		}
	}

	if len(spikes) == 0 {
		sb.WriteString("✅ 未检测到下载骤增\n")
	} else {
		sb.WriteString(fmt.Sprintf("⚠️ 检测到 %d 个异常包:\n\n", len(spikes)))
		for i, s := range spikes {
			sb.WriteString(fmt.Sprintf("%d. %s\n", i+1, s))
		}
	}
	sb.WriteString("\n💡 下载骤增可能原因: 正常发布/推广、缓存回源、供应链投毒或恶意拉取。建议结合包发布时间与内容校验确认。")
	return sb.String(), nil
}

// detectNewPackages 观察期内首次出现的包。
func (t *DownloadAnomalyTool) detectNewPackages(db *gorm.DB, hours, limit int) (string, error) {
	cutoff := time.Now().Add(-time.Duration(hours) * time.Hour)

	var newPkgs []downloadRow
	db.Raw(`SELECT package_name, package_type, COUNT(*) AS count
		FROM download_logs d
		WHERE d.created_at >= ? AND d.package_name NOT IN (
			SELECT package_name FROM download_logs WHERE created_at < ?
		)
		GROUP BY package_name, package_type
		ORDER BY count DESC
		LIMIT ?`, cutoff, cutoff, limit).
		Scan(&newPkgs)

	var sb strings.Builder
	sb.WriteString(fmt.Sprintf("🆕 新包检测（过去 %d 小时内首次出现）\n\n", hours))
	if len(newPkgs) == 0 {
		sb.WriteString("✅ 无新包出现\n")
		return sb.String(), nil
	}
	for i, p := range newPkgs {
		sb.WriteString(fmt.Sprintf("%d. **%s** (%s) - %d 次下载\n", i+1, p.PackageName, p.PackageType, p.Count))
	}
	sb.WriteString("\n⚠️ 新包 + 下载骤增 组合是高危信号（类域名抢注的包名仿冒攻击）。建议核查包来源与签名。")
	return sb.String(), nil
}

// analyzeIPFocus 下载 IP 集中度分析。
func (t *DownloadAnomalyTool) analyzeIPFocus(db *gorm.DB, hours, limit int) (string, error) {
	cutoff := time.Now().Add(-time.Duration(hours) * time.Hour)

	var total int64
	db.Model(&model.DownloadLog{}).Where("created_at >= ?", cutoff).Count(&total)
	if total == 0 {
		return "📭 观察期内无下载记录", nil
	}

	var topIPs []downloadRow
	db.Model(&model.DownloadLog{}).
		Select("ip_address, COUNT(*) as count").
		Where("created_at >= ? AND ip_address != ''", cutoff).
		Group("ip_address").
		Order("count DESC").
		Limit(limit).
		Scan(&topIPs)

	var topPkgs []downloadRow
	db.Model(&model.DownloadLog{}).
		Select("package_name, COUNT(*) as count").
		Where("created_at >= ? AND ip_address != ''", cutoff).
		Group("package_name").
		Order("count DESC").
		Limit(limit).
		Scan(&topPkgs)

	var sb strings.Builder
	sb.WriteString(fmt.Sprintf("🌐 IP 集中度分析（过去 %d 小时）\n\n", hours))
	sb.WriteString(fmt.Sprintf("🔢 总下载: %d\n\n", total))

	if len(topIPs) > 0 {
		sb.WriteString("📊 下载量最大的IP:\n\n")
		var topSum int64
		for i, ip := range topIPs {
			topSum += ip.Count
			flag := ""
			if pct(ip.Count, total) > 50 {
				flag = " ⚠️ 单一IP占比>50%"
			}
			sb.WriteString(fmt.Sprintf("%d. **%s** - %d 次 (%.1f%%)%s\n", i+1, ip.IPAddress, ip.Count, pct(ip.Count, total), flag))
		}
		sb.WriteString(fmt.Sprintf("\n📈 Top %d IP 占总下载的 %.1f%%\n", len(topIPs), pct(topSum, total)))
		if pct(topSum, total) > 80 {
			sb.WriteString("⚠️ 下载高度集中在少数 IP，可能存在恶意拉取或爬虫\n")
		}
	}

	if len(topPkgs) > 0 {
		sb.WriteString("\n📦 IP 关联的 Top 包:\n\n")
		for i, p := range topPkgs {
			sb.WriteString(fmt.Sprintf("%d. %s - %d 次\n", i+1, p.PackageName, p.Count))
		}
	}
	return sb.String(), nil
}

// detectFailedSpike 失败率骤增检测。
func (t *DownloadAnomalyTool) detectFailedSpike(db *gorm.DB, hours, threshold, limit int) (string, error) {
	cutoff := time.Now().Add(-time.Duration(hours) * time.Hour)
	baselineCutoff := cutoff.Add(-time.Duration(hours) * time.Hour)

	type pkgAgg struct {
		total  int64
		failed int64
	}
	baseline := make(map[string]*pkgAgg)
	var baseRows []downloadRow
	db.Model(&model.DownloadLog{}).
		Select("package_name, COUNT(*) as count, "+
			"SUM(CASE WHEN status = ? THEN 1 ELSE 0 END) as failed_count", model.DownloadStatusFailed).
		Where("created_at >= ? AND created_at < ?", baselineCutoff, cutoff).
		Group("package_name").
		Scan(&baseRows)
	for _, r := range baseRows {
		baseline[r.PackageName] = &pkgAgg{total: r.Count, failed: r.FailedCount}
	}

	var recentRows []downloadRow
	db.Model(&model.DownloadLog{}).
		Select("package_name, COUNT(*) as count, "+
			"SUM(CASE WHEN status = ? THEN 1 ELSE 0 END) as failed_count", model.DownloadStatusFailed).
		Where("created_at >= ?", cutoff).
		Group("package_name").
		Scan(&recentRows)

	var anomalies []string
	var sb strings.Builder
	sb.WriteString(fmt.Sprintf("❌ 失败率骤增检测（过去 %d 小时，阈值 %.0fx）\n\n", hours, float64(threshold)))

	for _, r := range recentRows {
		if r.Count < 10 {
			continue // 样本太少不判定
		}
		base, ok := baseline[r.PackageName]
		baseRate := float64(0)
		if ok && base.total > 0 {
			baseRate = float64(base.failed) / float64(base.total)
		}
		curRate := float64(r.FailedCount) / float64(r.Count)
		// 基线失败率显著时按相对倍数检测；基线过低或无基线（0%）时，
		// 任何 ≥50% 的失败率都是骤增（0% → 60% 这类场景不能用倍数阈值）。
		if (baseRate >= 0.05 && curRate >= baseRate*float64(threshold)) ||
			(baseRate < 0.05 && curRate >= 0.5) {
			anomalies = append(anomalies, fmt.Sprintf("🔴 **%s**: 失败率 %.0f%% → %.0f%% (基线 %.0f%%)",
				r.PackageName, baseRate*100, curRate*100, baseRate*100))
			if len(anomalies) >= limit {
				break
			}
		}
	}

	if len(anomalies) == 0 {
		sb.WriteString("✅ 未检测到失败率骤增\n")
	} else {
		sb.WriteString(fmt.Sprintf("⚠️ 检测到 %d 个失败率异常包:\n\n", len(anomalies)))
		for i, a := range anomalies {
			sb.WriteString(fmt.Sprintf("%d. %s\n", i+1, a))
		}
	}
	sb.WriteString("\n💡 失败率骤增可能表示: 版本被撤回/文件损坏、阻断规则误伤、或被定向探测。")
	return sb.String(), nil
}

// pct 计算百分比（保留一位小数）。
func pct(part, total int64) float64 {
	if total == 0 {
		return 0
	}
	return float64(int(float64(part)/float64(total)*1000)) / 10
}
