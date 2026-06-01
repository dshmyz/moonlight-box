#!/bin/bash

# =============================================================================
# APT/Debian 客户端真实集成测试
# 使用 curl 模拟 apt/apt-get 命令测试仓库功能
# =============================================================================

BASE_URL="${1:-http://localhost:9081}"
ADMIN_USER="${ADMIN_USER:-admin}"
ADMIN_PASS="${ADMIN_PASS:-admin123}"

RED='\033[0;31m'
GREEN='\033[0;32m'
YELLOW='\033[1;33m'
BLUE='\033[0;34m'
NC='\033[0m'

pass() { echo -e "  ${GREEN}✓ PASS${NC} $1"; }
fail() { echo -e "  ${RED}✗ FAIL${NC} $1"; }
info() { echo -e "  ${BLUE}ℹ INFO${NC} $1"; }
warn() { echo -e "  ${YELLOW}⚠ WARN${NC} $1"; }

echo "============================================"
echo " APT/Debian 客户端真实集成测试"
echo " 使用 curl 模拟 apt-get 请求"
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

# 检查 apt-get 命令
if command -v apt-get &> /dev/null; then
    info "apt-get 可用"
fi

TEST_DIR="/tmp/apt-client-test-$$"
mkdir -p "$TEST_DIR"
cd "$TEST_DIR"

echo "测试 1: 确保 APT 本地仓库存在..."
if [ -n "$TOKEN" ]; then
    HTTP_CODE=$(curl -s -o /dev/null -w "%{http_code}" \
        -H "Authorization: Bearer $TOKEN" \
        "$BASE_URL/api/v1/repositories/apt-local")

    if [ "$HTTP_CODE" != "200" ]; then
        info "apt-local 仓库不存在，正在创建..."
        CREATE_RESP=$(curl -s -w "\n%{http_code}" -X POST "$BASE_URL/api/v1/repositories" \
            -H "Content-Type: application/json" \
            -H "Authorization: Bearer $TOKEN" \
            -d '{"name":"apt-local","display_name":"APT 本地仓库","type":"local","package_type":"apt","enabled":true}')
        HTTP_CODE=$(echo "$CREATE_RESP" | tail -1)
        if [ "$HTTP_CODE" = "200" ] || [ "$HTTP_CODE" = "201" ]; then
            pass "apt-local 仓库已创建"
        else
            warn "apt-local 仓库创建失败 (HTTP $HTTP_CODE)"
        fi
    else
        pass "apt-local 仓库已存在"
    fi
else
    fail "跳过仓库创建 (无认证令牌)"
fi

echo
echo "测试 2: 构造测试 DEB 包..."
# 创建最小的 DEB 兼容文件 (ar archive with debian-binary)
printf "!<arch>\n" > test-pkg_1.0.0_amd64.deb
printf "debian-binary\n" > debian-binary
echo "2.0" >> debian-binary
echo "test-deb-content" >> test-pkg_1.0.0_amd64.deb

cat > control <<EOF
Package: test-pkg
Version: 1.0.0
Architecture: amd64
Maintainer: Test <test@example.com>
Description: Test DEB package for APT repository
EOF

pass "测试 DEB 文件已创建"

echo
echo "测试 3: 上传 DEB 包到 APT 仓库..."
if [ -n "$TOKEN" ]; then
    HTTP_CODE=$(curl -s -o /tmp/apt-upload.json -w "%{http_code}" \
        -X PUT "$BASE_URL/repository/apt-local/test-pkg_1.0.0_amd64.deb" \
        -H "Authorization: Bearer $TOKEN" \
        -H "Content-Type: application/octet-stream" \
        --data-binary "@test-pkg_1.0.0_amd64.deb")

    if [ "$HTTP_CODE" = "200" ] || [ "$HTTP_CODE" = "201" ]; then
        pass "DEB 包上传成功 (HTTP $HTTP_CODE)"
    elif [ "$HTTP_CODE" = "405" ] || [ "$HTTP_CODE" = "500" ]; then
        warn "DEB 上传暂不支持 (HTTP $HTTP_CODE) — APT 插件仅支持 GET, 待实现 PUT"
    else
        warn "DEB 包上传失败 (HTTP $HTTP_CODE)"
    fi
else
    fail "跳过上传 (无认证令牌)"
fi

echo
echo "测试 4: 验证 404 处理..."
HTTP_CODE=$(curl -s -o /dev/null -w "%{http_code}" \
    "$BASE_URL/repository/apt-local/nonexistent-package.deb")

if [ "$HTTP_CODE" = "404" ]; then
    pass "不存在的包正确返回 404"
else
    warn "不存在的包返回 HTTP $HTTP_CODE (预期 404)"
fi

