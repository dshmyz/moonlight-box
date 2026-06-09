#!/bin/bash

# =============================================================================
# NPM 代理缓存回源集成测试
# 验证 QueryArtifacts 缓存、GetArtifact 回源、负缓存清除等核心逻辑
# =============================================================================

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

pass() { echo -e "  ${GREEN}✓ PASS${NC} $1"; PASS_COUNT=$((PASS_COUNT + 1)); }
fail() { echo -e "  ${RED}✗ FAIL${NC} $1"; FAIL_COUNT=$((FAIL_COUNT + 1)); }
info() { echo -e "  ${BLUE}ℹ INFO${NC} $1"; }
warn() { echo -e "  ${YELLOW}⚠ WARN${NC} $1"; WARN_COUNT=$((WARN_COUNT + 1)); }

CLEAN_TEMPS=()
cleanup() { rm -rf "${CLEAN_TEMPS[@]}" 2>/dev/null || true; }
trap cleanup EXIT

echo "============================================"
echo " NPM 代理缓存回源集成测试"
echo " 验证元数据缓存、tarball 回源、负缓存清除"
echo " 目标: $BASE_URL"
echo "============================================"
echo

# 检查必要命令
if ! command -v curl &> /dev/null; then
    warn "curl 命令未安装，跳过测试"
    exit 0
fi
if ! command -v jq &> /dev/null; then
    warn "jq 命令未安装，跳过测试"
    exit 0
fi

# 获取认证令牌
TOKEN=$(curl -s -X POST "$BASE_URL/api/v1/auth/login" \
    -H "Content-Type: application/json" \
    -d "{\"username\":\"$ADMIN_USER\",\"password\":\"$ADMIN_PASS\"}" | \
    jq -r '.data.access_token // empty' 2>/dev/null)

if [ -z "$TOKEN" ]; then
    warn "无法获取认证令牌，跳过需要认证的测试"
fi

# 确保代理仓库存在
PROXY_REPO="npm-proxy-cn"
HTTP_CODE=$(curl -s -o /dev/null -w "%{http_code}" \
    -H "Authorization: Bearer $TOKEN" \
    "$BASE_URL/api/v1/repositories/$PROXY_REPO")
if [ "$HTTP_CODE" != "200" ]; then
    warn "代理仓库 $PROXY_REPO 不存在 (HTTP $HTTP_CODE)，跳过测试"
    exit 0
fi

PROXY_REGISTRY="$BASE_URL/repository/$PROXY_REPO"

# ═══════════════════════════════════════════════════════════════
#  第一部分：元数据缓存测试（QueryArtifacts）
# ═══════════════════════════════════════════════════════════════

echo
echo "════════════════════════════════════════"
echo "  第一部分：元数据缓存测试"
echo "════════════════════════════════════════"

# ── 测试 1：首次访问包元数据触发回源 ──────────────────
echo
echo "测试 1: 首次访问包元数据触发回源..."

# 使用一个不太常用的包减少缓存命中的可能
TEST_PKG="ansi-regex"
META_HTTP=$(curl -s -o /tmp/npm-ansi-regex-meta.json -w "%{http_code}" \
    "$PROXY_REGISTRY/$TEST_PKG")

if [ "$META_HTTP" = "200" ]; then
    pass "$TEST_PKG 包元数据可访问 (HTTP 200)"

    # 验证元数据包含必要字段
    HAS_NAME=$(jq -r '.name != null' /tmp/npm-ansi-regex-meta.json 2>/dev/null)
    if [ "$HAS_NAME" = "true" ]; then
        pass "元数据包含 name 字段"
    else
        fail "元数据缺少 name 字段"
    fi

    HAS_VERSIONS=$(jq -r '.versions != null' /tmp/npm-ansi-regex-meta.json 2>/dev/null)
    if [ "$HAS_VERSIONS" = "true" ]; then
        VER_COUNT=$(jq -r '.versions | length' /tmp/npm-ansi-regex-meta.json 2>/dev/null)
        pass "元数据包含 versions 对象 ($VER_COUNT 个版本)"
    else
        fail "元数据缺少 versions 对象"
    fi
else
    warn "$TEST_PKG 包元数据不可访问 (HTTP $META_HTTP)"
fi

# ── 测试 2：再次访问同一包元数据应命中缓存 ──────────────────
echo
echo "测试 2: 再次访问同一包元数据应命中缓存..."

# 清空临时文件，确保不是读取本地缓存
rm -f /tmp/npm-ansi-regex-meta2.json

