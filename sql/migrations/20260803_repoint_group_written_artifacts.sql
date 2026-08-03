-- ============================================================
-- 修复：组合（virtual）仓库误归属的 artifact
-- ============================================================
--
-- 背景
-- ----
-- 修复前（2026-08-03 之前），向组合仓库发布包时：
--   - 写路径：artifact 被打了【组合仓库】的 repository_id（Plugin 用 ctx.Repository.ID）
--   - 读路径：GroupRuntime 转发到 hosted 成员，HostedRuntime 强制用【成员】的
--             repository_id 查询
--   结果：发布返回 201，但 artifacts / packages / package_versions 的行归属成了
--         组合仓库 ID，下载时按成员 ID 查不到 → 404。
--
-- 服务端已在上游修复（HostedUploadSession 写路径强制归到成员 ID），本脚本用于
-- 修复【已经写坏】的历史数据。
--
-- 策略
-- ----
-- 1) 对每个组合仓库，选出第一个 type='local' 的成员作为"写入目标"
--    （与 cmd/registry/runtime_init.go 中 GroupRuntime.Writable 的选择一致）。
-- 2) 把 repository_id = 组合仓库ID 的 artifacts 行移动到该成员：
--      - 成员上不存在同 identity_key 的行 → 直接 UPDATE 改 repository_id。
--      - 成员上已存在同 identity_key 的行 → 说明是重复发布，删除组合仓库侧的行
--        （blob 为 CAS 共享，物理数据不丢，以成员已有记录为准）。
-- 3) 同步修复 packages / package_versions 聚合表（同为 repository_id 归属）。
--
-- 适用数据库：SQLite（默认开发库）
--   MySQL / PostgreSQL 需把 `:=` 变量语法改为相应数据库的临时表写法；
--   也可参考本文件最后的"通用临时表方案"备注。
-- ============================================================

-- ------------------------------------------------------------
-- 0) 预检：查看所有被误归属到组合仓库的 artifact 行
--    （type='virtual' ⇒ 该仓库本身从不合法持有 artifact，全是误写）
-- ------------------------------------------------------------
-- 说明：正常架构下组合仓库的 artifacts 表里不应有任何行；
--       此处若查不到数据，说明不存在需要修复的历史数据，可跳过后续步骤。
SELECT a.id, a.repository_id AS wrong_repo, r.name AS wrong_repo_name,
       a.format, a.name, a.version, a.remote_path
FROM artifacts a
JOIN repositories r ON r.id = a.repository_id
WHERE r.type = 'virtual'
ORDER BY a.repository_id, a.name;

-- ------------------------------------------------------------
-- 1) 构建"组合仓库 → 写入目标成员"的映射（取首个 local 成员）
-- ------------------------------------------------------------
-- runtime_init.go 里 Writable 取的是第一个 type='local' 的成员；
-- 因此这里按 position 升序取第一个 local 成员作为目标。
-- 注意：取的是 m.member_id（目标仓库 ID），不是 repository_members 的 id。
DROP TABLE IF EXISTS tmp_group_target;
CREATE TEMP TABLE tmp_group_target AS
SELECT rm.repository_id AS virtual_id,
       (SELECT m.member_id
          FROM repository_members m
          JOIN repositories mr ON mr.id = m.member_id
         WHERE m.repository_id = rm.repository_id
           AND mr.type = 'local'
         ORDER BY m.position ASC, m.id ASC
         LIMIT 1)      AS member_id
FROM (SELECT DISTINCT repository_id FROM repository_members) rm;

-- 组合仓库没有任何 local 成员时，member_id 为 NULL；
-- 这种情况无法自动判定目标，需人工处理（下方步骤会用 SQLite 语法排除 NULL）。

-- ------------------------------------------------------------
-- 2) 迁移 artifacts：成员无冲突 → 移动；有冲突 → 删除组合侧重复行
-- ------------------------------------------------------------

-- 2a) 先删除"目标成员已存在同 identity_key"的重复行（含其 blob 关联）
--     避免后续 UPDATE 触发唯一索引冲突。
DELETE FROM artifact_blobs
WHERE artifact_id IN (
  SELECT ga.id
  FROM artifacts ga
  JOIN tmp_group_target t ON t.virtual_id = ga.repository_id
  JOIN artifacts ma ON ma.repository_id = t.member_id
                    AND ma.identity_key = ga.identity_key
  WHERE t.member_id IS NOT NULL
    AND ga.id <> ma.id
);

DELETE FROM artifacts
WHERE id IN (
  SELECT ga.id
  FROM artifacts ga
  JOIN tmp_group_target t ON t.virtual_id = ga.repository_id
  JOIN artifacts ma ON ma.repository_id = t.member_id
                    AND ma.identity_key = ga.identity_key
  WHERE t.member_id IS NOT NULL
    AND ga.id <> ma.id
);

