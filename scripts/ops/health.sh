#!/bin/bash
# 健康检查脚本
# 用法: ./ops/health.sh [BASE_URL]

set -euo pipefail

BASE_URL="${1:-http://localhost:8080}"
ERRORS=0

check() {
    local name="$1" url="$2" expect_code="${3:-200}"
    shift 3 || shift $#
    local code
    code=$(curl -s -o /dev/null -w "%{http_code}" "$@" "$url" 2>/dev/null || echo "000")
    if [ "$code" = "$expect_code" ]; then
        echo "  ✓ $name (HTTP $code)"
    else
        echo "  ✗ $name (HTTP $code, expected $expect_code)"
        ERRORS=$((ERRORS + 1))
    fi
}

echo "=== Health Check: $BASE_URL ==="

echo ""
echo "[基本连通性]"
check "健康检查" "$BASE_URL/health"
check "Ping" "$BASE_URL/api/v1/ping"

echo ""
echo "[公共 API]"
check "包搜索" "$BASE_URL/api/v1/packages/search?name=test&page=1&page_size=1"
check "仓库列表" "$BASE_URL/api/v1/public/repositories"

echo ""
echo "[管理后台]"
TOKEN=$(curl -s -X POST "$BASE_URL/api/v1/auth/login" \
    -H "Content-Type: application/json" \
    -d '{"username":"admin","password":"admin123"}' 2>/dev/null \
    | grep -o '"access_token":"[^"]*"' | head -1 | cut -d'"' -f4)

if [ -z "$TOKEN" ]; then
    echo "  ✗ 无法获取 Token，跳过认证接口检查"
    ERRORS=$((ERRORS + 1))
else
    check "仪表盘" "$BASE_URL/api/v1/dashboard/stats" 200 -H "Authorization: Bearer $TOKEN"
    check "仓库管理" "$BASE_URL/api/v1/repositories" 200 -H "Authorization: Bearer $TOKEN"
    check "用户管理" "$BASE_URL/api/v1/users" 200 -H "Authorization: Bearer $TOKEN"
fi

echo ""
echo "[存储]"
# 检查存储目录是否可写
STORAGE_DIR="${STORAGE_DIR:-./data}"
if [ -d "$STORAGE_DIR" ]; then
    if touch "$STORAGE_DIR/.health_check" 2>/dev/null; then
        echo "  ✓ 存储目录可写 ($STORAGE_DIR)"
        rm -f "$STORAGE_DIR/.health_check"
    else
        echo "  ✗ 存储目录不可写 ($STORAGE_DIR)"
        ERRORS=$((ERRORS + 1))
    fi
else
    echo "  ✗ 存储目录不存在 ($STORAGE_DIR)"
    ERRORS=$((ERRORS + 1))
fi

echo ""
echo "==========================="
if [ $ERRORS -eq 0 ]; then
    echo "所有检查通过 ✓"
    exit 0
else
    echo "$ERRORS 项检查失败 ✗"
    exit 1
fi
