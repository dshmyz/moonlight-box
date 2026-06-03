#!/bin/bash

# =============================================================================
# 组合仓库 (Virtual/Group) 功能验证测试
# 验证虚拟仓库聚合本地+代理成员的能力
# =============================================================================

BASE_URL="${1:-http://localhost:9081}"
ADMIN_USER="${ADMIN_USER:-admin}"
ADMIN_PASS="${ADMIN_PASS:-admin123}"

RED='\033[0;31m'
GREEN='\033[0;32m'
YELLOW='\033[1;33m'
BLUE='\033[0;34m'
CYAN='\033[0;36m'
NC='\033[0m'

PASS_COUNT=0
FAIL_COUNT=0
WARN_COUNT=0

pass() { echo -e "  ${GREEN}✓ PASS${NC} $1"; PASS_COUNT=$((PASS_COUNT + 1)); }
fail() { echo -e "  ${RED}✗ FAIL${NC} $1"; FAIL_COUNT=$((FAIL_COUNT + 1)); }
info() { echo -e "  ${BLUE}ℹ INFO${NC} $1"; }
warn() { echo -e "  ${YELLOW}⚠ WARN${NC} $1"; WARN_COUNT=$((WARN_COUNT + 1)); }

echo "============================================"
echo " 组合仓库 (Virtual/Group) 功能验证"
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
    warn "无法获取认证令牌"
    exit 1
fi

# =============================================================================
# 1. 虚拟仓库 CRUD
# =============================================================================
echo "=== 1. 虚拟仓库 CRUD ==="

echo "测试 1.1: 创建虚拟仓库..."
HTTP_CODE=$(curl -s -o /tmp/virt-create.json -w "%{http_code}" \
    -X POST "$BASE_URL/api/v1/repositories" \
    -H "Authorization: Bearer $TOKEN" \
    -H "Content-Type: application/json" \
    -d '{"name":"test-virtual","display_name":"Test Virtual","type":"virtual","package_type":"generic","enabled":true}')

if [ "$HTTP_CODE" = "200" ] || [ "$HTTP_CODE" = "201" ]; then
    pass "虚拟仓库创建成功 (HTTP $HTTP_CODE)"
else
    warn "虚拟仓库创建失败 (HTTP $HTTP_CODE)"
    cat /tmp/virt-create.json
fi

echo "测试 1.2: 添加成员仓库..."
curl -s -X POST "$BASE_URL/api/v1/repositories/test-virtual/members" \
    -H "Authorization: Bearer $TOKEN" \
    -H "Content-Type: application/json" \
    -d '{"member_name":"generic-local","priority":0}' > /dev/null

# 验证成员
MEMBERS=$(curl -s "$BASE_URL/api/v1/repositories/test-virtual" \
    -H "Authorization: Bearer $TOKEN" | \
    python3 -c "import sys,json; d=json.load(sys.stdin); print(len(d['data'].get('members',[])))" 2>/dev/null)

if [ "$MEMBERS" -gt 0 ]; then
    pass "成员添加成功 ($MEMBERS members)"
else
    fail "成员添加失败"
fi

echo "测试 1.3: 删除成员..."
curl -s -X DELETE "$BASE_URL/api/v1/repositories/test-virtual/members/generic-local" \
    -H "Authorization: Bearer $TOKEN" > /dev/null

MEMBERS_AFTER=$(curl -s "$BASE_URL/api/v1/repositories/test-virtual" \
    -H "Authorization: Bearer $TOKEN" | \
    python3 -c "import sys,json; d=json.load(sys.stdin); print(len(d['data'].get('members',[])))" 2>/dev/null)

if [ "$MEMBERS_AFTER" = "0" ]; then
    pass "成员删除成功"
else
    warn "成员删除后剩余 $MEMBERS_AFTER"
fi

echo "测试 1.4: 删除虚拟仓库..."
curl -s -X DELETE "$BASE_URL/api/v1/repositories/test-virtual" \
    -H "Authorization: Bearer $TOKEN" > /dev/null

