-- 性能优化迁移脚本 - 创建包聚合表
-- 彻底优化包列表查询性能，从聚合查询改为直接查询
-- 执行时间：2026-06-04

-- ========================================
-- 创建包聚合表
-- ========================================

CREATE TABLE IF NOT EXISTS `packages` (
  `id` INTEGER UNSIGNED AUTO_INCREMENT PRIMARY KEY,
  `repository_id` INTEGER UNSIGNED NOT NULL,
  `format` VARCHAR(64) NOT NULL,
  `name` VARCHAR(512) NOT NULL,
  `namespace` VARCHAR(255) DEFAULT '',
  `display_name` VARCHAR(512) DEFAULT '',
  `description` TEXT,
  `latest_version` VARCHAR(255) DEFAULT '',
  `version_count` INTEGER NOT NULL DEFAULT 0,
  `download_count` BIGINT NOT NULL DEFAULT 0,
  `license` VARCHAR(128) DEFAULT '',
  `created_at` TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP,
  `updated_at` TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP ON UPDATE CURRENT_TIMESTAMP,
  UNIQUE KEY `idx_package_repo_format_name` (`repository_id`, `format`, `name`),
  INDEX `idx_package_name` (`name`),
  INDEX `idx_package_namespace` (`namespace`),
  INDEX `idx_package_format` (`format`),
  INDEX `idx_package_updated_at` (`updated_at`),
  INDEX `idx_package_download_count` (`download_count`)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci COMMENT='包聚合表';

-- ========================================
-- 创建包版本聚合表
-- ========================================

CREATE TABLE IF NOT EXISTS `package_versions` (
  `id` INTEGER UNSIGNED AUTO_INCREMENT PRIMARY KEY,
  `package_id` INTEGER UNSIGNED NOT NULL,
  `version` VARCHAR(255) NOT NULL,
  `artifact_id` INTEGER UNSIGNED NOT NULL,
  `size_bytes` BIGINT NOT NULL DEFAULT 0,
  `download_count` BIGINT NOT NULL DEFAULT 0,
  `published_at` TIMESTAMP NULL DEFAULT NULL,
  `created_at` TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP,
  INDEX `idx_pkg_version_package` (`package_id`),
  INDEX `idx_pkg_version_version` (`version`),
  INDEX `idx_pkg_version_artifact` (`artifact_id`),
  FOREIGN KEY (`package_id`) REFERENCES `packages` (`id`) ON DELETE CASCADE
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci COMMENT='包版本聚合表';

-- ========================================
-- 迁移历史数据
-- ========================================

-- 步骤1：从 artifacts 表聚合包数据
-- 注意：这里使用子查询，SQLite 和 MySQL 语法略有不同

-- MySQL 版本
INSERT INTO `packages` (`repository_id`, `format`, `name`, `version_count`, `latest_version`, `created_at`, `updated_at`)
SELECT
  a.repository_id,
  a.format,
  JSON_UNQUOTE(JSON_EXTRACT(a.coordinates, '$.name')) AS name,
  COUNT(*) AS version_count,
  (SELECT JSON_UNQUOTE(JSON_EXTRACT(a2.coordinates, '$.version'))
   FROM artifacts a2
   WHERE a2.repository_id = a.repository_id
     AND a2.format = a.format
     AND JSON_UNQUOTE(JSON_EXTRACT(a2.coordinates, '$.name')) = JSON_UNQUOTE(JSON_EXTRACT(a.coordinates, '$.name'))
   ORDER BY a2.updated_at DESC
   LIMIT 1) AS latest_version,
  MIN(a.created_at) AS created_at,
  MAX(a.updated_at) AS updated_at
FROM artifacts a
WHERE JSON_UNQUOTE(JSON_EXTRACT(a.coordinates, '$.name')) IS NOT NULL
  AND JSON_UNQUOTE(JSON_EXTRACT(a.coordinates, '$.name')) != ''
GROUP BY a.repository_id, a.format, JSON_UNQUOTE(JSON_EXTRACT(a.coordinates, '$.name'))
ON DUPLICATE KEY UPDATE
  version_count = VALUES(version_count),
  latest_version = VALUES(latest_version),
  updated_at = VALUES(updated_at);

-- ========================================
-- 验证迁移结果
-- ========================================

-- 查看迁移统计
SELECT
  'packages' AS table_name,
  COUNT(*) AS total_rows
FROM packages
UNION ALL
SELECT
  'artifacts' AS table_name,
  COUNT(*) AS total_rows
FROM artifacts;

-- 查看按格式统计
SELECT
  format,
  COUNT(*) AS package_count
FROM packages
GROUP BY format
ORDER BY package_count DESC;
