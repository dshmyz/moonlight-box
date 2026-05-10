-- Moonlight-Box Registry 数据库建表语句
-- 数据库：MySQL 8.0+
-- 生成时间：2026-05-07

-- ========================================
-- 基础表
-- ========================================

-- 仓库表
CREATE TABLE `repositories` (
  `id` INT UNSIGNED AUTO_INCREMENT PRIMARY KEY,
  `name` VARCHAR(100) NOT NULL,
  `display_name` VARCHAR(200) NOT NULL DEFAULT '',
  `description` TEXT,
  `type` VARCHAR(20) NOT NULL DEFAULT 'local',
  `package_type` VARCHAR(50) NOT NULL DEFAULT '',
  `enabled` TINYINT(1) NOT NULL DEFAULT 1,
  `storage_backend_id` INT UNSIGNED DEFAULT NULL,
  `remote_url` VARCHAR(500) NOT NULL DEFAULT '',
  `auth_type` VARCHAR(20) NOT NULL DEFAULT 'none',
  `auth_config` TEXT,
  `proxy_priority` INT NOT NULL DEFAULT 0,
  `cache_enabled` TINYINT(1) NOT NULL DEFAULT 1,
  `cache_ttl_seconds` INT NOT NULL DEFAULT 86400,
  `cache_max_size_gb` DOUBLE NOT NULL DEFAULT 10,
  `cache_negative_ttl` INT NOT NULL DEFAULT 300,
  `timeout_seconds` INT NOT NULL DEFAULT 0,
  `max_redirects` INT NOT NULL DEFAULT 0,
  `insecure_skip_verify` TINYINT(1) NOT NULL DEFAULT 0,
  `failure_cache_rules` TEXT,
  `allow_overwrite` TINYINT(1) NOT NULL DEFAULT 0,
  `allow_delete` TINYINT(1) NOT NULL DEFAULT 0,
  `download_count` BIGINT NOT NULL DEFAULT 0,
  `created_at` TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP,
  `updated_at` TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP ON UPDATE CURRENT_TIMESTAMP,
  UNIQUE KEY `uk_name` (`name`),
  INDEX `idx_repo_type_pkg` (`type`, `package_type`),
  INDEX `idx_enabled` (`enabled`)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci COMMENT='仓库表';

-- 虚拟仓库与成员仓库关联表
CREATE TABLE `repository_groups` (
  `id` INT UNSIGNED AUTO_INCREMENT PRIMARY KEY,
  `virtual_repo_id` INT UNSIGNED NOT NULL,
  `member_repo_id` INT UNSIGNED NOT NULL,
  `priority` INT NOT NULL DEFAULT 0,
  `created_at` TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP,
  UNIQUE KEY `idx_virtual_member` (`virtual_repo_id`, `member_repo_id`),
  FOREIGN KEY (`virtual_repo_id`) REFERENCES `repositories` (`id`) ON DELETE CASCADE,
  FOREIGN KEY (`member_repo_id`) REFERENCES `repositories` (`id`) ON DELETE CASCADE
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci COMMENT='虚拟仓库与成员仓库关联表';

-- ========================================
-- 包管理相关表
-- ========================================

-- 包表
CREATE TABLE `packages` (
  `id` INT UNSIGNED AUTO_INCREMENT PRIMARY KEY,
  `name` VARCHAR(200) NOT NULL,
  `display_name` VARCHAR(255) NOT NULL DEFAULT '',
  `type` VARCHAR(20) NOT NULL,
  `description` VARCHAR(500) NOT NULL DEFAULT '',
  `repository_id` INT UNSIGNED NOT NULL,
  `repository_type` VARCHAR(20) NOT NULL DEFAULT 'local',
  `homepage` VARCHAR(500) NOT NULL DEFAULT '',
  `license` VARCHAR(100) NOT NULL DEFAULT '',
  `download_count` BIGINT NOT NULL DEFAULT 0,
  `created_by` INT UNSIGNED DEFAULT NULL,
  `created_at` TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP,
  `updated_at` TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP ON UPDATE CURRENT_TIMESTAMP,
  UNIQUE KEY `idx_pkg_name_type` (`name`, `type`),
  INDEX `idx_display_name` (`display_name`),
  INDEX `idx_repository_id` (`repository_id`),
  INDEX `idx_repository_type` (`repository_type`),
  INDEX `idx_repo_id_type` (`repository_id`, `type`),
  INDEX `idx_download_count` (`download_count`),
  INDEX `idx_created_at` (`created_at`)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci COMMENT='包表';

-- 包版本表
CREATE TABLE `package_versions` (
  `id` INT UNSIGNED AUTO_INCREMENT PRIMARY KEY,
  `package_id` INT UNSIGNED NOT NULL,
  `version` VARCHAR(100) NOT NULL,
  `status` VARCHAR(20) NOT NULL DEFAULT 'published',
  `storage_path` VARCHAR(500) NOT NULL DEFAULT '',
  `published_at` TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP,
  `published_by` INT UNSIGNED DEFAULT NULL,
  `metadata` TEXT,
  `download_count` BIGINT NOT NULL DEFAULT 0,
  `size_bytes` BIGINT NOT NULL DEFAULT 0,
  `checksum_md5` VARCHAR(32) NOT NULL DEFAULT '',
  `checksum_sha256` VARCHAR(64) NOT NULL DEFAULT '',
  `files_downloaded` TINYINT(1) NOT NULL DEFAULT 0,
  UNIQUE KEY `idx_ver_pkg_version` (`package_id`, `version`),
  INDEX `idx_status` (`status`),
  FOREIGN KEY (`package_id`) REFERENCES `packages` (`id`) ON DELETE CASCADE
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci COMMENT='包版本表';

-- 包文件表
CREATE TABLE `package_files` (
  `id` INT UNSIGNED AUTO_INCREMENT PRIMARY KEY,
  `version_id` INT UNSIGNED NOT NULL,
  `filename` VARCHAR(255) NOT NULL,
  `file_type` VARCHAR(20) NOT NULL,
  `storage_path` VARCHAR(500) NOT NULL,
  `size_bytes` BIGINT NOT NULL DEFAULT 0,
  `checksum_sha256` VARCHAR(64) NOT NULL DEFAULT '',
  `checksum_md5` VARCHAR(32) NOT NULL DEFAULT '',
  `download_count` BIGINT NOT NULL DEFAULT 0,
  UNIQUE KEY `idx_file_ver_filename` (`version_id`, `filename`),
  INDEX `idx_file_type` (`file_type`),
  FOREIGN KEY (`version_id`) REFERENCES `package_versions` (`id`) ON DELETE CASCADE
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci COMMENT='包文件表';

-- 包依赖表
CREATE TABLE `package_dependencies` (
  `id` INT UNSIGNED AUTO_INCREMENT PRIMARY KEY,
  `version_id` INT UNSIGNED NOT NULL,
  `dep_name` VARCHAR(200) NOT NULL,
  `dep_version_constraint` VARCHAR(200) NOT NULL,
  `dep_type` VARCHAR(50) NOT NULL,
  `package_type` VARCHAR(20) NOT NULL,
  `is_optional` TINYINT(1) NOT NULL DEFAULT 0,
  INDEX `idx_version_id` (`version_id`),
  INDEX `idx_dep_name` (`dep_name`)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci COMMENT='包依赖表';

-- ========================================
-- 用户权限相关表
-- ========================================

-- 用户表
CREATE TABLE `users` (
  `id` INT UNSIGNED AUTO_INCREMENT PRIMARY KEY,
  `username` VARCHAR(50) NOT NULL,
  `password_hash` VARCHAR(255) NOT NULL DEFAULT '',
  `email` VARCHAR(255) NOT NULL DEFAULT '',
  `display_name` VARCHAR(100) NOT NULL DEFAULT '',
  `avatar_url` VARCHAR(500) NOT NULL DEFAULT '',
  `auth_source` VARCHAR(20) NOT NULL DEFAULT 'local',
  `is_active` TINYINT(1) NOT NULL DEFAULT 1,
  `last_login_at` TIMESTAMP NULL DEFAULT NULL,
  `created_at` TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP,
  `updated_at` TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP ON UPDATE CURRENT_TIMESTAMP,
  UNIQUE KEY `uk_username` (`username`),
  UNIQUE KEY `uk_email` (`email`)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci COMMENT='用户表';

-- 角色表
CREATE TABLE `roles` (
  `id` INT UNSIGNED AUTO_INCREMENT PRIMARY KEY,
  `name` VARCHAR(50) NOT NULL,
  `description` VARCHAR(255) NOT NULL DEFAULT '',
  `is_system_role` TINYINT(1) NOT NULL DEFAULT 0,
  `created_at` TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP,
  `updated_at` TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP ON UPDATE CURRENT_TIMESTAMP,
  UNIQUE KEY `uk_name` (`name`)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci COMMENT='角色表';

-- 权限表
CREATE TABLE `permissions` (
  `id` INT UNSIGNED AUTO_INCREMENT PRIMARY KEY,
  `resource` VARCHAR(100) NOT NULL,
  `action` VARCHAR(20) NOT NULL
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci COMMENT='权限表';

-- 用户角色关联表
CREATE TABLE `user_roles` (
  `user_id` INT UNSIGNED NOT NULL,
  `role_id` INT UNSIGNED NOT NULL,
  `assigned_at` TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP,
  `assigned_by` INT UNSIGNED DEFAULT NULL,
  PRIMARY KEY (`user_id`, `role_id`),
  FOREIGN KEY (`user_id`) REFERENCES `users` (`id`) ON DELETE CASCADE,
  FOREIGN KEY (`role_id`) REFERENCES `roles` (`id`) ON DELETE CASCADE
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci COMMENT='用户角色关联表';

-- 角色权限关联表
CREATE TABLE `role_permissions` (
  `role_id` INT UNSIGNED NOT NULL,
  `permission_id` INT UNSIGNED NOT NULL,
  PRIMARY KEY (`role_id`, `permission_id`),
  FOREIGN KEY (`role_id`) REFERENCES `roles` (`id`) ON DELETE CASCADE,
  FOREIGN KEY (`permission_id`) REFERENCES `permissions` (`id`) ON DELETE CASCADE
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci COMMENT='角色权限关联表';

-- ========================================
-- 安全扫描相关表
-- ========================================

-- 扫描结果表
CREATE TABLE `scan_results` (
  `id` INT UNSIGNED AUTO_INCREMENT PRIMARY KEY,
  `version_id` INT UNSIGNED NOT NULL,
  `scan_status` VARCHAR(20) NOT NULL,
  `scanner_version` VARCHAR(50) NOT NULL DEFAULT '',
  `total_vulnerabilities` INT NOT NULL DEFAULT 0,
  `critical_count` INT NOT NULL DEFAULT 0,
  `high_count` INT NOT NULL DEFAULT 0,
  `medium_count` INT NOT NULL DEFAULT 0,
  `low_count` INT NOT NULL DEFAULT 0,
  `scanned_at` TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP,
  `report_path` VARCHAR(500) NOT NULL DEFAULT '',
  `error_message` TEXT,
  `created_at` TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP,
  `updated_at` TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP ON UPDATE CURRENT_TIMESTAMP,
  UNIQUE KEY `uk_version_id` (`version_id`),
  INDEX `idx_scan_status` (`scan_status`)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci COMMENT='扫描结果表';

-- 漏洞表
CREATE TABLE `vulnerabilities` (
  `id` INT UNSIGNED AUTO_INCREMENT PRIMARY KEY,
  `scan_result_id` INT UNSIGNED NOT NULL,
  `cve_id` VARCHAR(30) NOT NULL,
  `severity` VARCHAR(20) NOT NULL,
  `cvss_score` DOUBLE NOT NULL DEFAULT 0,
  `dependency_name` VARCHAR(200) NOT NULL DEFAULT '',
  `current_version` VARCHAR(50) NOT NULL DEFAULT '',
  `fixed_version` VARCHAR(50) NOT NULL DEFAULT '',
  `is_direct_dep` TINYINT(1) NOT NULL DEFAULT 1,
  `title` VARCHAR(500) NOT NULL DEFAULT '',
  `description` TEXT,
  `references` TEXT,
  `created_at` TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP,
  INDEX `idx_scan_result_id` (`scan_result_id`),
  INDEX `idx_cve_id` (`cve_id`),
  INDEX `idx_severity` (`severity`)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci COMMENT='漏洞表';

-- ========================================
-- 存储相关表
-- ========================================

-- 存储后端表
CREATE TABLE `storage_backends` (
  `id` INT UNSIGNED AUTO_INCREMENT PRIMARY KEY,
  `name` VARCHAR(50) NOT NULL,
  `type` VARCHAR(20) NOT NULL,
  `description` VARCHAR(255) NOT NULL DEFAULT '',
  `config` TEXT NOT NULL,
  `is_default` TINYINT(1) NOT NULL DEFAULT 0,
  `status` VARCHAR(20) NOT NULL DEFAULT 'active',
  `is_active` TINYINT(1) NOT NULL DEFAULT 1,
  `created_at` TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP,
  `updated_at` TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP ON UPDATE CURRENT_TIMESTAMP,
  UNIQUE KEY `uk_name` (`name`)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci COMMENT='存储后端表';

-- 缓存条目表
CREATE TABLE `cache_entries` (
  `id` INT UNSIGNED AUTO_INCREMENT PRIMARY KEY,
  `remote_url` VARCHAR(500) NOT NULL,
  `local_key` VARCHAR(500) NOT NULL,
  `package_type` VARCHAR(20) NOT NULL,
  `etag` VARCHAR(100) NOT NULL DEFAULT '',
  `last_modified` VARCHAR(100) NOT NULL DEFAULT '',
  `content_type` VARCHAR(100) NOT NULL DEFAULT '',
  `cached_at` TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP,
  `expires_at` TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP,
  `access_count` BIGINT NOT NULL DEFAULT 0,
  `last_accessed_at` TIMESTAMP NULL DEFAULT NULL,
  `size_bytes` BIGINT NOT NULL DEFAULT 0,
  `hit_count` BIGINT NOT NULL DEFAULT 0,
  `miss_count` BIGINT NOT NULL DEFAULT 0,
  UNIQUE KEY `uk_remote_url` (`remote_url`),
  INDEX `idx_package_type` (`package_type`),
  INDEX `idx_expires_at` (`expires_at`)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci COMMENT='缓存条目表';

-- ========================================
-- Webhook 相关表
-- ========================================

-- Webhook 表
CREATE TABLE `webhooks` (
  `id` INT UNSIGNED AUTO_INCREMENT PRIMARY KEY,
  `name` VARCHAR(100) NOT NULL,
  `url` VARCHAR(500) NOT NULL,
  `secret` VARCHAR(255) NOT NULL DEFAULT '',
  `events` TEXT NOT NULL,
  `status` VARCHAR(20) NOT NULL DEFAULT 'active',
  `repository` VARCHAR(100) NOT NULL DEFAULT '',
  `package_type` VARCHAR(20) NOT NULL DEFAULT '',
  `last_triggered` TIMESTAMP NULL DEFAULT NULL,
  `failure_count` INT NOT NULL DEFAULT 0,
  `created_by` INT UNSIGNED DEFAULT NULL,
  `created_at` TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP,
  `updated_at` TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP ON UPDATE CURRENT_TIMESTAMP,
  UNIQUE KEY `uk_name` (`name`)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci COMMENT='Webhook 表';

-- Webhook 投递记录表
CREATE TABLE `webhook_deliveries` (
  `id` INT UNSIGNED AUTO_INCREMENT PRIMARY KEY,
  `webhook_id` INT UNSIGNED NOT NULL,
  `event` VARCHAR(50) NOT NULL,
  `payload` TEXT,
  `response_code` INT NOT NULL DEFAULT 0,
  `success` TINYINT(1) NOT NULL DEFAULT 0,
  `error` TEXT,
  `duration` BIGINT NOT NULL DEFAULT 0,
  `created_at` TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP,
  `updated_at` TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP ON UPDATE CURRENT_TIMESTAMP,
  INDEX `idx_webhook_id` (`webhook_id`)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci COMMENT='Webhook 投递记录表';

-- ========================================
-- 审计与配置相关表
-- ========================================

-- 审计日志表
CREATE TABLE `audit_logs` (
  `id` INT UNSIGNED AUTO_INCREMENT PRIMARY KEY,
  `user_id` INT UNSIGNED DEFAULT NULL,
  `action` VARCHAR(50) NOT NULL,
  `resource_type` VARCHAR(50) NOT NULL DEFAULT '',
  `resource_id` INT UNSIGNED DEFAULT NULL,
  `resource_name` VARCHAR(200) NOT NULL DEFAULT '',
  `ip_address` VARCHAR(45) NOT NULL DEFAULT '',
  `user_agent` VARCHAR(500) NOT NULL DEFAULT '',
  `request_id` VARCHAR(36) NOT NULL DEFAULT '',
  `response_status` INT NOT NULL DEFAULT 0,
  `details` TEXT,
  `duration_ms` INT NOT NULL DEFAULT 0,
  `created_at` TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP,
  INDEX `idx_action` (`action`),
  INDEX `idx_resource_type` (`resource_type`),
  INDEX `idx_created_at` (`created_at`)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci COMMENT='审计日志表';

-- 系统配置表
CREATE TABLE `system_configs` (
  `key` VARCHAR(100) NOT NULL,
  `value` TEXT NOT NULL,
  `value_type` VARCHAR(20) NOT NULL DEFAULT 'string',
  `category` VARCHAR(50) NOT NULL DEFAULT '',
  `description` VARCHAR(500) NOT NULL DEFAULT '',
  `is_sensitive` TINYINT(1) NOT NULL DEFAULT 0,
  `updated_by` INT UNSIGNED DEFAULT NULL,
  `updated_at` TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP ON UPDATE CURRENT_TIMESTAMP,
  PRIMARY KEY (`key`),
  INDEX `idx_category` (`category`)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci COMMENT='系统配置表';

-- 拦截规则表
CREATE TABLE `block_rules` (
  `id` INT UNSIGNED AUTO_INCREMENT PRIMARY KEY,
  `package_name` VARCHAR(200) NOT NULL,
  `version` VARCHAR(100) NOT NULL,
  `match_type` VARCHAR(20) NOT NULL DEFAULT 'exact',
  `package_type` VARCHAR(20) NOT NULL,
  `reason` VARCHAR(500) NOT NULL DEFAULT '',
  `enabled` TINYINT(1) NOT NULL DEFAULT 1,
  `created_by` INT UNSIGNED DEFAULT NULL,
  `created_at` TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP,
  `updated_at` TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP ON UPDATE CURRENT_TIMESTAMP,
  INDEX `idx_package_name` (`package_name`),
  INDEX `idx_package_type` (`package_type`)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci COMMENT='拦截规则表';

-- ========================================
-- 日志与任务相关表
-- ========================================

-- 代理下载日志表
CREATE TABLE `proxy_download_logs` (
  `id` INT UNSIGNED AUTO_INCREMENT PRIMARY KEY,
  `repository_id` INT UNSIGNED NOT NULL,
  `package_type` VARCHAR(20) NOT NULL,
  `package_name` VARCHAR(200) NOT NULL,
  `version` VARCHAR(50) NOT NULL DEFAULT '',
  `filename` VARCHAR(255) NOT NULL DEFAULT '',
  `remote_url` VARCHAR(500) NOT NULL DEFAULT '',
  `status` VARCHAR(20) NOT NULL,
  `status_code` INT NOT NULL DEFAULT 0,
  `size_bytes` BIGINT NOT NULL DEFAULT 0,
  `duration_ms` INT NOT NULL DEFAULT 0,
  `from_cache` TINYINT(1) NOT NULL DEFAULT 0,
  `ip_address` VARCHAR(45) NOT NULL DEFAULT '',
  `user_agent` VARCHAR(500) NOT NULL DEFAULT '',
  `user_id` INT UNSIGNED DEFAULT NULL,
  `error_message` TEXT,
  `created_at` TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP,
  INDEX `idx_repository_id` (`repository_id`),
  INDEX `idx_package_type` (`package_type`),
  INDEX `idx_package_name` (`package_name`),
  INDEX `idx_status` (`status`),
  INDEX `idx_created_at` (`created_at`)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci COMMENT='代理下载日志表';

-- 备份表
CREATE TABLE `backups` (
  `id` INT UNSIGNED AUTO_INCREMENT PRIMARY KEY,
  `name` VARCHAR(100) NOT NULL,
  `type` VARCHAR(20) NOT NULL DEFAULT 'full',
  `status` VARCHAR(20) NOT NULL DEFAULT 'pending',
  `size_bytes` BIGINT NOT NULL DEFAULT 0,
  `file_path` VARCHAR(500) NOT NULL,
  `description` VARCHAR(500) NOT NULL DEFAULT '',
  `started_at` TIMESTAMP NULL DEFAULT NULL,
  `completed_at` TIMESTAMP NULL DEFAULT NULL,
  `error` TEXT,
  `created_by` INT UNSIGNED DEFAULT NULL,
  `created_at` TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP,
  `updated_at` TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP ON UPDATE CURRENT_TIMESTAMP,
  UNIQUE KEY `uk_name` (`name`)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci COMMENT='备份表';

-- 迁移任务表
CREATE TABLE `migration_tasks` (
  `id` INT UNSIGNED AUTO_INCREMENT PRIMARY KEY,
  `source_type` VARCHAR(50) NOT NULL DEFAULT '',
  `source_url` VARCHAR(500) NOT NULL DEFAULT '',
  `username` VARCHAR(100) NOT NULL DEFAULT '',
  `password_encrypted` TEXT NOT NULL DEFAULT '',
  `status` VARCHAR(20) NOT NULL DEFAULT 'pending',
  `total_items` INT NOT NULL DEFAULT 0,
  `processed_items` INT NOT NULL DEFAULT 0,
  `failed_items` INT NOT NULL DEFAULT 0,
  `selected_repos` TEXT,
  `error_message` TEXT,
  `created_at` TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP,
  `updated_at` TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP ON UPDATE CURRENT_TIMESTAMP,
  `started_at` TIMESTAMP NULL DEFAULT NULL,
  `completed_at` TIMESTAMP NULL DEFAULT NULL,
  INDEX `idx_status` (`status`)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci COMMENT='迁移任务表';