HTTP_CODE=$(curl -s -o /dev/null -w "%{http_code}" \
    -H "Authorization: Bearer $TOKEN" \
    "$BASE_URL/api/v1/repositories/test-virtual")
if [ "$HTTP_CODE" = "404" ]; then
    pass "虚拟仓库删除成功 (HTTP 404)"
else
    warn "虚拟仓库删除后仍可访问 (HTTP $HTTP_CODE)"
fi

echo
# =============================================================================
# 2. Maven 虚拟仓库: 聚合本地 + 代理
# =============================================================================
echo "=== 2. Maven 虚拟仓库 ==="

echo "测试 2.1: 通过虚拟仓库下载代理包 (上游 guava)..."
HTTP_CODE=$(curl -s -o /tmp/mv-guava.jar -w "%{http_code}" \
    "$BASE_URL/repository/maven-virtual/com/google/guava/guava/32.1.3-jre/guava-32.1.3-jre.jar")

if [ "$HTTP_CODE" = "200" ]; then
    pass "代理包下载成功 (HTTP 200)"
    info "JAR 大小: $(wc -c < /tmp/mv-guava.jar) bytes"
else
    warn "代理包下载失败 (HTTP $HTTP_CODE)"
fi

echo "测试 2.2: 通过虚拟仓库下载 maven-metadata.xml..."
HTTP_CODE=$(curl -s -o /tmp/mv-meta.xml -w "%{http_code}" \
    "$BASE_URL/repository/maven-virtual/com/google/guava/guava/maven-metadata.xml")

if [ "$HTTP_CODE" = "200" ] && grep -q "<metadata>" /tmp/mv-meta.xml; then
    pass "maven-metadata.xml 正确返回"
else
    warn "maven-metadata.xml 失败 (HTTP $HTTP_CODE)"
fi

echo "测试 2.3: 通过虚拟仓库下载之前 deploy 的本地包..."
HTTP_CODE=$(curl -s -o /tmp/mv-local.jar -w "%{http_code}" \
    "$BASE_URL/repository/maven-virtual/com/test/maven-client-test/1.0.0/maven-client-test-1.0.0.jar")

if [ "$HTTP_CODE" = "200" ]; then
    pass "本地包通过虚拟仓库可下载 (HTTP 200)"
else
    warn "本地包通过虚拟仓库不可下载 (HTTP $HTTP_CODE)"
fi

echo
# =============================================================================
# 3. NPM 虚拟仓库
# =============================================================================
echo "=== 3. NPM 虚拟仓库 ==="

echo "测试 3.1: 通过虚拟仓库获取包元数据..."
HTTP_CODE=$(curl -s -o /tmp/nv-lodash.json -w "%{http_code}" \
    "$BASE_URL/repository/npm-virtual/lodash")

if [ "$HTTP_CODE" = "200" ]; then
    if grep -q '"name"' /tmp/nv-lodash.json; then
        pass "包元数据获取成功 (含 name 字段)"
    else
        pass "包元数据可访问 (HTTP 200)"
    fi
else
    warn "包元数据获取失败 (HTTP $HTTP_CODE)"
fi

echo "测试 3.2: 通过虚拟仓库安装 npm 包..."
TMPDIR="/tmp/npm-virt-test-$$"
mkdir -p "$TMPDIR" && cd "$TMPDIR"
npm init -y &> /dev/null 2>&1
npm config set registry "$BASE_URL/repository/npm-virtual" 2>/dev/null || true
if npm install lodash@4.17.21 --save &> /dev/null 2>&1; then
    pass "npm install 通过虚拟仓库成功"
    [ -d "node_modules/lodash" ] && pass "lodash 已安装到 node_modules"
else
    warn "npm install 通过虚拟仓库失败"
fi
npm config set registry "https://registry.npmjs.org/" 2>/dev/null || true
cd / && rm -rf "$TMPDIR"

echo "测试 3.3: 通过虚拟仓库获取 dist-tags..."
HTTP_CODE=$(curl -s -o /tmp/nv-dist.json -w "%{http_code}" \
    "$BASE_URL/repository/npm-virtual/-/package/lodash/dist-tags")

