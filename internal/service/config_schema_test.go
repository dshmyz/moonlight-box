package service

import "testing"

func TestDescribeConfig(t *testing.T) {
	tests := []struct {
		key      string
		category string
		visible  bool
	}{
		// 唯一入口且运行时生效：保留在系统配置页
		{"security.scan_on_upload", ConfigCategorySecurity, true},
		{"security.block_critical", ConfigCategorySecurity, true},
		{"health_check.enabled", ConfigCategoryNetwork, true},
		{"health_check.block_on_unhealthy", ConfigCategoryNetwork, true},

		// 有专属页面维护：系统配置页不重复暴露
		{"cas.server_url", ConfigCategoryAuth, false},
		{"scheduler.cron.security_scan", ConfigCategoryCleanup, false},
		{"scheduler.interval", ConfigCategoryCleanup, false},
		{"cleanup.interval", ConfigCategoryCleanup, false},
		{"log_cleanup.enabled", ConfigCategoryLog, false},
		{"maven_snapshot_cleanup.keep_last", ConfigCategoryCleanup, false},
		{"proxy_cache_gc.max_age_days", ConfigCategoryCleanup, false},
		{"backup.enabled", ConfigCategoryStorage, false},
		{"migration.block_rule_source_backfill_20261029", ConfigCategoryOther, false},

		// 历史死配置：能改但不生效，同样不可见
		{"system.name", ConfigCategoryOther, false},
		{"system.version", ConfigCategoryOther, false},
		{"metrics.enabled", ConfigCategoryOther, false},
		{"webhook.timeout", ConfigCategoryOther, false},
		{"storage.max_file_size", ConfigCategoryOther, false},
		{"cache.max_size_gb", ConfigCategoryOther, false},
		{"auth.token_expiry", ConfigCategoryOther, false},

		// 未登记的新键默认不可见，避免再次出现死配置
		{"future_feature.enabled", ConfigCategoryOther, false},
	}

	for _, tt := range tests {
		category, visible := DescribeConfig(tt.key)
		if category != tt.category || visible != tt.visible {
			t.Errorf("DescribeConfig(%q) = (%q, %v), want (%q, %v)",
				tt.key, category, visible, tt.category, tt.visible)
		}
	}
}
