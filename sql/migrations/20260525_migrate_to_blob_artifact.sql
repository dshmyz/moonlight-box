-- 从旧架构迁移到新架构的数据迁移脚本
-- 迁移日期：2026-05-25
-- 说明：此脚本将旧架构的 packages/package_versions 数据迁移到新架构的 artifacts 表

-- ========================================
-- 1. 迁移 packages + package_versions -> artifacts + blobs
-- ========================================

-- 1.1 迁移 packages 数据到 artifacts
INSERT INTO artifacts (repository_id, format, kind, coordinates, metadata, created_at, updated_at)
SELECT
    p.repository_id,
    p.type AS format,
    'published' AS kind,
    JSON_OBJECT('name', p.name, 'display_name', p.display_name, 'description', p.description) AS coordinates,
    JSON_OBJECT('homepage', p.homepage, 'license', p.license) AS metadata,
    p.created_at,
    p.updated_at
FROM packages p
WHERE NOT EXISTS (
    SELECT 1 FROM artifacts a
    WHERE a.repository_id = p.repository_id
    AND JSON_EXTRACT(a.coordinates, '$.name') = p.name
);

-- 1.2 迁移 package_versions 到 artifact_versions
INSERT INTO artifact_versions (artifact_id, version, normalized, created_at)
SELECT
    a.id AS artifact_id,
    pv.version,
    pv.version AS normalized,
    pv.published_at
FROM package_versions pv
JOIN packages p ON pv.package_id = p.id
JOIN artifacts a ON a.repository_id = p.repository_id
    AND JSON_EXTRACT(a.coordinates, '$.name') = p.name
WHERE NOT EXISTS (
    SELECT 1 FROM artifact_versions av
    WHERE av.artifact_id = a.id AND av.version = pv.version
);

-- 1.3 创建 blobs 记录（基于 package_files）
INSERT INTO blobs (algorithm, digest, size, storage_path, created_at)
SELECT DISTINCT
    'sha256' AS algorithm,
    COALESCE(pv.checksum_sha256, SHA2(CONCAT(pf.filename, pf.storage_path), 256)) AS digest,
    pf.size_bytes AS size,
    pf.storage_path,
    pv.published_at AS created_at
FROM package_files pf
JOIN package_versions pv ON pf.version_id = pv.id
JOIN packages p ON pv.package_id = p.id
WHERE NOT EXISTS (
    SELECT 1 FROM blobs b
    WHERE b.storage_path = pf.storage_path
);

-- 1.4 创建 artifact_blobs 关联
INSERT INTO artifact_blobs (artifact_id, blob_id, position, role)
SELECT
    a.id AS artifact_id,
    b.id AS blob_id,
    0 AS position,
    'primary' AS role
FROM package_files pf
JOIN package_versions pv ON pf.version_id = pv.id
JOIN packages p ON pv.package_id = p.id
JOIN artifacts a ON a.repository_id = p.repository_id
    AND JSON_EXTRACT(a.coordinates, '$.name') = p.name
JOIN blobs b ON b.storage_path = pf.storage_path
WHERE NOT EXISTS (
    SELECT 1 FROM artifact_blobs ab
    WHERE ab.artifact_id = a.id AND ab.blob_id = b.id
);

-- ========================================
-- 2. 更新 download_count
-- ========================================

-- 从 packages 表同步 download_count 到 artifacts
UPDATE artifacts a
JOIN (
    SELECT p.id, SUM(pv.download_count) as total_downloads
    FROM packages p
    JOIN package_versions pv ON pv.package_id = p.id
    GROUP BY p.id
) downloads ON JSON_EXTRACT(a.coordinates, '$.name') = (
    SELECT name FROM packages WHERE id = (
        SELECT package_id FROM package_versions WHERE id IN (
            SELECT version_id FROM package_files WHERE storage_path IN (
                SELECT storage_path FROM blobs WHERE id IN (
                    SELECT blob_id FROM artifact_blobs WHERE artifact_id = a.id
                )
            )
        )
    )
);

-- ========================================
-- 3. 清理旧数据（可选，建议先备份）
-- ========================================

-- 注意：以下操作会删除旧数据，建议先备份或确认数据已正确迁移

-- 3.1 清理依赖关系（如果需要）
-- DELETE FROM package_dependencies;

-- 3.2 清理文件记录（如果需要）
-- DELETE FROM package_files;

-- 3.3 清理版本记录（如果需要）
-- DELETE FROM package_versions;

-- 3.4 清理包记录（如果需要）
-- DELETE FROM packages;

-- ========================================
-- 4. 验证迁移结果
-- ========================================

-- 查看迁移统计
SELECT 'packages' AS table_name, COUNT(*) AS record_count FROM packages
UNION ALL
SELECT 'package_versions', COUNT(*) FROM package_versions
UNION ALL
SELECT 'artifacts', COUNT(*) FROM artifacts
UNION ALL
SELECT 'blobs', COUNT(*) FROM blobs
UNION ALL
SELECT 'artifact_versions', COUNT(*) FROM artifact_versions
UNION ALL
SELECT 'artifact_blobs', COUNT(*) FROM artifact_blobs;
