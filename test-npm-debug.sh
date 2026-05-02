#!/bin/bash

BASE="http://localhost:9081"
DB="/Users/gracegaoya/work/project/moonlight-box/data/registry.db"

echo "测试NPM包下载并检查版本号"
echo "========================================="

echo "1. 下载lodash@4.17.21"
HTTP=$(curl -s -o /dev/null -w "%{http_code}" "${BASE}/repo/npm-proxy-cn/lodash/-/lodash-4.17.21.tgz")
echo "HTTP状态码: $HTTP"

echo ""
echo "2. 检查数据库中的包记录"
PACKAGES=$(sqlite3 $DB "SELECT id, name, type FROM packages WHERE type = 'npm'")
echo "NPM包记录: $PACKAGES"

echo ""
echo "3. 检查数据库中的版本记录"
VERSIONS=$(sqlite3 $DB "SELECT p.name, pv.version, pv.storage_path FROM packages p JOIN package_versions pv ON p.id = pv.package_id WHERE p.type = 'npm'")
echo "NPM版本记录: $VERSIONS"

echo ""
echo "4. 检查存储目录"
ls -la /Users/gracegaoya/work/project/moonlight-box/data/packages/npm/ 2>/dev/null || echo "NPM存储目录不存在"

echo ""
echo "5. 检查所有包记录"
ALL_PACKAGES=$(sqlite3 $DB "SELECT id, name, type FROM packages LIMIT 10")
echo "所有包记录: $ALL_PACKAGES"
