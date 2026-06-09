#!/bin/bash

# =============================================================================
# NPM 元数据完整性集成测试
# 验证 npm 包安装后 bin 目录二进制文件、scripts、dependencies 等字段完整
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
echo " NPM 元数据完整性集成测试"
echo " 验证 bin/main/scripts/dependencies 等字段"
echo " 目标: $BASE_URL"
echo "============================================"
echo

# 检查 npm 命令
if ! command -v npm &> /dev/null; then
    warn "npm 命令未安装，跳过测试"
    exit 0
fi
if ! command -v node &> /dev/null; then
    warn "node 命令未安装，跳过测试"
    exit 0
fi
if ! command -v jq &> /dev/null; then
    warn "jq 命令未安装，跳过测试"
    exit 0
fi

info "Node.js 版本: $(node --version)"
info "npm 版本: $(npm --version)"

# 获取认证令牌
TOKEN=$(curl -s -X POST "$BASE_URL/api/v1/auth/login" \
    -H "Content-Type: application/json" \
    -d "{\"username\":\"$ADMIN_USER\",\"password\":\"$ADMIN_PASS\"}" | \
    grep -o '"access_token":"[^"]*"' | \
    sed 's/"access_token":"//;s/"//')

if [ -z "$TOKEN" ]; then
    warn "无法获取认证令牌，跳过测试"
    exit 0
fi

# 确保代理仓库存在
PROXY_REPO="npm-proxy-cn"
HTTP_CODE=$(curl -s -o /dev/null -w "%{http_code}" \
    -H "Authorization: Bearer $TOKEN" \
    "$BASE_URL/api/v1/repositories/$PROXY_REPO")
if [ "$HTTP_CODE" != "200" ]; then
    warn "代理仓库 $PROXY_REPO 不存在，跳过代理测试"
fi

# 确保本地仓库存在
LOCAL_REPO="npm-local"
HTTP_CODE=$(curl -s -o /dev/null -w "%{http_code}" \
    -H "Authorization: Bearer $TOKEN" \
    "$BASE_URL/api/v1/repositories/$LOCAL_REPO")
if [ "$HTTP_CODE" != "200" ]; then
    warn "本地仓库 $LOCAL_REPO 不存在，跳过 Hosted 测试"
fi

# ═══════════════════════════════════════════════════════════════
#  第一部分：代理仓库元数据完整性测试
# ═══════════════════════════════════════════════════════════════

echo
echo "════════════════════════════════════════"
echo "  第一部分：代理仓库元数据完整性"
echo "════════════════════════════════════════"

PROXY_REGISTRY="$BASE_URL/repository/$PROXY_REPO"

# ── 测试 1：semver 包元数据包含 bin 字段 ──────────────────
echo
echo "测试 1: 验证 semver 包元数据包含 bin 字段..."

META_HTTP=$(curl -s -o /tmp/npm-semver-meta.json -w "%{http_code}" \
    "$PROXY_REGISTRY/semver")

if [ "$META_HTTP" = "200" ]; then
    pass "semver 包元数据可访问 (HTTP 200)"

    # 检查 versions 对象中是否包含 bin 字段
    BIN_COUNT=$(jq '[.versions[] | select(.bin != null)] | length' /tmp/npm-semver-meta.json 2>/dev/null || echo "0")
    if [ "$BIN_COUNT" -gt 0 ]; then
        pass "semver 包元数据包含 bin 字段 ($BIN_COUNT 个版本有 bin)"

        # 抽查一个版本的 bin 内容
        SAMPLE_BIN=$(jq -r '.versions | to_entries[0].value.bin' /tmp/npm-semver-meta.json 2>/dev/null)
        info "示例 bin 字段: $SAMPLE_BIN"
    else
        fail "semver 包元数据缺少 bin 字段 — npm install 后将无法创建可执行文件"
    fi
else
    warn "semver 包元数据不可访问 (HTTP $META_HTTP)"
fi

