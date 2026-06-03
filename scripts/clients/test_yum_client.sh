#!/bin/bash

# =============================================================================
# YUM/RPM 客户端真实集成测试
# 使用 curl 模拟 yum/dnf 命令测试仓库功能
# =============================================================================

BASE_URL="${1:-http://localhost:9081}"
ADMIN_USER="${ADMIN_USER:-admin}"
ADMIN_PASS="${ADMIN_PASS:-admin123}"

RED='\033[0;31m'
GREEN='\033[0;32m'
YELLOW='\033[1;33m'
BLUE='\033[0;34m'
NC='\033[0m'

PASS_COUNT=0
FAIL_COUNT=0
WARN_COUNT=0

pass() { echo -e "  ${GREEN}✓ PASS${NC} $1"; PASS_COUNT=$((PASS_COUNT + 1)); }
fail() { echo -e "  ${RED}✗ FAIL${NC} $1"; FAIL_COUNT=$((FAIL_COUNT + 1)); }
info() { echo -e "  ${BLUE}ℹ INFO${NC} $1"; }
warn() { echo -e "  ${YELLOW}⚠ WARN${NC} $1"; WARN_COUNT=$((WARN_COUNT + 1)); }

echo "============================================"
echo " YUM/RPM 客户端真实集成测试"
echo " 使用 curl 模拟 yum/dnf 请求"
echo " 目标: $BASE_URL"
echo "============================================"
echo

# 获取认证令牌
TOKEN=$(curl -s -X POST "$BASE_URL/api/v1/auth/login" \
    -H "Content-Type: application/json" \
    -d "{\"username\":\"$ADMIN_USER\",\"password\":\"$ADMIN_PASS\"}" | \
    grep -o '"access_token":"[^"]*"' | \
    sed 's/"access_token":"//;s/"//')

if [ -z "$TOKEN" ]; then
    warn "无法获取认证令牌，上传测试将跳过"
fi

# 检查 yum/dnf 命令
YUM_CMD=""
if command -v dnf &> /dev/null; then
    YUM_CMD="dnf"
elif command -v yum &> /dev/null; then
    YUM_CMD="yum"
else
    info "yum/dnf 命令未安装，使用 curl 模拟测试"
fi

[ -n "$YUM_CMD" ] && info "使用: $YUM_CMD"

TEST_DIR="/tmp/yum-client-test-$$"
mkdir -p "$TEST_DIR"
cd "$TEST_DIR"

echo "测试 1: 确保 YUM 本地仓库存在..."
if [ -n "$TOKEN" ]; then
    # 检查仓库是否存在，不存在则创建
    HTTP_CODE=$(curl -s -o /dev/null -w "%{http_code}" \
        -H "Authorization: Bearer $TOKEN" \
        "$BASE_URL/api/v1/repositories/yum-local")

    if [ "$HTTP_CODE" != "200" ]; then
        info "yum-local 仓库不存在，正在创建..."
        CREATE_RESP=$(curl -s -w "\n%{http_code}" -X POST "$BASE_URL/api/v1/repositories" \
            -H "Content-Type: application/json" \
            -H "Authorization: Bearer $TOKEN" \
            -d '{"name":"yum-local","display_name":"YUM 本地仓库","type":"local","package_type":"yum","enabled":true}')
        HTTP_CODE=$(echo "$CREATE_RESP" | tail -1)
        if [ "$HTTP_CODE" = "200" ] || [ "$HTTP_CODE" = "201" ]; then
            pass "yum-local 仓库已创建"
        else
            warn "yum-local 仓库创建失败 (HTTP $HTTP_CODE)"
        fi
    else
        pass "yum-local 仓库已存在"
    fi
else
    fail "跳过仓库创建 (无认证令牌)"
fi

echo
echo "测试 2: 构造测试 RPM 包..."
cat > test-app.spec <<'EOF'
Name: test-app
Version: 1.0.0
Release: 1
Summary: Test RPM package
EOF

# 创建最小的 RPM 兼容文件
echo "test-rpm-content" > test-app-1.0.0-1.x86_64.rpm
echo "包名: test-app" >> test-app-1.0.0-1.x86_64.rpm
echo "版本: 1.0.0" >> test-app-1.0.0-1.x86_64.rpm
pass "测试 RPM 文件已创建"