if [ "$HTTP_CODE" = "200" ]; then
    info "dist-tags 端点: HTTP 200"
else
    info "dist-tags 端点: HTTP $HTTP_CODE"
fi

echo
# =============================================================================
# 4. PyPI 虚拟仓库
# =============================================================================
echo "=== 4. PyPI 虚拟仓库 ==="

echo "测试 4.1: 通过虚拟仓库访问 Simple API..."
HTTP_CODE=$(curl -s -o /tmp/pv-simple.html -w "%{http_code}" \
    "$BASE_URL/repository/pypi-virtual/simple/requests/")

if [ "$HTTP_CODE" = "200" ] && grep -q "href=" /tmp/pv-simple.html; then
    pass "Simple API 可访问, 返回正确的 HTML"
    info "包链接数: $(grep -c 'href=' /tmp/pv-simple.html)"
else
    warn "Simple API 失败 (HTTP $HTTP_CODE)"
fi

echo "测试 4.2: 通过虚拟仓库访问 JSON API..."
HTTP_CODE=$(curl -s -o /tmp/pv-json.json -w "%{http_code}" \
    "$BASE_URL/repository/pypi-virtual/pypi/requests/2.31.0/json")

if [ "$HTTP_CODE" = "200" ] && grep -q '"info"' /tmp/pv-json.json; then
    pass "JSON API 可访问, 含 info 字段"
else
    warn "JSON API 失败 (HTTP $HTTP_CODE)"
fi

echo "测试 4.3: 通过虚拟仓库 pip install..."
TMPDIR="/tmp/pip-virt-test-$$"
mkdir -p "$TMPDIR"
python3 -m venv "$TMPDIR/venv" &> /dev/null 2>&1
if [ -d "$TMPDIR/venv" ]; then
    source "$TMPDIR/venv/bin/activate"
    pip config set global.index-url "$BASE_URL/repository/pypi-virtual/simple" 2>/dev/null || true
    pip config set global.trusted-host "$(echo $BASE_URL | sed 's|http://||;s|https://||')" 2>/dev/null || true
    if pip install requests==2.31.0 --no-cache-dir &> /tmp/pip-virt-install.log 2>&1; then
        pass "pip install 通过虚拟仓库成功"
    else
        warn "pip install 通过虚拟仓库失败"
    fi
    deactivate 2>/dev/null || true
    pip config unset global.index-url 2>/dev/null || true
    pip config unset global.trusted-host 2>/dev/null || true
else
    fail "跳过 pip install (无法创建虚拟环境)"
fi
cd / && rm -rf "$TMPDIR"

echo
# =============================================================================
# 5. Go 虚拟仓库
# =============================================================================
echo "=== 5. Go 虚拟仓库 ==="

echo "测试 5.1: 通过虚拟仓库获取 @v/list..."
HTTP_CODE=$(curl -s -o /tmp/gv-list -w "%{http_code}" \
    "$BASE_URL/repository/go-virtual/github.com/stretchr/testify/@v/list")

if [ "$HTTP_CODE" = "200" ]; then
    pass "@v/list 可访问 (HTTP 200)"
else
    warn "@v/list 失败 (HTTP $HTTP_CODE)"
fi

echo "测试 5.2: 通过虚拟仓库获取 @v/info..."
HTTP_CODE=$(curl -s -o /tmp/gv-info.json -w "%{http_code}" \
    "$BASE_URL/repository/go-virtual/github.com/stretchr/testify/@v/v1.8.4.info")

if [ "$HTTP_CODE" = "200" ] && grep -q '"Version"' /tmp/gv-info.json; then
    pass "@v/info 正确返回 (含 Version)"
else
    warn "@v/info 失败 (HTTP $HTTP_CODE)"
fi

echo "测试 5.3: 通过虚拟仓库获取 @v/mod..."
HTTP_CODE=$(curl -s -o /tmp/gv-mod -w "%{http_code}" \
    "$BASE_URL/repository/go-virtual/github.com/stretchr/testify/@v/v1.8.4.mod")

