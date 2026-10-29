package service

import "strings"

// 系统配置页使用的分类标识，前端 web/src/views/SystemConfig.vue 的 CATEGORY_LABELS 需保持一致。
const (
	ConfigCategoryGeneral  = "general"
	ConfigCategoryNetwork  = "network"
	ConfigCategoryCache    = "cache"
	ConfigCategoryStorage  = "storage"
	ConfigCategorySecurity = "security"
	ConfigCategoryAuth     = "auth"
	ConfigCategoryLog      = "log"
	ConfigCategoryCleanup  = "cleanup"
	ConfigCategoryOther    = "other"
)

type configKeyRule struct {
	pattern  string
	prefix   bool
	category string
	visible  bool
}

// configKeyRules 是系统配置页的白名单：只有列在这里且 visible 的键才会出现在页面上。
// 页面上只保留「运行时真的会读取、且没有专属管理页面」的配置；
// 新增运行时配置必须在此登记，否则不会显示（避免再次出现能改但不生效的死配置）。
var configKeyRules = []configKeyRule{
	// 运行时由 SecurityScanner.LoadSecurityConfig 读取
	{pattern: "security.", prefix: true, category: ConfigCategorySecurity, visible: true},

	// 运行时由 main.go 与 proxy.HealthChecker.Reload 读取
	{pattern: "health_check.", prefix: true, category: ConfigCategoryNetwork, visible: true},

	// 以下均有专属页面维护，系统配置页不再重复暴露：
	// CAS 设置页 / 定时任务页（含任务参数）/ 备份管理页 / 下载日志页
	{pattern: "cas.", prefix: true, category: ConfigCategoryAuth, visible: false},
	{pattern: "scheduler.", prefix: true, category: ConfigCategoryCleanup, visible: false},
	{pattern: "cleanup.interval", category: ConfigCategoryCleanup, visible: false},
	{pattern: "log_cleanup.", prefix: true, category: ConfigCategoryLog, visible: false},
	{pattern: "maven_snapshot_cleanup.", prefix: true, category: ConfigCategoryCleanup, visible: false},
	{pattern: "proxy_cache_gc.", prefix: true, category: ConfigCategoryCleanup, visible: false},
	{pattern: "backup.", prefix: true, category: ConfigCategoryStorage, visible: false},

	// 一次性迁移标记（migration.*），在页面上被删除会导致迁移重跑
	{pattern: "migration.", prefix: true, category: ConfigCategoryOther, visible: false},
}

// DescribeConfig 返回配置键在系统配置页上的分类与是否可见。
// 未登记的键一律不可见：页面是白名单，新增运行时配置需要显式登记。
func DescribeConfig(key string) (category string, visible bool) {
	for _, rule := range configKeyRules {
		if !rule.prefix && rule.pattern == key {
			return rule.category, rule.visible
		}
	}
	for _, rule := range configKeyRules {
		if rule.prefix && strings.HasPrefix(key, rule.pattern) {
			return rule.category, rule.visible
		}
	}
	return ConfigCategoryOther, false
}
