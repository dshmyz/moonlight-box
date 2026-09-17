package main

import (
	"testing"
)

// 目标 MySQL 列/表名必须加反引号：一旦出现保留字列名（如 rank、usage），
// 裸拼接的 ORDER BY / SELECT COUNT 会直接语法错误。
func TestQuoteCols(t *testing.T) {
	if got := quoteCols([]string{"id", "name"}); got != "`id`, `name`" {
		t.Errorf("quoteCols = %q, want %q", got, "`id`, `name`")
	}
}

func TestQuoteIdentEscapesBacktick(t *testing.T) {
	if got := quoteIdent("we`ird"); got != "`we``ird`" {
		t.Errorf("quoteIdent = %q, want %q", got, "`we``ird`")
	}
}

// parseSpec 输出的主键列名必须能原样进 SQL：验证 quoteCols 是恒等包裹。
func TestQuoteColsRoundTripsSchemaNames(t *testing.T) {
	names := []string{"artifact_id", "blob_id", "position", "rank", "usage"}
	got := quoteCols(names)
	want := "`artifact_id`, `blob_id`, `position`, `rank`, `usage`"
	if got != want {
		t.Errorf("quoteCols = %q, want %q", got, want)
	}
}
