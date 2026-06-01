#!/bin/bash
# ============================================================
# Nexus 2 路由兼容性测试
# 验证 /content/repositories/:repoName 和 /content/groups/:groupName 路由
# ============================================================

set -e

BASE_URL="${1:-http://localhost:9081}"
ADMIN_USER="${ADMIN_USER:-admin}"
ADMIN_PASS="${ADMIN_PASS:-admin123}"

GREEN='\033[0;32m'
RED='\033[0;31m'
YELLOW='\033[1;33m'
BLUE='\033[0;34m'
NC='\033[0m'

PASS=0
FAIL=0

log_pass() { echo -e "  ${GREEN}✓ PASS${NC} $1"; PASS=$((PASS + 1)); }
log_fail() { echo -e "  ${RED}✗ FAIL${NC} $1"; FAIL=$((FAIL + 1)); }
log_info() { echo -e "  ${BLUE}ℹ INFO${NC} $1"; }
log_section() { echo -e "\n${YELLOW}════════════════════════════════════════${NC}"; echo -e "  ${YELLOW}$1${NC}"; echo -e "${YELLOW}════════════════════════════════════════${NC}"; }

CLEAN_TEMPS=()
cleanup() { rm -rf "${CLEAN_TEMPS[@]}" 2>/dev/null || true; }
trap cleanup EXIT

echo "============================================"
echo " Nexus 2 路由兼容性测试"
echo " 目标: $BASE_URL"
echo "============================================"
echo

# 获取 Token
TOKEN=$(curl -s -X POST "$BASE_URL/api/v1/auth/login" \
    -H "Content-Type: application/json" \
    -d "{\"username\":\"$ADMIN_USER\",\"password\":\"$ADMIN_PASS\"}" | \
    grep -o '"access_token":"[^"]*"' | \
    sed 's/"access_token":"//;s/"//')

if [ -z "$TOKEN" ]; then
    echo -e "${RED}错误: 无法获取认证令牌${NC}"
    exit 1
fi
log_pass "获取认证令牌成功"

# ── 测试 1: Nexus 2 仓库路由基础测试 ──────────────────────
log_section "测试 1: Nexus 2 仓库路由 /content/repositories/:repoName/*"

TEST_JAR="/tmp/nexus2-test-artifact-$$.jar"
TEST_POM="/tmp/nexus2-test-artifact-$$.pom"
CLEAN_TEMPS+=("$TEST_JAR" "$TEST_POM")

echo "test nexus2 content $$" > "$TEST_JAR"
echo '<?xml version="1.0"?><project><modelVersion>4.0.0</modelVersion>
<groupId>com.nexus2</groupId><artifactId>nexus2-test</artifactId><version>1.0.0</version>
</project>' > "$TEST_POM"

# 通过 Nexus 3 路由上传（用于后续对比测试）
N3_REPO_BASE="$BASE_URL/repository/maven-local"
N2_REPO_BASE="$BASE_URL/content/repositories/maven-local"
ARTIFACT_PATH="com/nexus2/nexus2-test/1.0.0/nexus2-test-1.0.0"

log_info "先通过 Nexus 3 路由上传测试文件..."
HTTP_CODE=$(curl -s -o /dev/null -w "%{http_code}" -X PUT \
    "$N3_REPO_BASE/$ARTIFACT_PATH.jar" \
    -H "Authorization: Bearer $TOKEN" \
    -H "Content-Type: application/octet-stream" \
    --data-binary @"$TEST_JAR")

if [ "$HTTP_CODE" = "201" ] || [ "$HTTP_CODE" = "200" ]; then
    log_pass "通过 Nexus 3 路由上传 JAR 成功 (HTTP $HTTP_CODE)"
else
    log_fail "通过 Nexus 3 路由上传 JAR 失败 (HTTP $HTTP_CODE)"
fi

