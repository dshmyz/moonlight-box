#!/bin/bash

set -e

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
SKIP_COUNT=0

pass() {
    echo -e "  ${GREEN}✓ PASS${NC} $1"
    PASS_COUNT=$((PASS_COUNT + 1))
}

fail() {
    echo -e "  ${RED}✗ FAIL${NC} $1"
    FAIL_COUNT=$((FAIL_COUNT + 1))
}

info() {
    echo -e "  ${BLUE}ℹ INFO${NC} $1"
}

warn() {
    echo -e "  ${YELLOW}⚠ WARN${NC} $1"
    WARN_COUNT=$((WARN_COUNT + 1))
}

get_auth_token() {
    curl -s -X POST "$BASE_URL/api/v1/auth/login" \
        -H "Content-Type: application/json" \
        -d "{\"username\":\"$ADMIN_USER\",\"password\":\"$ADMIN_PASS\"}" | \
        grep -o '"access_token":"[^"]*"' | \
        sed 's/"access_token":"//;s/"//'
}

repo_exists() {
    local repo_name="$1"
    local http_code=$(curl -s -o /dev/null -w "%{http_code}" \
        -H "Authorization: Bearer $TOKEN" \
        "$BASE_URL/api/v1/repositories/$repo_name")
    [ "$http_code" = "200" ]
}

create_local_repo() {
    local name="$1"
    local package_type="$2"
    local description="$3"

    if repo_exists "$name"; then
        info "$name 已存在，跳过"
        SKIP_COUNT=$((SKIP_COUNT + 1))
        return 0
    fi

    local response=$(curl -s -w "\n%{http_code}" -X POST "$BASE_URL/api/v1/repositories" \
        -H "Authorization: Bearer $TOKEN" \
        -H "Content-Type: application/json" \
        -d "{
            \"name\": \"$name\",
            \"type\": \"local\",
            \"package_type\": \"$package_type\",
            \"format\": \"$package_type\",
            \"description\": \"$description\",
            \"enabled\": true,
            \"config\": {\"allow_redeployment\": true}
        }")

    local http_code=$(echo "$response" | tail -n1)
    local body=$(echo "$response" | sed '$d')

    if [ "$http_code" = "200" ] || [ "$http_code" = "201" ]; then
        pass "$name (local) 创建成功"
    else
        fail "$name (local) 创建失败: HTTP $http_code - $body"
    fi
}

create_proxy_repo() {
    local name="$1"
    local package_type="$2"
    local remote_url="$3"
    local description="$4"

    if repo_exists "$name"; then
        info "$name 已存在，跳过"
        SKIP_COUNT=$((SKIP_COUNT + 1))
        return 0
    fi

    local response=$(curl -s -w "\n%{http_code}" -X POST "$BASE_URL/api/v1/repositories" \
        -H "Authorization: Bearer $TOKEN" \
        -H "Content-Type: application/json" \
        -d "{
            \"name\": \"$name\",
            \"type\": \"proxy\",
            \"package_type\": \"$package_type\",
            \"format\": \"$package_type\",
            \"description\": \"$description\",
            \"enabled\": true,
            \"cache_enabled\": true,
            \"cache_ttl_seconds\": 3600,
            \"config\": {\"remote_url\": \"$remote_url\"}
        }")

    local http_code=$(echo "$response" | tail -n1)
    local body=$(echo "$response" | sed '$d')

    if [ "$http_code" = "200" ] || [ "$http_code" = "201" ]; then
        pass "$name (proxy) 创建成功 -> $remote_url"
    else
        fail "$name (proxy) 创建失败: HTTP $http_code - $body"
    fi
}

echo "╔══════════════════════════════════════════════════════════╗"
echo "║  测试仓库初始化脚本                                    ║"
echo "╚══════════════════════════════════════════════════════════╝"
echo ""
echo "  目标地址: ${BLUE}$BASE_URL${NC}"
echo "  管理员: ${BLUE}$ADMIN_USER${NC}"
echo ""

echo "═══════════════════════════════════════════════════════════"
echo "  1/3  获取认证令牌"
echo "═══════════════════════════════════════════════════════════"

TOKEN=$(get_auth_token)
if [ -z "$TOKEN" ]; then
    echo -e "${RED}错误: 无法获取认证令牌，请检查用户名密码或服务是否启动${NC}"
    exit 1
fi
pass "获取认证令牌成功"

echo ""
echo "═══════════════════════════════════════════════════════════"
echo "  2/3  创建 Local 仓库"
echo "═══════════════════════════════════════════════════════════"

create_local_repo "maven-local" "maven" "Maven 本地仓库 - 测试用"
create_local_repo "npm-local" "npm" "npm 本地仓库 - 测试用"
create_local_repo "pypi-local" "pypi" "PyPI 本地仓库 - 测试用"
create_local_repo "go-local" "go" "Go Modules 本地仓库 - 测试用"
create_local_repo "generic-local" "generic" "Generic 本地仓库 - 测试用"

echo ""
echo "═══════════════════════════════════════════════════════════"
echo "  3/3  创建 Proxy 仓库 (国内镜像加速)"
echo "═══════════════════════════════════════════════════════════"

create_proxy_repo "maven-proxy-aliyun" "maven" "https://maven.aliyun.com/repository/public" "Maven 代理 - 阿里云镜像"
create_proxy_repo "npm-proxy-cn" "npm" "https://registry.npmmirror.com" "npm 代理 - 淘宝镜像"
create_proxy_repo "pypi-proxy-tuna" "pypi" "https://pypi.tuna.tsinghua.edu.cn" "PyPI 代理 - 清华镜像"
create_proxy_repo "go-proxy-goproxy-cn" "go" "https://goproxy.cn" "Go Modules 代理 - goproxy.cn"
create_proxy_repo "yum-proxy-baseos" "yum" "https://mirrors.aliyun.com/centos-stream/9-stream/BaseOS/x86_64/os" "YUM 代理 - CentOS Stream BaseOS 阿里云镜像"

echo ""
echo "╔══════════════════════════════════════════════════════════╗"
echo "║  执行结果                                               ║"
echo "╚══════════════════════════════════════════════════════════╝"
echo ""
echo "  ${GREEN}成功创建: $PASS_COUNT${NC}"
echo "  ${YELLOW}已存在跳过: $SKIP_COUNT${NC}"
echo "  ${RED}失败: $FAIL_COUNT${NC}"
echo "  ${YELLOW}警告: $WARN_COUNT${NC}"
echo ""

if [ $FAIL_COUNT -eq 0 ]; then
    echo -e "${GREEN}✓ 所有测试仓库初始化完成！${NC}"
    echo ""
    echo "  现在可以运行完整测试："
    echo "    bash scripts/run_all_tests.sh"
    exit 0
else
    echo -e "${RED}✗ 部分仓库创建失败，请检查上面的错误信息${NC}"
    exit 1
fi
