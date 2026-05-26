-- Nexus-style catalog: Component (installable unit) + Asset (file) + Blob (bytes)
-- One-shot migration: legacy packages/* tables are migrated at startup then dropped.

CREATE TABLE IF NOT EXISTS blobs (
    id INTEGER PRIMARY KEY AUTOINCREMENT,
    ref VARCHAR(500) NOT NULL UNIQUE,
    sha256 VARCHAR(64),
    md5 VARCHAR(32),
    size_bytes BIGINT DEFAULT 0,
    storage_backend_id INTEGER,
    created_at DATETIME
);

CREATE TABLE IF NOT EXISTS components (
    id INTEGER PRIMARY KEY AUTOINCREMENT,
    repository_id INTEGER NOT NULL,
    format VARCHAR(20) NOT NULL,
    namespace VARCHAR(255) DEFAULT '',
    name VARCHAR(500) NOT NULL,
    version VARCHAR(255) NOT NULL,
    display_name VARCHAR(255),
    description VARCHAR(500),
    status VARCHAR(20) DEFAULT 'published',
    published_at DATETIME,
    published_by INTEGER,
    metadata TEXT,
    license VARCHAR(100),
    download_count BIGINT DEFAULT 0,
    size_bytes BIGINT DEFAULT 0,
    files_downloaded BOOLEAN DEFAULT 0,
    created_by INTEGER,
    created_at DATETIME,
    updated_at DATETIME,
    UNIQUE (repository_id, format, namespace, name, version)
);

CREATE TABLE IF NOT EXISTS assets (
    id INTEGER PRIMARY KEY AUTOINCREMENT,
    component_id INTEGER NOT NULL,
    path VARCHAR(500) DEFAULT '',
    file_name VARCHAR(255) NOT NULL,
    kind VARCHAR(20) NOT NULL,
    content_type VARCHAR(100),
    blob_id INTEGER NOT NULL,
    download_count BIGINT DEFAULT 0,
    download_url VARCHAR(500),
    created_at DATETIME,
    updated_at DATETIME,
    UNIQUE (component_id, path)
);

CREATE TABLE IF NOT EXISTS component_dependencies (
    id INTEGER PRIMARY KEY AUTOINCREMENT,
    component_id INTEGER NOT NULL,
    dep_name VARCHAR(255) NOT NULL,
    dep_version_constraint VARCHAR(255) NOT NULL,
    dep_type VARCHAR(50) NOT NULL,
    package_type VARCHAR(20) NOT NULL,
    is_optional BOOLEAN DEFAULT 0
);

CREATE INDEX IF NOT EXISTS idx_comp_repo_coords ON components(repository_id, format, namespace, name, version);
CREATE INDEX IF NOT EXISTS idx_comp_download_count ON components(download_count);
CREATE INDEX IF NOT EXISTS idx_asset_comp ON assets(component_id);
CREATE INDEX IF NOT EXISTS idx_blob_sha256 ON blobs(sha256);