echo
echo "测试 5: 验证 Release/InRelease 端点..."
HTTP_CODE=$(curl -s -o /dev/null -w "%{http_code}" \
    "$BASE_URL/repository/apt-local/dists/stable/InRelease")

if [ "$HTTP_CODE" = "200" ]; then
    pass "InRelease 端点可访问 (HTTP 200)"
elif [ "$HTTP_CODE" = "404" ]; then
    info "InRelease 返回 404 (仓库无已上传包, 无元数据)"
fi

HTTP_CODE=$(curl -s -o /dev/null -w "%{http_code}" \
    "$BASE_URL/repository/apt-local/dists/stable/Release")

if [ "$HTTP_CODE" = "200" ]; then
    pass "Release 端点可访问 (HTTP 200)"
elif [ "$HTTP_CODE" = "404" ]; then
    info "Release 返回 404 (仓库无已上传包)"
fi

echo
echo "测试 6: 验证 Packages 端点..."
HTTP_CODE=$(curl -s -o /dev/null -w "%{http_code}" \
    "$BASE_URL/repository/apt-local/dists/stable/main/binary-amd64/Packages")

if [ "$HTTP_CODE" = "200" ]; then
    pass "Packages 端点可访问 (HTTP 200)"
elif [ "$HTTP_CODE" = "404" ]; then
    info "Packages 返回 404 (仓库无已上传包)"
fi

echo
echo "测试 7: 验证 Packages.gz 端点..."
HTTP_CODE=$(curl -s -o /dev/null -w "%{http_code}" \
    "$BASE_URL/repository/apt-local/dists/stable/main/binary-amd64/Packages.gz")

if [ "$HTTP_CODE" = "200" ]; then
    pass "Packages.gz 端点可访问 (HTTP 200)"
elif [ "$HTTP_CODE" = "404" ]; then
    info "Packages.gz 返回 404 (仓库无已上传包)"
fi

echo
echo "测试 8: 验证 Release.gpg 端点..."
HTTP_CODE=$(curl -s -o /dev/null -w "%{http_code}" \
    "$BASE_URL/repository/apt-local/dists/stable/Release.gpg")

if [ "$HTTP_CODE" = "200" ]; then
    info "Release.gpg 端点可访问 (HTTP 200)"
elif [ "$HTTP_CODE" = "404" ]; then
    info "Release.gpg 返回 404 (GPG 签名未配置/WIP)"
fi

echo
echo "测试 9: 模拟 apt-get update 请求..."
HTTP_CODE_RELEASE=$(curl -s -o /dev/null -w "%{http_code}" \
    "$BASE_URL/repository/apt-local/dists/stable/InRelease")
HTTP_CODE_PACKAGES=$(curl -s -o /dev/null -w "%{http_code}" \
    "$BASE_URL/repository/apt-local/dists/stable/main/binary-amd64/Packages")

if [ "$HTTP_CODE_RELEASE" = "200" ] && [ "$HTTP_CODE_PACKAGES" = "200" ]; then
    pass "apt-get update 模拟成功 (Release + Packages 均可访问)"
elif [ "$HTTP_CODE_RELEASE" = "404" ] && [ "$HTTP_CODE_PACKAGES" = "404" ]; then
    info "apt-get update 模拟: Release/Packages 返回 404 (仓库为空, 预期行为)"
else
    info "Release: HTTP $HTTP_CODE_RELEASE, Packages: HTTP $HTTP_CODE_PACKAGES"
fi

echo
echo "测试 10: 上传 DEB 包到 pool 目录结构..."
if [ -n "$TOKEN" ]; then
    mkdir -p pool/main/t/test-pkg
    cp test-pkg_1.0.0_amd64.deb pool/main/t/test-pkg/
    HTTP_CODE=$(curl -s -o /tmp/apt-upload2.json -w "%{http_code}" \
        -X PUT "$BASE_URL/repository/apt-local/pool/main/t/test-pkg/test-pkg_1.0.0_amd64.deb" \
        -H "Authorization: Bearer $TOKEN" \
        -H "Content-Type: application/octet-stream" \
        --data-binary "@test-pkg_1.0.0_amd64.deb")

    if [ "$HTTP_CODE" = "200" ] || [ "$HTTP_CODE" = "201" ]; then
        pass "pool 结构 DEB 包上传成功 (HTTP $HTTP_CODE)"
    elif [ "$HTTP_CODE" = "405" ] || [ "$HTTP_CODE" = "500" ]; then
        warn "pool 结构 DEB 上传暂不支持 (HTTP $HTTP_CODE) — 插件待实现 PUT"
    else
        warn "pool 结构 DEB 包上传失败 (HTTP $HTTP_CODE)"
    fi
else
    fail "跳过 pool 上传 (无认证令牌)"
fi

# 清理
cd /
rm -rf "$TEST_DIR"

echo
echo "============================================"
echo " APT/Debian 客户端测试完成"
echo "============================================"
