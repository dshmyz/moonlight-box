-- Artifacts lookup index optimization for PyPI/Maven/YUM/APT remote_path flows.
-- Run the section that matches your database dialect.

-- ========================================
-- MySQL 8.0+
-- ========================================
ALTER TABLE `artifacts`
  MODIFY COLUMN `remote_path` VARCHAR(1024) DEFAULT NULL;

CREATE INDEX `idx_artifacts_repo_format_remote_path`
  ON `artifacts` (`repository_id`, `format`, `remote_path`(512));

CREATE INDEX `idx_artifacts_repo_format_name`
  ON `artifacts` (`repository_id`, `format`, `name`);

CREATE INDEX `idx_artifacts_repo_format_name_version`
  ON `artifacts` (`repository_id`, `format`, `name`, `version`);

CREATE INDEX `idx_artifacts_repo_format_filename`
  ON `artifacts` (`repository_id`, `format`, `filename`(512));

CREATE INDEX `idx_artifacts_repo_format_kind_name_version`
  ON `artifacts` (`repository_id`, `format`, `kind`, `name`, `version`);

-- ========================================
-- PostgreSQL
-- ========================================
-- ALTER TABLE artifacts ALTER COLUMN remote_path TYPE varchar(1024);
-- CREATE INDEX IF NOT EXISTS idx_artifacts_repo_format_remote_path
--   ON artifacts (repository_id, format, remote_path);
-- CREATE INDEX IF NOT EXISTS idx_artifacts_repo_format_name
--   ON artifacts (repository_id, format, name);
-- CREATE INDEX IF NOT EXISTS idx_artifacts_repo_format_name_version
--   ON artifacts (repository_id, format, name, version);
-- CREATE INDEX IF NOT EXISTS idx_artifacts_repo_format_filename
--   ON artifacts (repository_id, format, filename);
-- CREATE INDEX IF NOT EXISTS idx_artifacts_repo_format_kind_name_version
--   ON artifacts (repository_id, format, kind, name, version);

-- ========================================
-- SQLite
-- ========================================
-- SQLite cannot alter a TEXT column to VARCHAR in place. The type is advisory
-- there, so keep existing data and add lookup indexes.
-- CREATE INDEX IF NOT EXISTS idx_artifacts_repo_format_remote_path
--   ON artifacts (repository_id, format, remote_path);
-- CREATE INDEX IF NOT EXISTS idx_artifacts_repo_format_name
--   ON artifacts (repository_id, format, name);
-- CREATE INDEX IF NOT EXISTS idx_artifacts_repo_format_name_version
--   ON artifacts (repository_id, format, name, version);
-- CREATE INDEX IF NOT EXISTS idx_artifacts_repo_format_filename
--   ON artifacts (repository_id, format, filename);
-- CREATE INDEX IF NOT EXISTS idx_artifacts_repo_format_kind_name_version
--   ON artifacts (repository_id, format, kind, name, version);
