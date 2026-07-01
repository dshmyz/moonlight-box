-- PostgreSQL migration for the package_versions rebuildable read model.
-- Source of truth remains artifacts.

CREATE TABLE IF NOT EXISTS package_versions (
  id BIGSERIAL PRIMARY KEY,
  repository_id BIGINT NOT NULL,
  format VARCHAR(64) NOT NULL,
  package_name VARCHAR(512) NOT NULL,
  namespace VARCHAR(512) DEFAULT '',
  version VARCHAR(255) NOT NULL,
  status VARCHAR(32) NOT NULL DEFAULT 'published',
  published_at TIMESTAMPTZ NULL,
  latest_artifact_at TIMESTAMPTZ NOT NULL DEFAULT CURRENT_TIMESTAMP,
  file_count INTEGER NOT NULL DEFAULT 0,
  files_downloaded BOOLEAN NOT NULL DEFAULT FALSE,
  size_bytes BIGINT NOT NULL DEFAULT 0,
  download_count BIGINT NOT NULL DEFAULT 0,
  license VARCHAR(128) DEFAULT '',
  checksum_sha256 VARCHAR(128) DEFAULT '',
  created_at TIMESTAMPTZ NOT NULL DEFAULT CURRENT_TIMESTAMP,
  updated_at TIMESTAMPTZ NOT NULL DEFAULT CURRENT_TIMESTAMP,
  CONSTRAINT idx_pkg_ver_repo_format_name_version UNIQUE (repository_id, format, package_name, version)
);

CREATE INDEX IF NOT EXISTS idx_pkg_ver_repo_format_name_updated
  ON package_versions (repository_id, format, package_name, latest_artifact_at);
CREATE INDEX IF NOT EXISTS idx_pkg_ver_repo_format_name_published
  ON package_versions (repository_id, format, package_name, published_at);
CREATE INDEX IF NOT EXISTS idx_package_version_status
  ON package_versions (status);
CREATE INDEX IF NOT EXISTS idx_package_version_version
  ON package_versions (version);
CREATE INDEX IF NOT EXISTS idx_package_version_namespace
  ON package_versions (namespace);
