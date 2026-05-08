-- 性能优化迁移脚本 - P0-1: 统一数据类型
-- 将 download_count 字段从 INT 统一为 BIGINT，防止数据溢出
-- 执行时间：2026-05-08

-- 1. 修改 package_versions 表的 download_count 字段
ALTER TABLE `package_versions` 
MODIFY COLUMN `download_count` BIGINT NOT NULL DEFAULT 0;

-- 2. 修改 package_files 表的 download_count 字段
ALTER TABLE `package_files` 
MODIFY COLUMN `download_count` BIGINT NOT NULL DEFAULT 0;

-- 验证迁移结果
SELECT 
  TABLE_NAME, 
  COLUMN_NAME, 
  COLUMN_TYPE 
FROM INFORMATION_SCHEMA.COLUMNS 
WHERE TABLE_SCHEMA = DATABASE() 
  AND COLUMN_NAME = 'download_count' 
  AND TABLE_NAME IN ('packages', 'package_versions', 'package_files', 'repositories')
ORDER BY TABLE_NAME;
