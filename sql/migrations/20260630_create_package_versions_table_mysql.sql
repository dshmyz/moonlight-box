-- MySQL 8.0+ migration for the package_versions rebuildable read model.
-- Source of truth remains artifacts.

CREATE TABLE IF NOT EXISTS `package_versions` (
  `id` INTEGER UNSIGNED AUTO_INCREMENT PRIMARY KEY,
  `repository_id` INTEGER UNSIGNED NOT NULL,
  `format` VARCHAR(64) NOT NULL,
  `package_name` VARCHAR(512) NOT NULL,
  `namespace` VARCHAR(512) DEFAULT '',
  `version` VARCHAR(255) NOT NULL,
  `status` VARCHAR(32) NOT NULL DEFAULT 'published',
  `published_at` TIMESTAMP NULL,
  `latest_artifact_at` TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP,
  `file_count` INTEGER NOT NULL DEFAULT 0,
  `files_downloaded` TINYINT(1) NOT NULL DEFAULT 0,
  `size_bytes` BIGINT NOT NULL DEFAULT 0,
  `download_count` BIGINT NOT NULL DEFAULT 0,
  `license` VARCHAR(128) DEFAULT '',
  `checksum_sha256` VARCHAR(128) DEFAULT '',
  `created_at` TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP,
  `updated_at` TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP ON UPDATE CURRENT_TIMESTAMP,
  UNIQUE KEY `idx_pkg_ver_repo_format_name_version` (`repository_id`, `format`, `package_name`, `version`),
  INDEX `idx_pkg_ver_repo_format_name_updated` (`repository_id`, `format`, `package_name`, `latest_artifact_at`),
  INDEX `idx_pkg_ver_repo_format_name_published` (`repository_id`, `format`, `package_name`, `published_at`),
  INDEX `idx_package_version_status` (`status`),
  INDEX `idx_package_version_version` (`version`),
  INDEX `idx_package_version_namespace` (`namespace`)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci COMMENT='包版本聚合索引表';