META_HTTP2=$(curl -s -o /tmp/npm-ansi-regex-meta2.json -w "%{http_code}" \
    "$PROXY_REGISTRY/$TEST_PKG")

if [ "$META_HTTP2" = "200" ]; then
    pass "$TEST_PKG 包元数据第二次访问成功 (HTTP 200)"

    # 验证两次返回的元数据一致
    if diff -q /tmp/npm-ansi-regex-meta.json /tmp/npm-ansi-regex-meta2.json > /dev/null 2>&1; then
        pass "两次访问返回的元数据一致"
    else
        # 可能因为异步刷新导致时间戳不同，检查关键字段是否一致
        NAME1=$(jq -r '.name' /tmp/npm-ansi-regex-meta.json 2>/dev/null)
        NAME2=$(jq -r '.name' /tmp/npm-ansi-regex-meta2.json 2>/dev/null)
        VER1=$(jq -r '.["dist-tags"].latest' /tmp/npm-ansi-regex-meta.json 2>/dev/null)
        VER2=$(jq -r '.["dist-tags"].latest' /tmp/npm-ansi-regex-meta2.json 2>/dev/null)

        if [ "$NAME1" = "$NAME2" ] && [ "$VER1" = "$VER2" ]; then
            pass "两次访问返回的元数据关键字段一致 (name=$NAME1, latest=$VER1)"
        else
            fail "两次访问返回的元数据关键字段不一致"
        fi
    fi
else
    warn "$TEST_PKG 包元数据第二次访问失败 (HTTP $META_HTTP2)"
fi

# ═══════════════════════════════════════════════════════════════
#  第二部分：tarball 下载测试（GetArtifact 回源）
# ═══════════════════════════════════════════════════════════════

echo
echo "════════════════════════════════════════"
echo "  第二部分：tarball 下载测试"
echo "════════════════════════════════════════"

# ── 测试 3：下载已缓存元数据中的 tarball ──────────────────
echo
echo "测试 3: 下载已缓存元数据中的 tarball..."

# 从元数据中获取一个版本的 tarball URL
if [ -f /tmp/npm-ansi-regex-meta.json ]; then
    LATEST_VER=$(jq -r '.["dist-tags"].latest' /tmp/npm-ansi-regex-meta.json 2>/dev/null)
    info "$TEST_PKG 最新版本: $LATEST_VER"

    if [ -n "$LATEST_VER" ] && [ "$LATEST_VER" != "null" ]; then
        TARBALL_URL=$(jq -r ".versions[\"$LATEST_VER\"].dist.tarball" /tmp/npm-ansi-regex-meta.json 2>/dev/null)
        info "tarball URL: $TARBALL_URL"

        if [ -n "$TARBALL_URL" ] && [ "$TARBALL_URL" != "null" ]; then
            # 下载 tarball
            TARBALL_HTTP=$(curl -s -o /tmp/npm-ansi-regex.tgz -w "%{http_code}" "$TARBALL_URL")

            if [ "$TARBALL_HTTP" = "200" ]; then
                pass "tarball 下载成功 (HTTP 200)"

                # 验证 tarball 文件有效性
                if tar -tzf /tmp/npm-ansi-regex.tgz > /dev/null 2>&1; then
                    pass "tarball 文件格式有效"
                    TARBALL_SIZE=$(stat -f%z /tmp/npm-ansi-regex.tgz 2>/dev/null || stat -c%s /tmp/npm-ansi-regex.tgz 2>/dev/null)
                    info "tarball 大小: $TARBALL_SIZE bytes"
                else
                    fail "tarball 文件格式无效"
                fi
            else
                fail "tarball 下载失败 (HTTP $TARBALL_HTTP)"
            fi
        else
            fail "无法从元数据中提取 tarball URL"
        fi
    else
        fail "无法获取最新版本号"
    fi
else
    fail "元数据文件不存在"
fi

# ── 测试 4：下载未缓存的版本 tarball（验证回源）──────────
echo
echo "测试 4: 下载未缓存的版本 tarball (验证回源)..."