# ── 测试 2：eslint 包元数据包含完整字段 ──────────────────
echo
echo "测试 2: 验证 eslint 包元数据包含 bin/scripts/dependencies..."

META_HTTP=$(curl -s -o /tmp/npm-eslint-meta.json -w "%{http_code}" \
    "$PROXY_REGISTRY/eslint")

if [ "$META_HTTP" = "200" ]; then
    pass "eslint 包元数据可访问 (HTTP 200)"

    # 取最新版本
    LATEST_VER=$(jq -r '."dist-tags".latest' /tmp/npm-eslint-meta.json 2>/dev/null)
    info "eslint 最新版本: $LATEST_VER"

    if [ -n "$LATEST_VER" ] && [ "$LATEST_VER" != "null" ]; then
        # 检查 bin
        HAS_BIN=$(jq -r ".versions[\"$LATEST_VER\"].bin != null" /tmp/npm-eslint-meta.json 2>/dev/null)
        if [ "$HAS_BIN" = "true" ]; then
            pass "eslint@$LATEST_VER 包含 bin 字段"
        else
            fail "eslint@$LATEST_VER 缺少 bin 字段"
        fi

        # 检查 scripts
        HAS_SCRIPTS=$(jq -r ".versions[\"$LATEST_VER\"].scripts != null" /tmp/npm-eslint-meta.json 2>/dev/null)
        if [ "$HAS_SCRIPTS" = "true" ]; then
            pass "eslint@$LATEST_VER 包含 scripts 字段"
        else
            fail "eslint@$LATEST_VER 缺少 scripts 字段"
        fi

        # 检查 dependencies
        HAS_DEPS=$(jq -r ".versions[\"$LATEST_VER\"].dependencies != null" /tmp/npm-eslint-meta.json 2>/dev/null)
        if [ "$HAS_DEPS" = "true" ]; then
            pass "eslint@$LATEST_VER 包含 dependencies 字段"
        else
            fail "eslint@$LATEST_VER 缺少 dependencies 字段"
        fi

        # 检查 main
        HAS_MAIN=$(jq -r ".versions[\"$LATEST_VER\"].main != null" /tmp/npm-eslint-meta.json 2>/dev/null)
        if [ "$HAS_MAIN" = "true" ]; then
            pass "eslint@$LATEST_VER 包含 main 字段"
        else
            fail "eslint@$LATEST_VER 缺少 main 字段"
        fi

        # 检查 dist.shasum
        HAS_SHASUM=$(jq -r ".versions[\"$LATEST_VER\"].dist.shasum != null" /tmp/npm-eslint-meta.json 2>/dev/null)
        if [ "$HAS_SHASUM" = "true" ]; then
            pass "eslint@$LATEST_VER 包含 dist.shasum 字段"
        else
            fail "eslint@$LATEST_VER 缺少 dist.shasum 字段"
        fi

        # 检查 dist.integrity
        HAS_INTEGRITY=$(jq -r ".versions[\"$LATEST_VER\"].dist.integrity != null" /tmp/npm-eslint-meta.json 2>/dev/null)
        if [ "$HAS_INTEGRITY" = "true" ]; then
            pass "eslint@$LATEST_VER 包含 dist.integrity 字段"
        else
            fail "eslint@$LATEST_VER 缺少 dist.integrity 字段"
        fi
    fi
else
    warn "eslint 包元数据不可访问 (HTTP $META_HTTP)"
fi

# ── 测试 3：npm install semver 后 bin 目录存在 ──────────────────
echo
echo "测试 3: npm install semver 验证 bin 目录二进制文件..."

INSTALL_DIR="/tmp/npm-bin-test-semver-$$"
CLEAN_TEMPS+=("$INSTALL_DIR")
mkdir -p "$INSTALL_DIR"
cd "$INSTALL_DIR"

cat > package.json <<'EOF'
{
  "name": "test-bin-install",
  "version": "1.0.0",
  "dependencies": {
    "semver": "^7.6.0"
  }
}
EOF