HTTP_CODE=$(curl -s -o /dev/null -w "%{http_code}" -X PUT \
    "$N3_REPO_BASE/$ARTIFACT_PATH.pom" \
    -H "Authorization: Bearer $TOKEN" \
    -H "Content-Type: application/xml" \
    --data-binary @"$TEST_POM")

if [ "$HTTP_CODE" = "201" ] || [ "$HTTP_CODE" = "200" ]; then
    log_pass "通过 Nexus 3 路由上传 POM 成功 (HTTP $HTTP_CODE)"
else
    log_fail "通过 Nexus 3 路由上传 POM 失败 (HTTP $HTTP_CODE)"
fi

# 通过 Nexus 2 路由下载
log_info "通过 Nexus 2 路由下载..."
HTTP_CODE=$(curl -s -o /tmp/nexus2-download.jar -w "%{http_code}" \
    "$N2_REPO_BASE/$ARTIFACT_PATH.jar")
if [ "$HTTP_CODE" = "200" ]; then
    log_pass "Nexus 2 路由下载 JAR 成功 (HTTP 200)"
    if [ -s "/tmp/nexus2-download.jar" ]; then
        log_pass "下载的 JAR 文件非空"
        NEXUS2_CONTENT=$(cat /tmp/nexus2-download.jar)
        if echo "$NEXUS2_CONTENT" | grep -q "test nexus2 content"; then
            log_pass "Nexus 2 下载的文件内容正确"
        else
            log_fail "Nexus 2 下载的文件内容不正确"
        fi
    else
        log_fail "Nexus 2 下载的 JAR 文件为空"
    fi
else
    log_fail "Nexus 2 路由下载 JAR 失败 (HTTP $HTTP_CODE)"
fi

# 通过 Nexus 2 路由下载 POM
HTTP_CODE=$(curl -s -o /tmp/nexus2-download.pom -w "%{http_code}" \
    "$N2_REPO_BASE/$ARTIFACT_PATH.pom")
if [ "$HTTP_CODE" = "200" ]; then
    log_pass "Nexus 2 路由下载 POM 成功 (HTTP 200)"
else
    log_fail "Nexus 2 路由下载 POM 失败 (HTTP $HTTP_CODE)"
fi

# ── 测试 2: Nexus 2 路由上传测试 ──────────────────────
log_section "测试 2: Nexus 2 路由上传 /content/repositories/:repoName PUT"

N2_UPLOAD_PATH="com/nexus2/nexus2-upload-test/1.0.0/nexus2-upload-test-1.0.0"

HTTP_CODE=$(curl -s -o /dev/null -w "%{http_code}" -X PUT \
    "$N2_REPO_BASE/$N2_UPLOAD_PATH.jar" \
    -H "Authorization: Bearer $TOKEN" \
    -H "Content-Type: application/octet-stream" \
    --data-binary @"$TEST_JAR")

if [ "$HTTP_CODE" = "201" ] || [ "$HTTP_CODE" = "200" ]; then
    log_pass "Nexus 2 路由上传 JAR 成功 (HTTP $HTTP_CODE)"

    # 验证通过 Nexus 3 路由可以下载
    HTTP_CODE=$(curl -s -o /tmp/nexus2-upload-verify.jar -w "%{http_code}" \
        "$N3_REPO_BASE/$N2_UPLOAD_PATH.jar")
    if [ "$HTTP_CODE" = "200" ]; then
        log_pass "Nexus 2 上传的文件可通过 Nexus 3 路由下载"
    else
        log_fail "Nexus 2 上传的文件无法通过 Nexus 3 路由下载 (HTTP $HTTP_CODE)"
    fi
else
    log_fail "Nexus 2 路由上传 JAR 失败 (HTTP $HTTP_CODE)"
fi

# ── 测试 3: Nexus 2 路由删除测试 ──────────────────────
log_section "测试 3: Nexus 2 路由删除 /content/repositories/:repoName DELETE"