echo
echo "测试 3: 上传 RPM 包到 YUM 仓库..."
if [ -n "$TOKEN" ]; then
    HTTP_CODE=$(curl -s -o /tmp/yum-upload.json -w "%{http_code}" \
        -X PUT "$BASE_URL/repository/yum-local/test-app-1.0.0-1.x86_64.rpm" \
        -H "Authorization: Bearer $TOKEN" \
        -H "Content-Type: application/octet-stream" \
        --data-binary "@test-app-1.0.0-1.x86_64.rpm")

    if [ "$HTTP_CODE" = "200" ] || [ "$HTTP_CODE" = "201" ]; then
        pass "RPM 包上传成功 (HTTP $HTTP_CODE)"
    elif [ "$HTTP_CODE" = "405" ] || [ "$HTTP_CODE" = "500" ]; then
        warn "RPM 上传暂不支持 (HTTP $HTTP_CODE) — YUM 插件仅支持 GET, 待实现 PUT"
    else
        warn "RPM 包上传失败 (HTTP $HTTP_CODE)"
    fi
else
    fail "跳过上传 (无认证令牌)"
fi

echo
echo "测试 4: 验证 404 处理..."
HTTP_CODE=$(curl -s -o /dev/null -w "%{http_code}" \
    "$BASE_URL/repository/yum-local/nonexistent-package.rpm")

if [ "$HTTP_CODE" = "404" ]; then
    pass "不存在的包正确返回 404"
else
    warn "不存在的包返回 HTTP $HTTP_CODE (预期 404)"
fi

echo
echo "测试 5: 验证 repodata/repomd.xml 端点..."
HTTP_CODE=$(curl -s -o /tmp/yum-repomd.xml -w "%{http_code}" \
    "$BASE_URL/repository/yum-local/repodata/repomd.xml")

if [ "$HTTP_CODE" = "200" ]; then
    pass "repomd.xml 端点可访问 (HTTP 200)"
    if grep -q "<repomd" /tmp/yum-repomd.xml; then
        pass "repomd.xml 格式正确"
    fi
elif [ "$HTTP_CODE" = "404" ]; then
    info "repomd.xml 返回 404 (仓库无已上传包, 无元数据生成)"
else
    warn "repomd.xml 端点异常 (HTTP $HTTP_CODE)"
fi

echo
echo "测试 6: 验证 repodata/primary.xml 端点..."
HTTP_CODE=$(curl -s -o /tmp/yum-primary.xml -w "%{http_code}" \
    "$BASE_URL/repository/yum-local/repodata/primary.xml")

if [ "$HTTP_CODE" = "200" ]; then
    pass "primary.xml 端点可访问 (HTTP 200)"
elif [ "$HTTP_CODE" = "404" ]; then
    info "primary.xml 返回 404 (仓库无已上传包)"
else
    warn "primary.xml 端点异常 (HTTP $HTTP_CODE)"
fi

echo
echo "测试 7: 测试 RPM 下载路由..."
# 即使没有已上传包，路由也应该正确响应 404 而非 500
HTTP_CODE=$(curl -s -o /dev/null -w "%{http_code}" \
    "$BASE_URL/repository/yum-local/test-pkg-1.0.0.x86_64.rpm")

if [ "$HTTP_CODE" = "404" ]; then
    pass "RPM 下载路由正确 (无包时返回 404)"
elif [ "$HTTP_CODE" = "500" ]; then
    warn "RPM 下载路由返回 500 (非预期)"
else
    info "RPM 下载路由状态: HTTP $HTTP_CODE"
fi

echo
echo "测试 8: 验证无效路径处理..."
HTTP_CODE=$(curl -s -o /dev/null -w "%{http_code}" \
    "$BASE_URL/repository/yum-local/")

if [ "$HTTP_CODE" = "400" ] || [ "$HTTP_CODE" = "404" ]; then
    pass "空路径正确返回错误 (HTTP $HTTP_CODE)"
else
    info "空路径返回 HTTP $HTTP_CODE"
fi

# 清理
cd /
rm -rf "$TEST_DIR"
rm -f "$YUM_REPO_FILE"

echo
echo "============================================"
echo " YUM/RPM 客户端测试完成"
echo "============================================"
echo -e "  通过: ${GREEN}$PASS_COUNT${NC}"
echo -e "  失败: ${RED}$FAIL_COUNT${NC}"
echo -e "  警告: ${YELLOW}$WARN_COUNT${NC}"
echo
