-- 性能优化迁移脚本 - P0-2: 添加缺失的关键索引
-- 根据高频查询场景添加复合索引，优化查询性能
-- 执行时间：2026-05-08

-- ========================================
-- packages 表索引优化
-- ========================================

-- 1. 仓库+包类型复合索引（高频查询：按仓库和类型过滤包）
ALTER TABLE `packages` 
ADD INDEX `idx_repo_id_type` (`repository_id`, `type`);

-- 2. 下载量索引（高频排序：按下载量排序）
ALTER TABLE `packages` 
ADD INDEX `idx_download_count` (`download_count`);

-- 3. 创建时间索引（高频排序：按创建时间排序）
ALTER TABLE `packages` 
ADD INDEX `idx_created_at` (`created_at`);

-- ========================================
-- package_versions 表索引优化
-- ========================================

-- 4. 包ID+状态复合索引（高频查询：查询已发布的版本）
ALTER TABLE `package_versions` 
ADD INDEX `idx_pkg_id_status` (`package_id`, `status`);

-- 5. 发布时间索引（高频查询：按时间范围查询版本）
ALTER TABLE `package_versions` 
ADD INDEX `idx_published_at` (`published_at`);

-- ========================================
-- proxy_download_logs 表索引优化
-- ========================================

-- 6. 创建时间+仓库复合索引（高频统计：按时间段和仓库统计下载）
ALTER TABLE `proxy_download_logs` 
ADD INDEX `idx_created_at_repo_id` (`created_at`, `repository_id`);

-- 7. 包名+版本复合索引（高频查询：查询特定包的下载记录）
ALTER TABLE `proxy_download_logs` 
ADD INDEX `idx_pkg_name_version` (`package_name`, `version`);

-- 8. 状态+创建时间复合索引（高频统计：按状态和时间统计）
ALTER TABLE `proxy_download_logs` 
ADD INDEX `idx_status_created_at` (`status`, `created_at`);

-- ========================================
-- audit_logs 表索引优化
-- ========================================

-- 9. 用户ID+创建时间复合索引（高频查询：查询用户操作历史）
ALTER TABLE `audit_logs` 
ADD INDEX `idx_user_id_created_at` (`user_id`, `created_at`);

-- 10. 动作+创建时间复合索引（高频统计：按动作类型统计）
ALTER TABLE `audit_logs` 
ADD INDEX `idx_action_created_at` (`action`, `created_at`);

-- ========================================
-- vulnerabilities 表索引优化
-- ========================================

-- 11. 严重度+CVSS分数复合索引（高频查询：按严重度排序漏洞）
ALTER TABLE `vulnerabilities` 
ADD INDEX `idx_severity_cvss` (`severity`, `cvss_score`);

-- 12. 依赖名称索引（高频查询：查询特定依赖的漏洞）
ALTER TABLE `vulnerabilities` 
ADD INDEX `idx_dep_name` (`dependency_name`);

-- ========================================
-- 验证索引创建结果
-- ========================================
SELECT 
  TABLE_NAME,
  INDEX_NAME,
  GROUP_CONCAT(COLUMN_NAME ORDER BY SEQ_IN_INDEX) AS COLUMNS,
  NON_UNIQUE,
  INDEX_TYPE
FROM INFORMATION_SCHEMA.STATISTICS
WHERE TABLE_SCHEMA = DATABASE()
  AND TABLE_NAME IN (
    'packages', 
    'package_versions', 
    'proxy_download_logs', 
    'audit_logs', 
    'vulnerabilities'
  )
GROUP BY TABLE_NAME, INDEX_NAME, NON_UNIQUE, INDEX_TYPE
ORDER BY TABLE_NAME, INDEX_NAME;