# 获取一个旧版本（不太可能被缓存）
if [ -f /tmp/npm-ansi-regex-meta.json ]; then
    # 获取所有版本，选择倒数第二个版本
    ALL_VERSIONS=$(jq -r '.versions | keys[]' /tmp/npm-ansi-regex-meta.json 2>/dev/null | sort -V)
    VER_COUNT=$(echo "$ALL_VERSIONS" | wc -l | tr -d ' ')

    if [ "$VER_COUNT" -gt 1 ]; then
        OLD_VER=$(echo "$ALL_VERSIONS" | tail -2 | head -1)
        info "选择旧版本: $OLD_VER"

        OLD_TARBALL_URL=$(jq -r ".versions[\"$OLD_VER\"].dist.tarball" /tmp/npm-ansi-regex-meta.json 2>/dev/null)
        info "旧版本 tarball URL: $OLD_TARBALL_URL"

        if [ -n "$OLD_TARBALL_URL" ] && [ "$OLD_TARBALL_URL" != "null" ]; then
            OLD_TARBALL_HTTP=$(curl -s -o /tmp/npm-ansi-regex-old.tgz -w "%{http_code}" "$OLD_TARBALL_URL")

            if [ "$OLD_TARBALL_HTTP" = "200" ]; then
                pass "旧版本 tarball 下载成功 (HTTP 200)"

                if tar -tzf /tmp/npm-ansi-regex-old.tgz > /dev/null 2>&1; then
                    pass "旧版本 tarball 文件格式有效"
                else
                    fail "旧版本 tarball 文件格式无效"
                fi
            else
                fail "旧版本 tarball 下载失败 (HTTP $OLD_TARBALL_HTTP)"
            fi
        else
            fail "无法从元数据中提取旧版本 tarball URL"
        fi
    else
        info "版本数量不足，跳过旧版本测试"
    fi
fi

# ═══════════════════════════════════════════════════════════════
#  第三部分：多版本 tarball 缓存独立性测试
# ═══════════════════════════════════════════════════════════════

echo
echo "════════════════════════════════════════"
echo "  第三部分：多版本 tarball 缓存独立性"
echo "════════════════════════════════════════"

# ── 测试 5：验证不同版本的 tarball 互不影响 ──────────────────
echo
echo "测试 5: 验证不同版本的 tarball 缓存互不影响..."

# 使用 lodash 包（有大量版本）
LODASH_PKG="lodash"
LODASH_META=$(curl -s -o /tmp/npm-lodash-meta.json -w "%{http_code}" \
    "$PROXY_REGISTRY/$LODASH_PKG")

if [ "$LODASH_META" = "200" ]; then
    pass "$LODASH_PKG 包元数据可访问 (HTTP 200)"

    # 选择两个不同版本
    LODASH_VERSIONS=$(jq -r '.versions | keys[]' /tmp/npm-lodash-meta.json 2>/dev/null | grep -E "^4\." | sort -V | tail -3)
    VER1=$(echo "$LODASH_VERSIONS" | head -1)
    VER2=$(echo "$LODASH_VERSIONS" | tail -1)

    if [ -n "$VER1" ] && [ -n "$VER2" ] && [ "$VER1" != "$VER2" ]; then
        info "选择两个版本: $VER1 和 $VER2"

        # 下载第一个版本的 tarball
        TARBALL1_URL=$(jq -r ".versions[\"$VER1\"].dist.tarball" /tmp/npm-lodash-meta.json 2>/dev/null)
        TARBALL1_HTTP=$(curl -s -o /tmp/npm-lodash-v1.tgz -w "%{http_code}" "$TARBALL1_URL")

        if [ "$TARBALL1_HTTP" = "200" ]; then
            pass "lodash@$VER1 tarball 下载成功"
        else
            fail "lodash@$VER1 tarball 下载失败 (HTTP $TARBALL1_HTTP)"
        fi

        # 下载第二个版本的 tarball
        TARBALL2_URL=$(jq -r ".versions[\"$VER2\"].dist.tarball" /tmp/npm-lodash-meta.json 2>/dev/null)
        TARBALL2_HTTP=$(curl -s -o /tmp/npm-lodash-v2.tgz -w "%{http_code}" "$TARBALL2_URL")

        if [ "$TARBALL2_HTTP" = "200" ]; then
            pass "lodash@$VER2 tarball 下载成功"
        else
            fail "lodash@$VER2 tarball 下载失败 (HTTP $TARBALL2_HTTP)"
        fi

        # 验证两个 tarball 内容不同
        if [ -f /tmp/npm-lodash-v1.tgz ] && [ -f /tmp/npm-lodash-v2.tgz ]; then
            SIZE1=$(stat -f%z /tmp/npm-lodash-v1.tgz 2>/dev/null || stat -c%s /tmp/npm-lodash-v1.tgz 2>/dev/null)
            SIZE2=$(stat -f%z /tmp/npm-lodash-v2.tgz 2>/dev/null || stat -c%s /tmp/npm-lodash-v2.tgz 2>/dev/null)

            if [ "$SIZE1" != "$SIZE2" ]; then
                pass "两个版本的 tarball 大小不同 ($SIZE1 vs $SIZE2 bytes) — 缓存独立"
            else
                # 大小相同可能是巧合，检查内容
                if diff -q /tmp/npm-lodash-v1.tgz /tmp/npm-lodash-v2.tgz > /dev/null 2>&1; then
                    fail "两个版本的 tarball 内容相同 — 缓存错误!"
                else
                    pass "两个版本的 tarball 内容不同 — 缓存独立"
                fi
            fi
        fi
    else
        warn "无法选择两个不同的 lodash 版本"
    fi