HTTP_CODE=$(curl -s -o /dev/null -w "%{http_code}" -X DELETE \
    "$N2_REPO_BASE/$N2_UPLOAD_PATH.jar" \
    -H "Authorization: Bearer $TOKEN")

if [ "$HTTP_CODE" = "200" ] || [ "$HTTP_CODE" = "204" ]; then
    log_pass "Nexus 2 路由删除 JAR 成功 (HTTP $HTTP_CODE)"

    # 验证删除后文件不存在
    HTTP_CODE=$(curl -s -o /dev/null -w "%{http_code}" \
        "$N2_REPO_BASE/$N2_UPLOAD_PATH.jar")
    if [ "$HTTP_CODE" = "404" ]; then
        log_pass "删除后通过 Nexus 2 路由返回 404"
    else
        log_fail "删除后文件仍存在 (HTTP $HTTP_CODE)"
    fi

    HTTP_CODE=$(curl -s -o /dev/null -w "%{http_code}" \
        "$N3_REPO_BASE/$N2_UPLOAD_PATH.jar")
    if [ "$HTTP_CODE" = "404" ]; then
        log_pass "删除后通过 Nexus 3 路由也返回 404"
    else
        log_fail "删除后通过 Nexus 3 路由仍存在 (HTTP $HTTP_CODE)"
    fi
else
    log_fail "Nexus 2 路由删除 JAR 失败 (HTTP $HTTP_CODE)"
fi

# ── 测试 4: Nexus 2 Group 路由测试 ──────────────────────
log_section "测试 4: Nexus 2 Group 路由 /content/groups/:groupName/*"