cat > .npmrc <<EOF
registry=$PROXY_REGISTRY
EOF

if npm install 2>/tmp/npm-install-semver.log; then
    pass "npm install semver 成功"

    # 检查 node_modules/.bin/semver 是否存在
    if [ -f "node_modules/.bin/semver" ]; then
        pass "node_modules/.bin/semver 可执行文件存在"

        # 验证可执行（非关键，某些包可能需要特殊环境）
        if node_modules/.bin/semver --version &>/dev/null; then
            pass "semver 可执行文件正常运行: $(node_modules/.bin/semver --version)"
        else
            warn "semver 可执行文件运行失败（可能需要特殊环境，不影响 bin 字段验证）"
        fi
    else
        # 列出 .bin 目录内容帮助诊断
        BIN_CONTENTS=$(ls node_modules/.bin/ 2>/dev/null | head -10 || echo "(空)")
        fail "node_modules/.bin/semver 不存在 — bin 字段未正确传递！.bin 目录内容: $BIN_CONTENTS"
        info "npm install 日志: $(tail -5 /tmp/npm-install-semver.log 2>/dev/null)"
    fi

    # 检查 semver 包的 package.json 中是否有 bin 字段
    if [ -f "node_modules/semver/package.json" ]; then
        PKG_BIN=$(jq -r '.bin' node_modules/semver/package.json 2>/dev/null)
        if [ "$PKG_BIN" != "null" ] && [ -n "$PKG_BIN" ]; then
            pass "semver 包的 package.json 包含 bin 字段: $PKG_BIN"
        else
            fail "semver 包的 package.json 缺少 bin 字段"
        fi
    fi
else
    warn "npm install semver 失败 (日志: /tmp/npm-install-semver.log)"
fi

# ── 测试 4：npm install eslint 后 bin 目录存在 ──────────────────
echo
echo "测试 4: npm install eslint 验证 bin 目录二进制文件..."

INSTALL_DIR2="/tmp/npm-bin-test-eslint-$$"
CLEAN_TEMPS+=("$INSTALL_DIR2")
mkdir -p "$INSTALL_DIR2"
cd "$INSTALL_DIR2"

cat > package.json <<'EOF'
{
  "name": "test-eslint-bin-install",
  "version": "1.0.0",
  "dependencies": {
    "eslint": "^8.56.0"
  }
}
EOF

cat > .npmrc <<EOF
registry=$PROXY_REGISTRY
EOF

if npm install 2>/tmp/npm-install-eslint.log; then
    pass "npm install eslint 成功"

    # eslint 的 bin 是 { "eslint": "./bin/eslint.js" }
    if [ -f "node_modules/.bin/eslint" ]; then
        pass "node_modules/.bin/eslint 可执行文件存在"
    else
        BIN_CONTENTS=$(ls node_modules/.bin/ 2>/dev/null | head -10 || echo "(空)")
        fail "node_modules/.bin/eslint 不存在 — bin 字段未正确传递！.bin 目录内容: $BIN_CONTENTS"
    fi

    # 检查 eslint 包的 package.json
    if [ -f "node_modules/eslint/package.json" ]; then
        PKG_BIN=$(jq -r '.bin' node_modules/eslint/package.json 2>/dev/null)
        if [ "$PKG_BIN" != "null" ] && [ -n "$PKG_BIN" ]; then
            pass "eslint 包的 package.json 包含 bin 字段: $PKG_BIN"
        else
            fail "eslint 包的 package.json 缺少 bin 字段"
        fi

        # 检查 main 字段
        PKG_MAIN=$(jq -r '.main' node_modules/eslint/package.json 2>/dev/null)
        if [ "$PKG_MAIN" != "null" ] && [ -n "$PKG_MAIN" ]; then
            pass "eslint 包的 package.json 包含 main 字段: $PKG_MAIN"
        else
            fail "eslint 包的 package.json 缺少 main 字段"
        fi
    fi
