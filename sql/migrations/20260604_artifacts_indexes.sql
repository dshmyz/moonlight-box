-- 性能优化迁移脚本 - Artifacts 表索引优化
-- 针对包搜索场景添加关键索引，优化查询性能
-- 执行时间：2026-06-04

-- ========================================
-- artifacts 表索引优化
-- ========================================

-- 1. format 字段索引（按包类型过滤）
ALTER TABLE `artifacts`
ADD INDEX IF NOT EXISTS `idx_artifacts_format` (`format`);

-- 2. created_at 字段索引（按创建时间排序）
ALTER TABLE `artifacts`
ADD INDEX IF NOT EXISTS `idx_artifacts_created_at` (`created_at`);

-- 3. updated_at 字段索引（按更新时间排序）
ALTER TABLE `artifacts`
ADD INDEX IF NOT EXISTS `idx_artifacts_updated_at` (`updated_at`);

-- 4. 复合索引：仓库+格式+创建时间（高频查询：按仓库和类型过滤，按时间排序）
ALTER TABLE `artifacts`
ADD INDEX IF NOT EXISTS `idx_artifacts_repo_format_created` (`repository_id`, `format`, `created_at`);

-- 5. 复合索引：仓库+格式+更新时间（高频查询：按仓库和类型过滤，按更新时间排序）
ALTER TABLE `artifacts`
ADD INDEX IF NOT EXISTS `idx_artifacts_repo_format_updated` (`repository_id`, `format`, `updated_at`);

-- ========================================
-- SQLite 版本（如果使用 SQLite）
-- ========================================
-- SQLite 不支持 IF NOT EXISTS，需要单独处理
-- 以下语句在 SQLite 中执行时，如果索引已存在会报错，可以忽略

-- CREATE INDEX IF NOT EXISTS idx_artifacts_format ON artifacts(format);
-- CREATE INDEX IF NOT EXISTS idx_artifacts_created_at ON artifacts(created_at);
-- CREATE INDEX IF NOT EXISTS idx_artifacts_updated_at ON artifacts(updated_at);
-- CREATE INDEX IF NOT EXISTS idx_artifacts_repo_format_created ON artifacts(repository_id, format, created_at);
-- CREATE INDEX IF NOT EXISTS idx_artifacts_repo_format_updated ON artifacts(repository_id, format, updated_at);

-- ========================================
-- 验证索引创建结果（MySQL）
-- ========================================
-- SELECT
--   TABLE_NAME,
--   INDEX_NAME,
--   GROUP_CONCAT(COLUMN_NAME ORDER BY SEQ_IN_INDEX) AS COLUMNS,
--   NON_UNIQUE,
--   INDEX_TYPE
-- FROM INFORMATION_SCHEMA.STATISTICS
-- WHERE TABLE_SCHEMA = DATABASE()
--   AND TABLE_NAME = 'artifacts'
-- GROUP BY TABLE_NAME, INDEX_NAME, NON_UNIQUE, INDEX_TYPE
-- ORDER BY TABLE_NAME, INDEX_NAME;
