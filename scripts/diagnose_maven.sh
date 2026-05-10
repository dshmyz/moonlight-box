#!/bin/bash
# 诊断 Maven 仓库配置问题
# 检查数据库中是否有代理仓库的 RemoteURL 为空

DB_PATH="/Users/gracegaoya/work/project/moonlight-box/data/registry.db"

echo "============================================"
echo " Maven 仓库配置诊断"
echo "============================================"
echo

echo "1. 检查所有 Maven 仓库的配置："
echo "----------------------------------------"
sqlite3 "$DB_PATH" "SELECT id, name, type, remote_url FROM repositories WHERE package_type = 'maven' ORDER BY id;"

echo
echo "2. 检查是否有代理仓库的 RemoteURL 为空："
echo "----------------------------------------"
EMPTY_COUNT=$(sqlite3 "$DB_PATH" "SELECT COUNT(*) FROM repositories WHERE package_type = 'maven' AND type = 'proxy' AND (remote_url IS NULL OR remote_url = '');")

if [ "$EMPTY_COUNT" -gt 0 ]; then
    echo "⚠️  发现 $EMPTY_COUNT 个代理仓库的 RemoteURL 为空！"
    echo
    echo "问题仓库："
    sqlite3 "$DB_PATH" "SELECT id, name, type, remote_url FROM repositories WHERE package_type = 'maven' AND type = 'proxy' AND (remote_url IS NULL OR remote_url = '');"
    echo
    echo "修复方法："
    echo "----------------------------------------"
    echo "方案 1: 更新 RemoteURL"
    echo "  sqlite3 \"$DB_PATH\" \"UPDATE repositories SET remote_url = 'https://repo.maven.apache.org/maven2' WHERE id = <仓库ID>;\""
    echo
    echo "方案 2: 删除问题仓库"
    echo "  sqlite3 \"$DB_PATH\" \"DELETE FROM repositories WHERE id = <仓库ID>;\""
    echo
    echo "方案 3: 禁用问题仓库"
    echo "  sqlite3 \"$DB_PATH\" \"UPDATE repositories SET enabled = 0 WHERE id = <仓库ID>;\""
else
    echo "✓ 所有代理仓库的 RemoteURL 都已配置"
fi

echo
echo "3. 检查虚拟仓库的成员配置："
echo "----------------------------------------"
sqlite3 "$DB_PATH" "SELECT r.name as virtual_repo, m.name as member_repo, m.type as member_type, rg.priority 
FROM repository_groups rg 
JOIN repositories r ON rg.virtual_repo_id = r.id 
JOIN repositories m ON rg.member_repo_id = m.id 
WHERE r.package_type = 'maven' 
ORDER BY r.name, rg.priority;"

echo
echo "4. 检查最近的下载日志："
echo "----------------------------------------"
sqlite3 "$DB_PATH" "SELECT repo_name, package_name, version, source, error_message, created_at 
FROM proxy_download_logs 
WHERE package_type = 'maven' 
ORDER BY created_at DESC 
LIMIT 10;"

echo
echo "============================================"
echo " 诊断完成"
echo "============================================"