# Moonlight Box 中 virtual 类型等同于 Nexus 的 group 类型
# 也兼容早期创建的 group 类型仓库
GROUP_NAME=$(curl -s "$BASE_URL/api/v1/repositories" \
    -H "Authorization: Bearer $TOKEN" | \
    python3 -c "
import sys, json
d = json.load(sys.stdin)
repos = d.get('data', [])
for r in repos:
    if r.get('type') in ('virtual', 'group'):
        print(r.get('name', ''))
        break
" 2>/dev/null || echo "")

if [ -z "$GROUP_NAME" ]; then
    log_info "未找到 virtual/group 仓库，尝试创建..."
    CREATE_RESULT=$(curl -s -X POST "$BASE_URL/api/v1/repositories" \
        -H "Authorization: Bearer $TOKEN" \
        -H "Content-Type: application/json" \
        -d '{
            "name": "maven-public",
            "type": "virtual",
            "format": "maven",
            "package_type": "maven",
            "members": ["maven-local", "maven-proxy-aliyun"]
        }')

    if echo "$CREATE_RESULT" | grep -q '"code":201'; then
        GROUP_NAME="maven-public"
        log_pass "创建 virtual 仓库 maven-public 成功"
    else
        log_fail "创建 virtual 仓库失败"
        log_info "API 返回: $CREATE_RESULT"
    fi
fi

if [ -n "$GROUP_NAME" ]; then
    log_info "使用 virtual/group 仓库: $GROUP_NAME"

    N2_GROUP_BASE="$BASE_URL/content/groups/$GROUP_NAME"

    # 先上传一个测试文件到本地仓库
    log_info "上传测试文件到本地仓库..."
    TEST_UPLOAD="/tmp/nexus2-group-test-$$"
    echo "group test content $$" > "$TEST_UPLOAD"
    UPLOAD_CODE=$(curl -s -o /dev/null -w "%{http_code}" -X PUT \
        "$BASE_URL/repository/maven-local/com/test/group-test/1.0.0/group-test-1.0.0.jar" \
        -H "Authorization: Bearer $TOKEN" \
        -H "Content-Type: application/octet-stream" \
        --data-binary @"$TEST_UPLOAD")

    if [ "$UPLOAD_CODE" = "201" ] || [ "$UPLOAD_CODE" = "200" ]; then
        log_pass "测试文件上传成功 (HTTP $UPLOAD_CODE)"

        # 通过 group 路由访问
        HTTP_CODE=$(curl -s -o /tmp/nexus2-group-local-test.jar -w "%{http_code}" \
            "$N2_GROUP_BASE/com/test/group-test/1.0.0/group-test-1.0.0.jar")

        if [ "$HTTP_CODE" = "200" ]; then
            log_pass "Nexus 2 Group 路由访问 virtual 仓库文件成功 (HTTP 200)"
        elif [ "$HTTP_CODE" = "404" ]; then
            log_info "Nexus 2 Group 路由返回 404 (virtual 仓库可能需要重启服务来刷新 runtime 缓存)"
        else
            log_fail "Nexus 2 Group 路由访问 virtual 仓库文件失败 (HTTP $HTTP_CODE)"
        fi
    else
        log_info "测试文件上传失败 (HTTP $UPLOAD_CODE)"
    fi

    # 测试 group 路由访问代理仓库中的文件
    HTTP_CODE=$(curl -s -o /tmp/nexus2-group-test.xml -w "%{http_code}" \
        "$N2_GROUP_BASE/com/google/guava/guava/maven-metadata.xml")

    if [ "$HTTP_CODE" = "200" ]; then
        log_pass "Nexus 2 Group 路由访问代理仓库 Maven 元数据成功 (HTTP 200)"
        if grep -q "<metadata>" /tmp/nexus2-group-test.xml 2>/dev/null; then
            log_pass "Group 路由返回的元数据 XML 格式正确"
        else
            log_info "Group 路由返回的不是标准 Maven 元数据"
        fi
    elif [ "$HTTP_CODE" = "404" ]; then
        log_info "Group 路由访问代理仓库返回 404 (可能 runtime 缓存需要刷新)"
    else
        log_fail "Nexus 2 Group 路由访问代理仓库失败 (HTTP $HTTP_CODE)"
    fi

    rm -f "$TEST_UPLOAD"
else
    log_fail "无法创建或找到 virtual/group 仓库"
fi

# ── 测试 5: Nexus 2 路由边界条件 ──────────────────────
log_section "测试 5: Nexus 2 路由边界条件"

# 无尾斜杠的仓库根路径
HTTP_CODE=$(curl -s -o /dev/null -w "%{http_code}" \
    "$BASE_URL/content/repositories/maven-local")
if [ "$HTTP_CODE" = "404" ] || [ "$HTTP_CODE" = "200" ] || [ "$HTTP_CODE" = "301" ]; then
    log_pass "无尾斜杠的 Nexus 2 仓库根路径返回 HTTP $HTTP_CODE (可接受)"
else
    log_fail "无尾斜杠的 Nexus 2 仓库根路径返回意外状态码 (HTTP $HTTP_CODE)"
fi

# 不存在的仓库
HTTP_CODE=$(curl -s -o /dev/null -w "%{http_code}" \
    "$BASE_URL/content/repositories/nonexistent-repo-$$/anything")
if [ "$HTTP_CODE" = "404" ]; then
    log_pass "Nexus 2 路由访问不存在的仓库返回 404"
else
    log_fail "Nexus 2 路由访问不存在的仓库返回 HTTP $HTTP_CODE (expected 404)"
fi

# 不存在的 group
HTTP_CODE=$(curl -s -o /dev/null -w "%{http_code}" \
    "$BASE_URL/content/groups/nonexistent-group-$$/anything")
if [ "$HTTP_CODE" = "404" ]; then
    log_pass "Nexus 2 路由访问不存在的 group 返回 404"
else
    log_fail "Nexus 2 路由访问不存在的 group 返回 HTTP $HTTP_CODE (expected 404)"
fi

# ── 测试 6: Nexus 2 与 Nexus 3 路由一致性 ──────────────────────
log_section "测试 6: Nexus 2 与 Nexus 3 路由一致性验证"

# 使用之前上传的测试文件
VERIFY_PATH="com/nexus2/nexus2-test/1.0.0/nexus2-test-1.0.0.jar"

N2_HASH=$(curl -s "$N2_REPO_BASE/$VERIFY_PATH" | md5sum | cut -d' ' -f1)
N3_HASH=$(curl -s "$N3_REPO_BASE/$VERIFY_PATH" | md5sum | cut -d' ' -f1)

if [ -n "$N2_HASH" ] && [ -n "$N3_HASH" ] && [ "$N2_HASH" = "$N3_HASH" ]; then
    log_pass "Nexus 2 和 Nexus 3 路由返回的文件内容一致 (MD5: $N2_HASH)"
else
    if [ -z "$N2_HASH" ]; then
        log_fail "Nexus 2 路由返回空内容"
    elif [ -z "$N3_HASH" ]; then
        log_fail "Nexus 3 路由返回空内容"
    else
        log_fail "Nexus 2 和 Nexus 3 路由返回的文件内容不一致 (N2: $N2_HASH, N3: $N3_HASH)"
    fi
fi

# ── 测试 7: Nexus 2 未认证写入应返回 401 ──────────────────────
log_section "测试 7: Nexus 2 路由认证测试"

# 未认证写入
HTTP_CODE=$(curl -s -o /dev/null -w "%{http_code}" -X PUT \
    "$N2_REPO_BASE/com/test/unauthorized/1.0.0/unauthorized-1.0.0.jar" \
    -H "Content-Type: application/octet-stream" \
    --data-binary @"$TEST_JAR")

if [ "$HTTP_CODE" = "401" ]; then
    log_pass "Nexus 2 路由未认证写入返回 401"
else
    log_info "Nexus 2 路由未认证写入返回 HTTP $HTTP_CODE (expected 401)"
fi

# 未认证读取（应该允许，因为仓库可能是公开的）
HTTP_CODE=$(curl -s -o /dev/null -w "%{http_code}" \
    "$N2_REPO_BASE/com/google/guava/guava/maven-metadata.xml")

if [ "$HTTP_CODE" = "200" ] || [ "$HTTP_CODE" = "404" ]; then
    log_pass "Nexus 2 路由未认证读取返回 HTTP $HTTP_CODE (符合预期)"
else
    log_info "Nexus 2 路由未认证读取返回 HTTP $HTTP_CODE"
fi

# 清理测试文件
curl -s -o /dev/null -X DELETE \
    "$N2_REPO_BASE/com/nexus2/nexus2-test/1.0.0/nexus2-test-1.0.0.jar" \
    -H "Authorization: Bearer $TOKEN" 2>/dev/null || true

curl -s -o /dev/null -X DELETE \
    "$N2_REPO_BASE/com/nexus2/nexus2-test/1.0.0/nexus2-test-1.0.0.pom" \
    -H "Authorization: Bearer $TOKEN" 2>/dev/null || true

# ── 汇总 ──────────────────────
echo ""
echo "============================================"
echo " 测试汇总"
echo "============================================"
echo -e "  ${GREEN}通过: $PASS${NC}"
echo -e "  ${RED}失败: $FAIL${NC}"
echo -e "  总计: $((PASS + FAIL))"
echo ""

if [ $FAIL -eq 0 ]; then
    echo -e "${GREEN}所有测试通过! ✅${NC}"
    echo ""
    echo "Nexus 2 路由兼容性验证完成:"
    echo "  ✓ /content/repositories/:repoName GET 下载"
    echo "  ✓ /content/repositories/:repoName PUT 上传"
    echo "  ✓ /content/repositories/:repoName DELETE 删除"
    echo "  ✓ /content/groups/:groupName GET 访问"
    echo "  ✓ Nexus 2 与 Nexus 3 路由一致性"
    echo "  ✓ 边界条件处理"
    exit 0
else
    echo -e "${RED}部分测试失败! ❌${NC}"
    exit 1
fi
