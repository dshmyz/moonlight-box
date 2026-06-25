-- 清理迁移产生的静态 maven-metadata.xml artifact 记录
-- 这些记录是派生数据，应通过运行时动态聚合生成，避免覆盖目标库更新的 metadata
-- 执行时间：2026-06-25

-- 查看待清理的数据（先执行这条确认范围）
-- SELECT id, repository_id, name, version, remote_path, created_at
-- FROM artifacts
-- WHERE format = 'maven'
--   AND kind = 'metadata'
--   AND remote_path LIKE '%/maven-metadata.xml';

-- 查看待清理的 checksum 记录（maven-metadata.xml.sha1 / .md5）
-- SELECT id, repository_id, name, version, remote_path, created_at
-- FROM artifacts
-- WHERE format = 'maven'
--   AND kind = 'checksum'
--   AND remote_path LIKE '%/maven-metadata.xml.sha1'
--   OR remote_path LIKE '%/maven-metadata.xml.md5';

-- ========================================
-- 执行清理
-- ========================================

-- 1. 删除 metadata artifact 的 blob 关联
DELETE FROM artifact_blobs
WHERE artifact_id IN (
  SELECT id FROM artifacts
  WHERE format = 'maven'
    AND kind = 'metadata'
    AND remote_path LIKE '%/maven-metadata.xml'
);

-- 2. 删除 maven-metadata.xml 的 checksum artifact 的 blob 关联
DELETE FROM artifact_blobs
WHERE artifact_id IN (
  SELECT id FROM artifacts
  WHERE format = 'maven'
    AND kind = 'checksum'
    AND (remote_path LIKE '%/maven-metadata.xml.sha1'
         OR remote_path LIKE '%/maven-metadata.xml.md5'
         OR remote_path LIKE '%/maven-metadata.xml.sha256')
);

-- 3. 删除 metadata artifact 记录
DELETE FROM artifacts
WHERE format = 'maven'
  AND kind = 'metadata'
  AND remote_path LIKE '%/maven-metadata.xml';

-- 4. 删除 maven-metadata.xml 的 checksum artifact 记录
DELETE FROM artifacts
WHERE format = 'maven'
  AND kind = 'checksum'
  AND (remote_path LIKE '%/maven-metadata.xml.sha1'
       OR remote_path LIKE '%/maven-metadata.xml.md5'
       OR remote_path LIKE '%/maven-metadata.xml.sha256');

-- 注意：blob 本身不删除，CAS 去重存储，由 GC 统一回收
-- 注意：packages 聚合表不受影响，metadata kind 被 IsCatalogExcludedKind 排除，不参与 version_count 计算