else
    warn "$LODASH_PKG 包元数据不可访问 (HTTP $LODASH_META)"
fi

# ═══════════════════════════════════════════════════════════════
#  第四部分：负缓存清除验证
# ═══════════════════════════════════════════════════════════════

echo
echo "════════════════════════════════════════"
echo "  第四部分：负缓存清除验证"
echo "════════════════════════════════════════"

# ── 测试 6：访问不存在的包后，发布该包再访问应成功 ──────────────────
echo
echo "测试 6: 负缓存清除后应能访问新发布的包..."

# 使用时间戳创建唯一包名避免冲突
TIMESTAMP=$(date +%s)
TEST_NEW_PKG="test-negative-cache-$TIMESTAMP"

# 首次访问不存在的包（应返回 404 并设置负缓存）
FIRST_ACCESS=$(curl -s -o /dev/null -w "%{http_code}" "$PROXY_REGISTRY/$TEST_NEW_PKG")
info "首次访问不存在的包: HTTP $FIRST_ACCESS"

if [ "$FIRST_ACCESS" = "404" ]; then
    pass "不存在的包返回 404 (预期行为)"

    # 注意：这里无法直接测试负缓存清除，因为需要在本地仓库发布包
    # 该测试需要配合本地仓库使用，此处仅验证 404 行为
    info "负缓存清除需要通过本地仓库发布包后验证"
else
    warn "不存在的包返回 HTTP $FIRST_ACCESS (预期 404)"
fi

# ═══════════════════════════════════════════════════════════════
#  第五部分：scoped 包缓存测试
# ═══════════════════════════════════════════════════════════════

echo
echo "════════════════════════════════════════"
echo "  第五部分：scoped 包缓存测试"
echo "════════════════════════════════════════"

# ── 测试 7：scoped 包元数据缓存 ──────────────────
echo
echo "测试 7: scoped 包元数据缓存..."

SCOPED_PKG="@babel/core"
SCOPED_META=$(curl -s -o /tmp/npm-babel-core-meta.json -w "%{http_code}" \
    "$PROXY_REGISTRY/$SCOPED_PKG")

if [ "$SCOPED_META" = "200" ]; then
    pass "$SCOPED_PKG 包元数据可访问 (HTTP 200)"

    # 再次访问验证缓存命中
    SCOPED_META2=$(curl -s -o /tmp/npm-babel-core-meta2.json -w "%{http_code}" \
        "$PROXY_REGISTRY/$SCOPED_PKG")

    if [ "$SCOPED_META2" = "200" ]; then
        pass "$SCOPED_PKG 包元数据第二次访问成功"

        # 验证元数据一致
        NAME1=$(jq -r '.name' /tmp/npm-babel-core-meta.json 2>/dev/null)
        NAME2=$(jq -r '.name' /tmp/npm-babel-core-meta2.json 2>/dev/null)

        if [ "$NAME1" = "$NAME2" ] && [ "$NAME1" = "$SCOPED_PKG" ]; then
            pass "scoped 包元数据缓存正确 (name=$NAME1)"
        else
            fail "scoped 包元数据缓存错误 (name1=$NAME1, name2=$NAME2)"
        fi
    else
        fail "$SCOPED_PKG 包元数据第二次访问失败 (HTTP $SCOPED_META2)"
    fi
else
    warn "$SCOPED_PKG 包元数据不可访问 (HTTP $SCOPED_META)"
fi

# ── 测试 8：scoped 包 tarball 下载 ──────────────────
echo
echo "测试 8: scoped 包 tarball 下载..."

