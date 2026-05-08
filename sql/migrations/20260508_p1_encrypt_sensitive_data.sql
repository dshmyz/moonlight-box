-- 性能优化迁移脚本 - P1-3: 加密敏感数据
-- 将 migration_tasks 表中的 password 字段改为 password_encrypted
-- 执行时间：2026-05-08

-- 1. 添加新字段
ALTER TABLE `migration_tasks` 
ADD COLUMN `password_encrypted` TEXT NOT NULL DEFAULT '' AFTER `username`;

-- 2. 迁移现有数据（注意：如果现有数据是明文密码，需要应用层处理加密）
-- 这里只是复制数据，实际加密需要在应用层完成
UPDATE `migration_tasks` 
SET `password_encrypted` = `password` 
WHERE `password` != '';

-- 3. 删除旧字段（确认迁移完成后再执行）
-- ALTER TABLE `migration_tasks` DROP COLUMN `password`;

-- 验证迁移结果
SELECT 
  COLUMN_NAME, 
  COLUMN_TYPE, 
  IS_NULLABLE 
FROM INFORMATION_SCHEMA.COLUMNS 
WHERE TABLE_SCHEMA = DATABASE() 
  AND TABLE_NAME = 'migration_tasks' 
  AND COLUMN_NAME IN ('username', 'password', 'password_encrypted')
ORDER BY ORDINAL_POSITION;