-- 2b) 移动无冲突的 artifact 行到目标成员
UPDATE artifacts
SET repository_id = (
  SELECT t.member_id
  FROM tmp_group_target t
  WHERE t.virtual_id = artifacts.repository_id
    AND t.member_id IS NOT NULL
)
WHERE repository_id IN (SELECT virtual_id FROM tmp_group_target)
  AND (SELECT t.member_id FROM tmp_group_target t
       WHERE t.virtual_id = artifacts.repository_id) IS NOT NULL;

-- 2c) 处理仍未归属的（组合仓库没有 local 成员）—— 应人工介入
--     这里只把剩余的组合仓库归属行列出，不做任何自动移动。
SELECT 'NEEDS-MANUAL' AS action, a.id, a.repository_id, a.name, a.remote_path
FROM artifacts a
JOIN repositories r ON r.id = a.repository_id
WHERE r.type = 'virtual'
  AND NOT EXISTS (SELECT 1 FROM tmp_group_target t WHERE t.virtual_id = a.repository_id AND t.member_id IS NOT NULL);

-- ------------------------------------------------------------
-- 3) 修复聚合表 packages / package_versions
--    （它们以 artifacts 为源，按 repository_id 归属包）
-- ------------------------------------------------------------

-- 3a) packages：目标成员上有同名同 format 的包 →
--     合并（删除组合侧行，保留成员行）；无冲突 → 移动
-- 先删除组合侧与成员冲突的 packages 行（成员行是权威）
DELETE FROM packages
WHERE id IN (
  SELECT gp.id
  FROM packages gp
  JOIN tmp_group_target t ON t.virtual_id = gp.repository_id
  JOIN packages mp ON mp.repository_id = t.member_id
                   AND mp.format = gp.format
                   AND mp.name   = gp.name
  WHERE t.member_id IS NOT NULL
    AND gp.id <> mp.id
);

-- 再移动无冲突的 packages 行
UPDATE packages
SET repository_id = (
  SELECT t.member_id FROM tmp_group_target t
  WHERE t.virtual_id = packages.repository_id AND t.member_id IS NOT NULL
)
WHERE repository_id IN (SELECT virtual_id FROM tmp_group_target)
  AND (SELECT t.member_id FROM tmp_group_target t
       WHERE t.virtual_id = packages.repository_id) IS NOT NULL;

-- 3b) package_versions：目标成员上有同名同 format 同 version 的行 →
--     合并（删除组合侧）；无冲突 → 移动
DELETE FROM package_versions
WHERE id IN (
  SELECT gv.id
  FROM package_versions gv
  JOIN tmp_group_target t ON t.virtual_id = gv.repository_id
  JOIN package_versions mv ON mv.repository_id = t.member_id
                           AND mv.format = gv.format
                           AND mv.package_name = gv.package_name
                           AND mv.version = gv.version
  WHERE t.member_id IS NOT NULL
    AND gv.id <> mv.id
);

UPDATE package_versions
SET repository_id = (
  SELECT t.member_id FROM tmp_group_target t
  WHERE t.virtual_id = package_versions.repository_id AND t.member_id IS NOT NULL
)
WHERE repository_id IN (SELECT virtual_id FROM tmp_group_target)
  AND (SELECT t.member_id FROM tmp_group_target t
       WHERE t.virtual_id = package_versions.repository_id) IS NOT NULL;

-- ------------------------------------------------------------
-- 4) 清理临时表
-- ------------------------------------------------------------
DROP TABLE IF EXISTS tmp_group_target;

-- ============================================================
-- 验证（修复后应返回 0 行）
-- ------------------------------------------------------------
-- SELECT COUNT(*) FROM artifacts
--   JOIN repositories r ON r.id = artifacts.repository_id
--   WHERE r.type = 'virtual';
--
-- 若有历史包走到组合仓库但没有 local 成员（临时表看不到属于哪个），
-- 请手工确认目标成员后再 UPDATE。
-- ============================================================

-- ============================================================
-- 备注：MySQL / PostgreSQL 通用临时表方案
-- ------------------------------------------------------------
-- 本脚本直接使用 SQLite 的 TEMP TABLE（CREATE TEMP TABLE）即可，
-- MySQL / PostgreSQL 同样支持 TEMPORARY TABLE，但需把第 2a/2b/3a/3b 步的
-- UPDATE ... SET repository_id = (SELECT ...) 改写为 JOIN 式更新
-- （MySQL: UPDATE ... JOIN；PG: UPDATE ... FROM）。
-- 只要保证"先删冲突重复行、再移动无冲突行"的顺序，逻辑一致即可。
-- ============================================================