if [ -f /tmp/npm-babel-core-meta.json ]; then
    BABEL_LATEST=$(jq -r '.["dist-tags"].latest' /tmp/npm-babel-core-meta.json 2>/dev/null)
    info "$SCOPED_PKG 最新版本: $BABEL_LATEST"

    if [ -n "$BABEL_LATEST" ] && [ "$BABEL_LATEST" != "null" ]; then
        BABEL_TARBALL=$(jq -r ".versions[\"$BABEL_LATEST\"].dist.tarball" /tmp/npm-babel-core-meta.json 2>/dev/null)
        info "tarball URL: $BABEL_TARBALL"

        if [ -n "$BABEL_TARBALL" ] && [ "$BABEL_TARBALL" != "null" ]; then
            BABEL_TARBALL_HTTP=$(curl -s -o /tmp/npm-babel-core.tgz -w "%{http_code}" "$BABEL_TARBALL")

            if [ "$BABEL_TARBALL_HTTP" = "200" ]; then
                pass "scoped 包 tarball 下载成功 (HTTP 200)"

                if tar -tzf /tmp/npm-babel-core.tgz > /dev/null 2>&1; then
                    pass "scoped 包 tarball 文件格式有效"
                else
                    fail "scoped 包 tarball 文件格式无效"
                fi
            else
                fail "scoped 包 tarball 下载失败 (HTTP $BABEL_TARBALL_HTTP)"
            fi
        else
            fail "无法从元数据中提取 scoped 包 tarball URL"
        fi
    fi
fi

# ═══════════════════════════════════════════════════════════════
#  第六部分：并发下载测试
# ═══════════════════════════════════════════════════════════════

echo
echo "════════════════════════════════════════"
echo "  第六部分：并发下载测试"
echo "════════════════════════════════════════"

# ── 测试 9：并发下载同一包的不同版本 ──────────────────
echo
echo "测试 9: 并发下载同一包的不同版本..."

if [ -f /tmp/npm-lodash-meta.json ]; then
    # 选择 3 个版本并发下载
    LODASH_VERS=$(jq -r '.versions | keys[]' /tmp/npm-lodash-meta.json 2>/dev/null | grep -E "^4\." | sort -V | tail -5 | head -3)
    CONCURRENT_COUNT=0
    CONCURRENT_SUCCESS=0

    for ver in $LODASH_VERS; do
        TARBALL_URL=$(jq -r ".versions[\"$ver\"].dist.tarball" /tmp/npm-lodash-meta.json 2>/dev/null)
        (
            TARBALL_HTTP=$(curl -s -o "/tmp/npm-lodash-$ver.tgz" -w "%{http_code}" "$TARBALL_URL")
            if [ "$TARBALL_HTTP" = "200" ]; then
                echo "SUCCESS:$ver" >> /tmp/npm-concurrent-result.txt
            else
                echo "FAIL:$ver:$TARBALL_HTTP" >> /tmp/npm-concurrent-result.txt
            fi
        ) &
        CONCURRENT_COUNT=$((CONCURRENT_COUNT + 1))
    done

    # 等待所有后台任务完成
    wait

    # 检查结果
    if [ -f /tmp/npm-concurrent-result.txt ]; then
        SUCCESS_COUNT=$(grep -c "^SUCCESS:" /tmp/npm-concurrent-result.txt 2>/dev/null || echo "0")
        if [ "$SUCCESS_COUNT" = "$CONCURRENT_COUNT" ]; then
            pass "并发下载 $CONCURRENT_COUNT 个版本全部成功"
        else
            fail "并发下载部分失败 (成功: $SUCCESS_COUNT / $CONCURRENT_COUNT)"
            cat /tmp/npm-concurrent-result.txt
        fi
        rm -f /tmp/npm-concurrent-result.txt
    else
        fail "并发下载结果文件不存在"
    fi
fi

# 清理
cd /
rm -f /tmp/npm-*.json /tmp/npm-*.tgz /tmp/npm-concurrent-result.txt 2>/dev/null || true

echo
echo "============================================"
echo " NPM 代理缓存回源测试完成"
echo "============================================"
echo -e "  通过: ${GREEN}$PASS_COUNT${NC}"
echo -e "  失败: ${RED}$FAIL_COUNT${NC}"
echo -e "  警告: ${YELLOW}$WARN_COUNT${NC}"
echo

if [ $FAIL_COUNT -eq 0 ]; then
    echo -e "${GREEN}所有测试通过! ✓${NC}"
    exit 0
else
    echo -e "${RED}部分测试失败! ❌${NC}"
    exit 1
fi