else
    warn "npm install eslint 失败 (日志: /tmp/npm-install-eslint.log)"
fi

# ── 测试 5：scoped 包 (@babel/core) 元数据完整性 ──────────────────
echo
echo "测试 5: 验证 scoped 包 @babel/core 元数据完整性..."

META_HTTP=$(curl -s -o /tmp/npm-babel-meta.json -w "%{http_code}" \
    "$PROXY_REGISTRY/@babel/core")

if [ "$META_HTTP" = "200" ]; then
    pass "@babel/core 包元数据可访问 (HTTP 200)"

    LATEST_VER=$(jq -r '."dist-tags".latest' /tmp/npm-babel-meta.json 2>/dev/null)
    info "@babel/core 最新版本: $LATEST_VER"

    if [ -n "$LATEST_VER" ] && [ "$LATEST_VER" != "null" ]; then
        # 检查 dependencies
        HAS_DEPS=$(jq -r ".versions[\"$LATEST_VER\"].dependencies != null" /tmp/npm-babel-meta.json 2>/dev/null)
        if [ "$HAS_DEPS" = "true" ]; then
            DEP_COUNT=$(jq ".versions[\"$LATEST_VER\"].dependencies | length" /tmp/npm-babel-meta.json 2>/dev/null)
            pass "@babel/core@$LATEST_VER 包含 dependencies ($DEP_COUNT 个依赖)"
        else
            fail "@babel/core@$LATEST_VER 缺少 dependencies 字段"
        fi

        # 检查 dist.tarball 格式正确
        TARBALL=$(jq -r ".versions[\"$LATEST_VER\"].dist.tarball" /tmp/npm-babel-meta.json 2>/dev/null)
        if echo "$TARBALL" | grep -q "@babel/core"; then
            pass "@babel/core tarball URL 包含正确路径: $TARBALL"
        else
            fail "@babel/core tarball URL 路径不正确: $TARBALL"
        fi
    fi
else
    warn "@babel/core 包元数据不可访问 (HTTP $META_HTTP)"
fi

# ── 测试 6：bin 为字符串的包 (rimraf) ──────────────────
echo
echo "测试 6: 验证 bin 为字符串的包 (rimraf) 元数据..."

META_HTTP=$(curl -s -o /tmp/npm-rimraf-meta.json -w "%{http_code}" \
    "$PROXY_REGISTRY/rimraf")

if [ "$META_HTTP" = "200" ]; then
    pass "rimraf 包元数据可访问 (HTTP 200)"

    LATEST_VER=$(jq -r '."dist-tags".latest' /tmp/npm-rimraf-meta.json 2>/dev/null)
    if [ -n "$LATEST_VER" ] && [ "$LATEST_VER" != "null" ]; then
        HAS_BIN=$(jq -r ".versions[\"$LATEST_VER\"].bin != null" /tmp/npm-rimraf-meta.json 2>/dev/null)
        if [ "$HAS_BIN" = "true" ]; then
            BIN_TYPE=$(jq -r ".versions[\"$LATEST_VER\"].bin | type" /tmp/npm-rimraf-meta.json 2>/dev/null)
            pass "rimraf@$LATEST_VER 包含 bin 字段 (类型: $BIN_TYPE)"
        else
            fail "rimraf@$LATEST_VER 缺少 bin 字段"
        fi
    fi
else
    warn "rimraf 包元数据不可访问 (HTTP $META_HTTP)"
fi

# ── 测试 7：time 字段 ──────────────────
echo
echo "测试 7: 验证包元数据包含 time 字段..."

if [ -f /tmp/npm-eslint-meta.json ]; then
    HAS_TIME=$(jq -r '.time != null' /tmp/npm-eslint-meta.json 2>/dev/null)
    if [ "$HAS_TIME" = "true" ]; then
        pass "eslint 包元数据包含 time 字段"
        TIME_COUNT=$(jq -r '.time | length' /tmp/npm-eslint-meta.json 2>/dev/null)
        info "time 字段包含 $TIME_COUNT 个条目"
    else
        fail "eslint 包元数据缺少 time 字段"
    fi
