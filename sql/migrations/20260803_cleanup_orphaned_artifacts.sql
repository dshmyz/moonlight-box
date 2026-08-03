-- ============================================================
-- 清理：指向已不存在仓库的孤儿行（artifacts / packages / package_versions / repository_members）
-- ============================================================
--
-- 背景
-- ----
-- 删除仓库时（internal/repository/repository_repo.go 的 Delete）只删了 repositories
-- 这一行，没有清理它名下的数据。于是 artifacts / packages / package_versions /
-- repository_members 里残留了大量 repository_id 指向已不存在仓库的行。
--
-- 包管理查询（searchFromArtifactsGrouped / listByVersionMatch）按
-- (repository_id, format, name) 分组，每个孤儿 repo_id 都会让同一个包多显示一行
-- （且查不到仓库名、显示为空），导致"包管理查询里出现重复包"。
--
-- 本脚本只做行级清理，不触碰 blob 物理文件 —— blob 是 CAS 共享的，删 DB 行不会
-- 影响仍被其它（存活的）artifact 引用的 blob；彻底回收未引用 blob 应由单独的
-- 垃圾回收任务负责。
--
-- 适用数据库：SQLite（默认开发库）。MySQL/PostgreSQL 语法基本一致（DELETE 子查询，
-- 无 SQLite 特有语法），可直接使用。
-- ============================================================

-- ------------------------------------------------------------
-- 预检：看看各表有多少悬空行（修复后应全部为 0）
-- ------------------------------------------------------------
-- SELECT
--   (SELECT COUNT(*) FROM artifacts a LEFT JOIN repositories r ON r.id=a.repository_id WHERE r.id IS NULL) AS orphan_artifacts,
--   (SELECT COUNT(*) FROM packages  p LEFT JOIN repositories r ON r.id=p.repository_id      WHERE r.id IS NULL) AS orphan_packages,
--   (SELECT COUNT(*) FROM package_versions v LEFT JOIN repositories r ON r.id=v.repository_id WHERE r.id IS NULL) AS orphan_versions;

-- ------------------------------------------------------------
-- 1) artifact_blobs：清理「引用了不存在 artifact」以及「artifact 属于不存在仓库」的行
--    —— 必须先于 artifacts 删除执行，否则删除 artifacts 后这些关联行会变成新孤儿
-- ------------------------------------------------------------
DELETE FROM artifact_blobs
WHERE artifact_id IN (
  SELECT a.id FROM artifacts a
  LEFT JOIN repositories r ON r.id = a.repository_id
  WHERE r.id IS NULL
   OR NOT EXISTS (SELECT 1 FROM artifacts a2 WHERE a2.id = a.id)
);

-- ------------------------------------------------------------
-- 2) artifacts：清理引用已不存在仓库的行
-- ------------------------------------------------------------
DELETE FROM artifacts
WHERE repository_id IN (
  SELECT a.repository_id FROM artifacts a
  LEFT JOIN repositories r ON r.id = a.repository_id
  WHERE r.id IS NULL
);

-- ------------------------------------------------------------
-- 3) packages / package_versions：清理引用已不存在仓库的行
-- ------------------------------------------------------------
DELETE FROM packages
WHERE repository_id IN (
  SELECT p.repository_id FROM packages p
  LEFT JOIN repositories r ON r.id = p.repository_id
  WHERE r.id IS NULL
);

DELETE FROM package_versions
WHERE repository_id IN (
  SELECT v.repository_id FROM package_versions v
  LEFT JOIN repositories r ON r.id = v.repository_id
  WHERE r.id IS NULL
);

-- ------------------------------------------------------------
-- 4) repository_members：清理引用了已不存在仓库的行（虚拟仓库的归档成员）
--    —— 用 IN 子查询，同时覆盖 repository_id 或 member_id 悬空两种情况
-- ------------------------------------------------------------
DELETE FROM repository_members
WHERE id IN (
  SELECT rm.id FROM repository_members rm
  LEFT JOIN repositories r1 ON r1.id = rm.repository_id
  LEFT JOIN repositories r2 ON r2.id = rm.member_id
  WHERE r1.id IS NULL OR r2.id IS NULL
);

-- ============================================================
-- 验证（修复后应全部返回 0）
-- ------------------------------------------------------------
-- SELECT
--   (SELECT COUNT(*) FROM artifacts a LEFT JOIN repositories r ON r.id=a.repository_id      WHERE r.id IS NULL) AS orphan_artifacts,
--   (SELECT COUNT(*) FROM packages  p LEFT JOIN repositories r ON r.id=p.repository_id      WHERE r.id IS NULL) AS orphan_packages,
--   (SELECT COUNT(*) FROM package_versions v LEFT JOIN repositories r ON r.id=v.repository_id WHERE r.id IS NULL) AS orphan_versions,
--   (SELECT COUNT(*) FROM repository_members rm LEFT JOIN repositories r ON r.id=rm.repository_id WHERE r.id IS NULL) AS orphan_members;
-- ============================================================