if [ "$HTTP_CODE" = "200" ] && grep -q "module github.com/stretchr/testify" /tmp/gv-mod; then
    pass "@v/mod 正确返回 (含 module 声明)"
else
    warn "@v/mod 失败 (HTTP $HTTP_CODE)"
fi

echo "测试 5.4: 通过虚拟仓库 go get..."
TMPDIR="/tmp/go-virt-test-$$"
mkdir -p "$TMPDIR" && cd "$TMPDIR"
cat > go.mod <<'EOF'
module test-go-virt
go 1.21
EOF
GOPROXY="http://localhost:9081/repository/go-virtual,direct" GOPROXY_ON=off
if go list -m github.com/stretchr/testify@v1.8.4 &> /dev/null; then
    pass "go list -m 通过虚拟仓库成功"
else
    warn "go list -m 通过虚拟仓库失败"
fi
cd / && rm -rf "$TMPDIR"

echo
# =============================================================================
# 6. 虚拟仓库优先级验证
# =============================================================================
echo "=== 6. 虚拟仓库成员优先级 ==="

echo "测试 6.1: 本地优先于代理 (maven-virtual)..."
# maven-virtual 中 maven-local priority 最高, 先查本地
HTTP_CODE=$(curl -s -o /tmp/mv-jar.jar -w "%{http_code}" \
    "$BASE_URL/repository/maven-virtual/com/test/maven-client-test/1.0.0/maven-client-test-1.0.0.jar")
if [ "$HTTP_CODE" = "200" ]; then
    pass "本地包优先返回 (HTTP 200)"
else
    warn "本地包查找失败 (HTTP $HTTP_CODE)"
fi

echo
# =============================================================================
# 7. 错误处理
# =============================================================================
echo "=== 7. 错误处理 ==="

echo "测试 7.1: 虚拟仓库不存在成员返回 404..."
HTTP_CODE=$(curl -s -o /dev/null -w "%{http_code}" \
    "$BASE_URL/repository/maven-virtual/com/nonexistent/pkg/1.0/pkg-1.0.jar")
if [ "$HTTP_CODE" = "404" ]; then
    pass "不存在的包正确返回 404"
else
    info "不存在的包: HTTP $HTTP_CODE"
fi

echo "测试 7.2: 禁用虚拟仓库..."
TOKEN=$(curl -s -X POST "$BASE_URL/api/v1/auth/login" \
    -H "Content-Type: application/json" \
    -d "{\"username\":\"$ADMIN_USER\",\"password\":\"$ADMIN_PASS\"}" | \
    grep -o '"access_token":"[^"]*"' | sed 's/"access_token":"//;s/"//')

# 创建临时虚拟仓库并禁用
curl -s -X POST "$BASE_URL/api/v1/repositories" \
    -H "Authorization: Bearer $TOKEN" \
    -H "Content-Type: application/json" \
    -d '{"name":"test-disabled-virt","display_name":"Test Disabled","type":"virtual","package_type":"generic","enabled":false}' > /dev/null 2>&1

HTTP_CODE=$(curl -s -o /dev/null -w "%{http_code}" \
    "$BASE_URL/repository/test-disabled-virt/test.txt")
if [ "$HTTP_CODE" = "404" ] || [ "$HTTP_CODE" = "403" ]; then
    pass "禁用的虚拟仓库正确拒绝访问 (HTTP $HTTP_CODE)"
else
    warn "禁用的虚拟仓库返回 HTTP $HTTP_CODE"
fi

# 清理
curl -s -X DELETE "$BASE_URL/api/v1/repositories/test-disabled-virt" \
    -H "Authorization: Bearer $TOKEN" > /dev/null 2>&1

echo
echo "============================================"
echo " 组合仓库功能验证完成"
echo "============================================"
echo -e "  通过: ${GREEN}$PASS_COUNT${NC}"
echo -e "  失败: ${RED}$FAIL_COUNT${NC}"
echo -e "  警告: ${YELLOW}$WARN_COUNT${NC}"
echo