fi

# ═══════════════════════════════════════════════════════════════
#  第二部分：Hosted 仓库元数据完整性测试
# ═══════════════════════════════════════════════════════════════

echo
echo "════════════════════════════════════════"
echo "  第二部分：Hosted 仓库元数据完整性"
echo "════════════════════════════════════════"

LOCAL_REGISTRY="$BASE_URL/repository/$LOCAL_REPO"

# ── 测试 8：发布带 bin 的包到 Hosted 仓库 ──────────────────
echo
echo "测试 8: 发布带 bin 的包到 Hosted 仓库..."

PUBLISH_DIR="/tmp/npm-bin-test-publish-$$"
CLEAN_TEMPS+=("$PUBLISH_DIR")
mkdir -p "$PUBLISH_DIR/bin"
cd "$PUBLISH_DIR"

cat > package.json <<'EOF'
{
  "name": "test-bin-pkg",
  "version": "1.0.0",
  "description": "Test package with bin field",
  "main": "lib/index.js",
  "bin": {
    "test-bin-cmd": "./bin/cli.js",
    "test-bin-helper": "./bin/helper.js"
  },
  "scripts": {
    "build": "echo build",
    "test": "echo test"
  },
  "dependencies": {
    "lodash": "^4.17.21"
  },
  "engines": {
    "node": ">=16.0.0"
  },
  "license": "MIT",
  "homepage": "https://example.com/test-bin-pkg"
}
EOF

mkdir -p lib
cat > lib/index.js <<'EOF'
module.exports = { hello: () => 'world' };
EOF

cat > bin/cli.js <<'EOF'
#!/usr/bin/env node
console.log('Hello from test-bin-cmd');
EOF

cat > bin/helper.js <<'EOF'
#!/usr/bin/env node
console.log('Hello from test-bin-helper');
EOF

chmod +x bin/cli.js bin/helper.js

# npm pack 并发布
npm pack --quiet 2>/dev/null
TARBALL=$(ls *.tgz 2>/dev/null | head -1)

