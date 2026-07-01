-- SQLite migration for the package_versions rebuildable read model.
-- Source of truth remains artifacts.

CREATE TABLE IF NOT EXISTS package_versions (
  id INTEGER PRIMARY KEY AUTOINCREMENT,
  repository_id INTEGER NOT NULL,
  format TEXT NOT NULL,
  package_name TEXT NOT NULL,
  namespace TEXT DEFAULT '',
  version TEXT NOT NULL,
  status TEXT NOT NULL DEFAULT 'published',
  published_at DATETIME NULL,
  latest_artifact_at DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP,
  file_count INTEGER NOT NULL DEFAULT 0,
  files_downloaded INTEGER NOT NULL DEFAULT 0,
  size_bytes INTEGER NOT NULL DEFAULT 0,
  download_count INTEGER NOT NULL DEFAULT 0,
  license TEXT DEFAULT '',
  checksum_sha256 TEXT DEFAULT '',
  created_at DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP,
  updated_at DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP,
  UNIQUE (repository_id, format, package_name, version)
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