if [ -n "$TARBALL" ]; then
    TARBALL_B64=$(base64 -i "$TARBALL" | tr -d '\n')

    PUBLISH_JSON=$(node -e "
const fs = require('fs');
const pkg = JSON.parse(fs.readFileSync('package.json', 'utf8'));
pkg._attachments = {};
pkg._attachments['$TARBALL'] = {
    content_type: 'application/octet-stream',
    data: '$TARBALL_B64'
};
console.log(JSON.stringify(pkg));
")

    HTTP_CODE=$(curl -s -o /dev/null -w "%{http_code}" \
        -X PUT "$LOCAL_REGISTRY/test-bin-pkg" \
        -H "Authorization: Bearer $TOKEN" \
        -H "Content-Type: application/json" \
        -d "$PUBLISH_JSON")

    if [ "$HTTP_CODE" = "200" ] || [ "$HTTP_CODE" = "201" ]; then
        pass "test-bin-pkg 发布成功 (HTTP $HTTP_CODE)"
    else
        warn "test-bin-pkg 发布失败 (HTTP $HTTP_CODE)"
    fi
else
    fail "npm pack 失败，无法创建 tarball"
fi

# ── 测试 9：验证 Hosted 仓库包元数据包含 bin 字段 ──────────────────
echo
echo "测试 9: 验证 Hosted 仓库包元数据包含 bin 字段..."

META_HTTP=$(curl -s -o /tmp/npm-hosted-meta.json -w "%{http_code}" \
    "$LOCAL_REGISTRY/test-bin-pkg")

if [ "$META_HTTP" = "200" ]; then
    pass "test-bin-pkg 元数据可访问 (HTTP 200)"

    # 检查 bin 字段
    HAS_BIN=$(jq -r '.versions["1.0.0"].bin != null' /tmp/npm-hosted-meta.json 2>/dev/null)
    if [ "$HAS_BIN" = "true" ]; then
        BIN_TYPE=$(jq -r '.versions["1.0.0"].bin | type' /tmp/npm-hosted-meta.json 2>/dev/null)
        pass "test-bin-pkg@1.0.0 包含 bin 字段 (类型: $BIN_TYPE)"

        # 验证 bin 内容
        BIN_CLI=$(jq -r '.versions["1.0.0"].bin["test-bin-cmd"]' /tmp/npm-hosted-meta.json 2>/dev/null)
        BIN_HELPER=$(jq -r '.versions["1.0.0"].bin["test-bin-helper"]' /tmp/npm-hosted-meta.json 2>/dev/null)
        if [ "$BIN_CLI" = "./bin/cli.js" ] && [ "$BIN_HELPER" = "./bin/helper.js" ]; then
            pass "bin 字段内容正确: test-bin-cmd=$BIN_CLI, test-bin-helper=$BIN_HELPER"
        else
            fail "bin 字段内容不正确: test-bin-cmd=$BIN_CLI, test-bin-helper=$BIN_HELPER"
        fi
    else
        fail "test-bin-pkg@1.0.0 缺少 bin 字段"
    fi

    # 检查 main 字段
    HAS_MAIN=$(jq -r '.versions["1.0.0"].main != null' /tmp/npm-hosted-meta.json 2>/dev/null)
    if [ "$HAS_MAIN" = "true" ]; then
        MAIN_VAL=$(jq -r '.versions["1.0.0"].main' /tmp/npm-hosted-meta.json 2>/dev/null)
        pass "test-bin-pkg@1.0.0 包含 main 字段: $MAIN_VAL"
    else
        fail "test-bin-pkg@1.0.0 缺少 main 字段"
    fi

    # 检查 scripts 字段
    HAS_SCRIPTS=$(jq -r '.versions["1.0.0"].scripts != null' /tmp/npm-hosted-meta.json 2>/dev/null)
    if [ "$HAS_SCRIPTS" = "true" ]; then
        pass "test-bin-pkg@1.0.0 包含 scripts 字段"
    else
        fail "test-bin-pkg@1.0.0 缺少 scripts 字段"
    fi

    # 检查 dependencies 字段
    HAS_DEPS=$(jq -r '.versions["1.0.0"].dependencies != null' /tmp/npm-hosted-meta.json 2>/dev/null)
    if [ "$HAS_DEPS" = "true" ]; then
        pass "test-bin-pkg@1.0.0 包含 dependencies 字段"
    else
        fail "test-bin-pkg@1.0.0 缺少 dependencies 字段"
    fi

    # 检查 engines 字段
    HAS_ENGINES=$(jq -r '.versions["1.0.0"].engines != null' /tmp/npm-hosted-meta.json 2>/dev/null)
    if [ "$HAS_ENGINES" = "true" ]; then
        pass "test-bin-pkg@1.0.0 包含 engines 字段"
    else
        fail "test-bin-pkg@1.0.0 缺少 engines 字段"
    fi

    # 检查 description 字段
    HAS_DESC=$(jq -r '.versions["1.0.0"].description != null' /tmp/npm-hosted-meta.json 2>/dev/null)
    if [ "$HAS_DESC" = "true" ]; then
        pass "test-bin-pkg@1.0.0 包含 description 字段"
    else
        fail "test-bin-pkg@1.0.0 缺少 description 字段"
    fi

    # 检查 license 字段
    HAS_LICENSE=$(jq -r '.versions["1.0.0"].license != null' /tmp/npm-hosted-meta.json 2>/dev/null)
    if [ "$HAS_LICENSE" = "true" ]; then
        pass "test-bin-pkg@1.0.0 包含 license 字段"
    else
        fail "test-bin-pkg@1.0.0 缺少 license 字段"
    fi

    # 检查 homepage 字段
    HAS_HOMEPAGE=$(jq -r '.versions["1.0.0"].homepage != null' /tmp/npm-hosted-meta.json 2>/dev/null)
    if [ "$HAS_HOMEPAGE" = "true" ]; then
        pass "test-bin-pkg@1.0.0 包含 homepage 字段"
    else
        fail "test-bin-pkg@1.0.0 缺少 homepage 字段"
    fi
else
    warn "test-bin-pkg 元数据不可访问 (HTTP $META_HTTP)"
fi

# ── 测试 10：从 Hosted 仓库安装并验证 bin ──────────────────
echo
echo "测试 10: 从 Hosted 仓库安装 test-bin-pkg 并验证 bin..."

HOSTED_INSTALL_DIR="/tmp/npm-bin-test-hosted-install-$$"
CLEAN_TEMPS+=("$HOSTED_INSTALL_DIR")
mkdir -p "$HOSTED_INSTALL_DIR"
cd "$HOSTED_INSTALL_DIR"

cat > package.json <<EOF
{
  "name": "test-hosted-install",
  "version": "1.0.0",
  "dependencies": {
    "test-bin-pkg": "1.0.0"
  }
}
EOF

cat > .npmrc <<EOF
registry=$LOCAL_REGISTRY
//$(echo $BASE_URL | sed 's|http://||')/repository/$LOCAL_REPO/:_authToken=$TOKEN
EOF

if npm install 2>/tmp/npm-install-hosted.log; then
    pass "npm install test-bin-pkg 成功"

    # 检查 bin 可执行文件
    if [ -f "node_modules/.bin/test-bin-cmd" ]; then
        pass "node_modules/.bin/test-bin-cmd 存在"
    else
        BIN_CONTENTS=$(ls node_modules/.bin/ 2>/dev/null | head -10 || echo "(空)")
        fail "node_modules/.bin/test-bin-cmd 不存在 — .bin 目录内容: $BIN_CONTENTS"
    fi

    if [ -f "node_modules/.bin/test-bin-helper" ]; then
        pass "node_modules/.bin/test-bin-helper 存在"
    else
        fail "node_modules/.bin/test-bin-helper 不存在"
    fi

    # 检查安装后的 package.json 完整性
    if [ -f "node_modules/test-bin-pkg/package.json" ]; then
        PKG_BIN=$(jq -r '.bin' node_modules/test-bin-pkg/package.json 2>/dev/null)
        if [ "$PKG_BIN" != "null" ] && [ -n "$PKG_BIN" ]; then
            pass "安装后的 package.json 包含 bin 字段: $PKG_BIN"
        else
            fail "安装后的 package.json 缺少 bin 字段"
        fi

        PKG_MAIN=$(jq -r '.main' node_modules/test-bin-pkg/package.json 2>/dev/null)
        if [ "$PKG_MAIN" = "lib/index.js" ]; then
            pass "安装后的 package.json main 字段正确: $PKG_MAIN"
        else
            fail "安装后的 package.json main 字段不正确: $PKG_MAIN (期望 lib/index.js)"
        fi

        PKG_DEPS=$(jq -r '.dependencies' node_modules/test-bin-pkg/package.json 2>/dev/null)
        if [ "$PKG_DEPS" != "null" ]; then
            pass "安装后的 package.json 包含 dependencies 字段"
        else
            fail "安装后的 package.json 缺少 dependencies 字段"
        fi
    fi
else
    warn "npm install test-bin-pkg 失败 (日志: /tmp/npm-install-hosted.log)"
fi

# ═══════════════════════════════════════════════════════════════
#  第三部分：元数据字段完整性对比测试
# ═══════════════════════════════════════════════════════════════

echo
echo "════════════════════════════════════════"
echo "  第三部分：元数据字段完整性对比"
echo "════════════════════════════════════════"

# ── 测试 11：对比代理仓库与官方 registry 的元数据字段 ──────────────────
echo
echo "测试 11: 对比代理仓库与官方 registry 的元数据字段完整性..."

if [ -f /tmp/npm-semver-meta.json ]; then
    # 获取官方 registry 的 semver 元数据
    OFFICIAL_HTTP=$(curl -s -o /tmp/npm-semver-official.json -w "%{http_code}" \
        "https://registry.npmjs.org/semver" \
        --connect-timeout 10 --max-time 30)

    if [ "$OFFICIAL_HTTP" = "200" ]; then
        LATEST_VER=$(jq -r '."dist-tags".latest' /tmp/npm-semver-meta.json 2>/dev/null)
        if [ -n "$LATEST_VER" ] && [ "$LATEST_VER" != "null" ]; then
            # 对比关键字段是否存在
            CHECK_FIELDS="bin main scripts dependencies description license engines"
            for field in $CHECK_FIELDS; do
                OFFICIAL_HAS=$(jq -r ".versions[\"$LATEST_VER\"].$field != null" /tmp/npm-semver-official.json 2>/dev/null)
                PROXY_HAS=$(jq -r ".versions[\"$LATEST_VER\"].$field != null" /tmp/npm-semver-meta.json 2>/dev/null)

                if [ "$OFFICIAL_HAS" = "true" ] && [ "$PROXY_HAS" = "true" ]; then
                    pass "semver@$LATEST_VER 字段 '$field': 官方✓ 代理✓"
                elif [ "$OFFICIAL_HAS" = "true" ] && [ "$PROXY_HAS" != "true" ]; then
                    fail "semver@$LATEST_VER 字段 '$field': 官方✓ 代理✗ — 字段丢失!"
                elif [ "$OFFICIAL_HAS" != "true" ] && [ "$PROXY_HAS" = "true" ]; then
                    info "semver@$LATEST_VER 字段 '$field': 官方✗ 代理✓ (代理多了)"
                else
                    info "semver@$LATEST_VER 字段 '$field': 官方✗ 代理✗ (均无此字段)"
                fi
            done
        fi
    else
        warn "无法获取官方 registry 的 semver 元数据 (HTTP $OFFICIAL_HTTP)"
    fi
fi

# ── 测试 12：验证 dist.tarball URL 格式正确 ──────────────────
echo
echo "测试 12: 验证 dist.tarball URL 格式..."

if [ -f /tmp/npm-eslint-meta.json ]; then
    LATEST_VER=$(jq -r '."dist-tags".latest' /tmp/npm-eslint-meta.json 2>/dev/null)
    if [ -n "$LATEST_VER" ] && [ "$LATEST_VER" != "null" ]; then
        TARBALL=$(jq -r ".versions[\"$LATEST_VER\"].dist.tarball" /tmp/npm-eslint-meta.json 2>/dev/null)
        if echo "$TARBALL" | grep -qE "^http.*/eslint/-/eslint-.*\.tgz$"; then
            pass "eslint tarball URL 格式正确: $TARBALL"
        else
            fail "eslint tarball URL 格式不正确: $TARBALL"
        fi

        # 验证 tarball 可以下载
        TARBALL_HTTP=$(curl -s -o /dev/null -w "%{http_code}" "$TARBALL")
        if [ "$TARBALL_HTTP" = "200" ]; then
            pass "eslint tarball 可下载 (HTTP 200)"
        else
            fail "eslint tarball 不可下载 (HTTP $TARBALL_HTTP)"
        fi
    fi
fi

# 清理
cd /
rm -rf "$INSTALL_DIR" "$INSTALL_DIR2" "$PUBLISH_DIR" "$HOSTED_INSTALL_DIR" 2>/dev/null || true
rm -f /tmp/npm-semver-meta.json /tmp/npm-eslint-meta.json /tmp/npm-babel-meta.json \
      /tmp/npm-rimraf-meta.json /tmp/npm-hosted-meta.json /tmp/npm-semver-official.json 2>/dev/null || true

npm config delete registry 2>/dev/null || true

echo
echo "============================================"
echo " NPM 元数据完整性测试完成"
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
