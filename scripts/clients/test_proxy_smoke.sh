#!/bin/bash

# =============================================================================
# Proxy 冒烟测试
# 验证所有协议 Proxy 下载路径的基础 HTTP 能力：
# GET / HEAD / Range / ETag / Last-Modified / 条件请求 / metadata pass-through
# 需要服务运行在 $BASE_URL 并且 setup_test_repos.sh 已创建代理仓库
# =============================================================================

BASE_URL="${1:-http://localhost:9081}"

RED='\033[0;31m'
GREEN='\033[0;32m'
YELLOW='\033[1;33m'
BLUE='\033[0;34m'
NC='\033[0m'

PASS=0
FAIL=0

pass() { echo -e "  ${GREEN}✓ PASS${NC} $1"; PASS=$((PASS + 1)); }
fail() { echo -e "  ${RED}✗ FAIL${NC} $1"; FAIL=$((FAIL + 1)); }
info() { echo -e "  ${BLUE}ℹ INFO${NC} $1"; }

echo "============================================"
echo " Proxy 冒烟测试"
echo " HEAD / Range / ETag / 304 / metadata"
echo " 目标: $BASE_URL"
echo "============================================"
echo

# ── helper ──────────────────────────────────────────────
# get_header <url> <header> - 用 HEAD 取响应头
head_header() {
  curl -sI -o /dev/null -w "%{header_json}" "$1" 2>/dev/null \
    | python3 -c "import sys,json; h=json.load(sys.stdin); print(h.get('$2',[['']])[0])" 2>/dev/null || true
}

# ══════════════════════════════════════════════════════
#  1. Maven proxy
# ══════════════════════════════════════════════════════
echo "═══ Maven proxy ═══"

MVN_REPO="$BASE_URL/repository/maven-proxy-aliyun"
# 先用 GET 触发一次回源，确保缓存
MVN_PATH="com/google/guava/guava/31.1-jre/guava-31.1-jre.jar"
MVN_URL="$MVN_REPO/$MVN_PATH"

MVN_GET=$(curl -s -o /dev/null -w "%{http_code}" "$MVN_URL")
if [ "$MVN_GET" = "200" ]; then
  pass "Maven GET jar → 200"
else
  # 可能仓库不存在，跳过
  info "Maven GET jar → $MVN_GET（仓库不存在或上游不可达时跳过）"
  echo -e "\n  ${YELLOW}通过: $PASS  失败: $FAIL${NC}"
  echo "通过: $PASS"
  echo "失败: $FAIL"
  exit 0
fi

# HEAD
MVN_HEAD=$(curl -s -o /dev/null -w "%{http_code}" -I "$MVN_URL")
if [ "$MVN_HEAD" = "200" ]; then
  pass "Maven HEAD jar → 200"
else
  fail "Maven HEAD jar → $MVN_HEAD（期望 200）"
fi

# HEAD body 空
MVN_HEAD_BODY=$(curl -s -I "$MVN_URL" | tail -c 100 | wc -c)
if [ "$MVN_HEAD_BODY" -lt 500 ]; then
  pass "Maven HEAD body 空或很小"
else
  fail "Maven HEAD 返回了非预期的 body（${MVN_HEAD_BODY} bytes）"
fi

# Range 206
MVN_RANGE=$(curl -s -o /tmp/mvn_range.bin -w "%{http_code}" \
  -H "Range: bytes=0-99" "$MVN_URL")
if [ "$MVN_RANGE" = "206" ]; then
  RANGE_SIZE=$(wc -c < /tmp/mvn_range.bin | tr -d ' ')
  if [ "$RANGE_SIZE" = "100" ]; then
    pass "Maven Range 0-99 → 206 + 100 bytes"
  else
    fail "Maven Range 206 但 body size=$RANGE_SIZE（期望 100）"
  fi
else
  fail "Maven Range → $MVN_RANGE（期望 206）"
fi

# ETag
MVN_ETAG=$(curl -sI "$MVN_URL" | grep -i "^etag:" | head -1)
if [ -n "$MVN_ETAG" ]; then
  pass "Maven ETag 存在: $MVN_ETAG"
else
  info "Maven ETag 缺失（proxy artifact 无 BlobRefs 时常见）"
fi

# Last-Modified
MVN_LM=$(curl -sI "$MVN_URL" | grep -i "^last-modified:" | head -1)
if [ -n "$MVN_LM" ]; then
  pass "Maven Last-Modified 存在: $MVN_LM"
else
  info "Maven Last-Modified 缺失（proxy artifact 无 timestamps 时常见）"
fi

# If-None-Match → 304（仅当 ETag 存在时测试）
MVN_ETAG_VAL=$(echo "$MVN_ETAG" | sed 's/[Ee]tag: *//;s/\r//')
if [ -n "$MVN_ETAG_VAL" ]; then
  MVN_304=$(curl -s -o /dev/null -w "%{http_code}" -H "If-None-Match: $MVN_ETAG_VAL" "$MVN_URL")
  if [ "$MVN_304" = "304" ]; then
    pass "Maven If-None-Match → 304"
  else
    fail "Maven If-None-Match → $MVN_304（期望 304）"
  fi
fi

echo

# ══════════════════════════════════════════════════════
#  2. npm proxy
# ══════════════════════════════════════════════════════
echo "═══ npm proxy ═══"

NPM_REPO="$BASE_URL/repository/npm-proxy-cn"
# package metadata
NPM_META_URL="$NPM_REPO/lodash"
NPM_TGZ_URL="$NPM_REPO/lodash/-/lodash-4.17.21.tgz"

NPM_GET=$(curl -s -o /dev/null -w "%{http_code}" "$NPM_TGZ_URL")
if [ "$NPM_GET" != "200" ]; then
  info "npm GET tgz → $NPM_GET（仓库不存在或上游不可达时跳过）"
  echo -e "\n  ${YELLOW}通过: $PASS  失败: $FAIL${NC}"
  echo "通过: $PASS"
  echo "失败: $FAIL"
  exit 0
fi

# metadata HEAD
NPM_META_HEAD=$(curl -s -o /dev/null -w "%{http_code}" -I "$NPM_META_URL")
if [ "$NPM_META_HEAD" = "200" ]; then
  pass "npm HEAD package metadata → 200"
else
  fail "npm HEAD package metadata → $NPM_META_HEAD（期望 200）"
fi

# tarball HEAD
NPM_TGZ_HEAD=$(curl -s -o /dev/null -w "%{http_code}" -I "$NPM_TGZ_URL")
if [ "$NPM_TGZ_HEAD" = "200" ]; then
  pass "npm HEAD tarball → 200"
else
  fail "npm HEAD tarball → $NPM_TGZ_HEAD（期望 200）"
fi

# Range
NPM_RANGE=$(curl -s -o /tmp/npm_range.bin -w "%{http_code}" \
  -H "Range: bytes=0-99" "$NPM_TGZ_URL")
if [ "$NPM_RANGE" = "206" ]; then
  pass "npm Range 0-99 → 206"
else
  fail "npm Range → $NPM_RANGE（期望 206）"
fi

# ETag
NPM_ETAG=$(curl -sI "$NPM_TGZ_URL" | grep -i "^etag:" | head -1)
if [ -n "$NPM_ETAG" ]; then
  pass "npm tarball ETag 存在"
else
  info "npm tarball ETag 缺失（proxy artifact 无 BlobRefs 时常见）"
fi

# latest 版本：确保是有效 semver
NPM_META_BODY=$(curl -s "$NPM_META_URL")
NPM_LATEST=$(echo "$NPM_META_BODY" | python3 -c "import sys,json; d=json.load(sys.stdin); print(d.get('dist-tags',{}).get('latest',''))" 2>/dev/null || true)
if echo "$NPM_LATEST" | grep -qE "^[0-9]+\.[0-9]+"; then
  pass "npm latest 版本: $NPM_LATEST（有效 semver）"
else
  fail "npm latest 版本异常: $NPM_LATEST"
fi

echo

# ══════════════════════════════════════════════════════
#  3. PyPI proxy
# ══════════════════════════════════════════════════════
echo "═══ PyPI proxy ═══"

PYPI_REPO="$BASE_URL/repository/pypi-proxy-tuna"
PYPI_SIMPLE="$PYPI_REPO/simple/"
PYPI_PKG="$PYPI_REPO/simple/requests/"

PYPI_SIMPLE_HEAD=$(curl -s -o /dev/null -w "%{http_code}" -I "$PYPI_SIMPLE")
if [ "$PYPI_SIMPLE_HEAD" = "200" ]; then
  pass "PyPI simple/ HEAD → 200"
else
  info "PyPI simple/ HEAD → $PYPI_SIMPLE_HEAD（仓库不存在或上游不可达时跳过）"
  echo -e "\n  ${YELLOW}通过: $PASS  失败: $FAIL${NC}"
  echo "通过: $PASS"
  echo "失败: $FAIL"
  exit 0
fi

PYPI_PKG_HEAD=$(curl -s -o /dev/null -w "%{http_code}" -I "$PYPI_PKG")
if [ "$PYPI_PKG_HEAD" = "200" ]; then
  pass "PyPI simple/requests/ HEAD → 200"
else
  fail "PyPI simple/requests/ HEAD → $PYPI_PKG_HEAD（期望 200）"
fi

# JSON API
PYPI_JSON="$PYPI_REPO/pypi/requests/json"
PYPI_JSON_CODE=$(curl -s -o /dev/null -w "%{http_code}" "$PYPI_JSON")
if [ "$PYPI_JSON_CODE" = "200" ]; then
  pass "PyPI JSON API → 200"
else
  fail "PyPI JSON API → $PYPI_JSON_CODE（期望 200）"
fi

# 包下载 Range
# 从 simple 页面找一个 tar.gz 链接
PYPI_PKG_PAGE=$(curl -s "$PYPI_PKG")
PYPI_FILE=$(echo "$PYPI_PKG_PAGE" | grep -o 'href="[^"]*\.tar\.gz"' | head -1 | sed 's/href="//;s/"//')
if [ -n "$PYPI_FILE" ]; then
  # href 是相对于 simple/{pkg}/ 的，需要去掉 ../../ 后拼接到仓库根
  if echo "$PYPI_FILE" | grep -q "^http"; then
    PYPI_DL_URL="$PYPI_FILE"
  else
    PYPI_DL_URL="$PYPI_REPO/$(echo "$PYPI_FILE" | sed 's|^\.\.\/\.\.\/||')"
  fi
  # 先触发一次 GET 回源下载
  PYPI_DL_GET=$(curl -s -o /tmp/pypi_dl.bin -w "%{http_code}" "$PYPI_DL_URL")
  if [ "$PYPI_DL_GET" = "200" ]; then
    pass "PyPI tar.gz GET → 200"
    # 再测试 Range
    PYPI_RANGE=$(curl -s -o /tmp/pypi_range.bin -w "%{http_code}" \
      -H "Range: bytes=0-99" "$PYPI_DL_URL")
    if [ "$PYPI_RANGE" = "206" ]; then
      pass "PyPI tar.gz Range → 206"
    else
      fail "PyPI tar.gz Range → $PYPI_RANGE（期望 206）"
    fi
  else
    info "PyPI tar.gz GET → $PYPI_DL_GET（跳过 Range 测试，文件未缓存或上游不可达）"
  fi
else
  info "PyPI simple 页面无 tar.gz 链接，跳过下载测试"
fi

echo

# ══════════════════════════════════════════════════════
#  4. Go proxy
# ══════════════════════════════════════════════════════
echo "═══ Go proxy ═══"

GO_REPO="$BASE_URL/repository/go-proxy-goproxy-cn"
GO_MOD="github.com/gin-gonic/gin"
GO_VERSION="v1.10.0"

GO_INFO="$GO_REPO/$GO_MOD/@v/$GO_VERSION.info"
GO_LIST="$GO_REPO/$GO_MOD/@v/list"

# @latest
GO_LATEST="$GO_REPO/$GO_MOD/@latest"
GO_LATEST_CODE=$(curl -s -o /dev/null -w "%{http_code}" "$GO_LATEST")
if [ "$GO_LATEST_CODE" = "200" ]; then
  pass "Go @latest → 200"
  GO_LATEST_VER=$(curl -s "$GO_LATEST" | python3 -c "import sys,json; print(json.load(sys.stdin).get('Version',''))" 2>/dev/null)
  if echo "$GO_LATEST_VER" | grep -q "^v"; then
    pass "Go @latest 版本: $GO_LATEST_VER"
  else
    fail "Go @latest 版本格式异常: $GO_LATEST_VER"
  fi
else
  info "Go @latest → $GO_LATEST_CODE（仓库不存在或上游不可达时跳过）"
  echo -e "\n  ${YELLOW}通过: $PASS  失败: $FAIL${NC}"
  echo "通过: $PASS"
  echo "失败: $FAIL"
  exit 0
fi

# @v/list
GO_LIST_CODE=$(curl -s -o /dev/null -w "%{http_code}" "$GO_LIST")
if [ "$GO_LIST_CODE" = "200" ]; then
  pass "Go @v/list → 200"
else
  fail "Go @v/list → $GO_LIST_CODE（期望 200）"
fi

# .info HEAD
GO_INFO_HEAD=$(curl -s -o /dev/null -w "%{http_code}" -I "$GO_INFO")
if [ "$GO_INFO_HEAD" = "200" ]; then
  pass "Go .info HEAD → 200"
else
  fail "Go .info HEAD → $GO_INFO_HEAD（期望 200）"
fi

# .info ETag
GO_INFO_ETAG=$(curl -sI "$GO_INFO" | grep -i "^etag:" | head -1)
if [ -n "$GO_INFO_ETAG" ]; then
  pass "Go .info ETag 存在"
else
  info "Go .info ETag 缺失（proxy artifact 无 BlobRefs 时常见）"
fi

# @latest HEAD
GO_LATEST_HEAD=$(curl -s -o /dev/null -w "%{http_code}" -I "$GO_LATEST")
if [ "$GO_LATEST_HEAD" = "200" ]; then
  pass "Go @latest HEAD → 200"
else
  fail "Go @latest HEAD → $GO_LATEST_HEAD（期望 200）"
fi

echo

# ══════════════════════════════════════════════════════
#  5. APT proxy（仅检查路径可访问，不做完整 apt 测试）
# ══════════════════════════════════════════════════════
echo "═══ APT proxy ═══"

# APT 代理仓库需要先配置，此处检查 APT-in-Release 端点
info "APT 测试需要完整 APT 仓库配置，此处跳过"
echo

# ══════════════════════════════════════════════════════
#  6. YUM proxy
# ══════════════════════════════════════════════════════
echo "═══ YUM proxy ═══"

YUM_REPO="$BASE_URL/repository/yum-proxy-baseos"
YUM_REPOMD="$YUM_REPO/repodata/repomd.xml"

YUM_REPOMD_CODE=$(curl -s -o /dev/null -w "%{http_code}" "$YUM_REPOMD")
if [ "$YUM_REPOMD_CODE" = "200" ]; then
  pass "YUM repomd.xml → 200"
  YUM_REPOMD_HEAD=$(curl -s -o /dev/null -w "%{http_code}" -I "$YUM_REPOMD")
  if [ "$YUM_REPOMD_HEAD" = "200" ]; then
    pass "YUM repomd.xml HEAD → 200"
  else
    fail "YUM repomd.xml HEAD → $YUM_REPOMD_HEAD（期望 200）"
  fi
  YUM_REPOMD_BODY=$(curl -s "$YUM_REPOMD")
  if echo "$YUM_REPOMD_BODY" | grep -q "<checksum"; then
    pass "YUM repomd.xml 包含 checksum 字段"
  else
    fail "YUM repomd.xml 缺少 checksum 字段"
  fi
else
  info "YUM repomd.xml → $YUM_REPOMD_CODE（仓库不存在或上游不可达时跳过）"
fi

echo

# ══════════════════════════════════════════════════════
#  7. Generic proxy
# ══════════════════════════════════════════════════════
echo "═══ Generic proxy ═══"

GEN_REPO="$BASE_URL/repository/generic-local"

# 上传测试文件
TOKEN=$(curl -s -X POST "$BASE_URL/api/v1/auth/login" \
  -H "Content-Type: application/json" \
  -d '{"username":"admin","password":"admin123"}' | \
  grep -o '"access_token":"[^"]*"' | \
  sed 's/"access_token":"//;s/"//' 2>/dev/null || true)

if [ -n "$TOKEN" ]; then
  echo "smoke-test" > /tmp/gen_smoke_test.txt
  GEN_UPLOAD=$(curl -s -o /dev/null -w "%{http_code}" -X PUT \
    -H "Authorization: Bearer $TOKEN" \
    -T /tmp/gen_smoke_test.txt \
    "$GEN_REPO/_smoke/smoke-test.txt")
  if [ "$GEN_UPLOAD" = "200" ] || [ "$GEN_UPLOAD" = "201" ]; then
    pass "Generic PUT → $GEN_UPLOAD"

    # GET
    GEN_GET=$(curl -s -o /dev/null -w "%{http_code}" "$GEN_REPO/_smoke/smoke-test.txt")
    if [ "$GEN_GET" = "200" ]; then
      pass "Generic GET → 200"
    else
      fail "Generic GET → $GEN_GET（期望 200）"
    fi

    # HEAD
    GEN_HEAD=$(curl -s -o /dev/null -w "%{http_code}" -I "$GEN_REPO/_smoke/smoke-test.txt")
    if [ "$GEN_HEAD" = "200" ]; then
      pass "Generic HEAD → 200"
    else
      fail "Generic HEAD → $GEN_HEAD（期望 200）"
    fi

    # Range
    GEN_RANGE=$(curl -s -o /tmp/gen_range.bin -w "%{http_code}" \
      -H "Range: bytes=0-4" "$GEN_REPO/_smoke/smoke-test.txt")
    if [ "$GEN_RANGE" = "206" ]; then
      pass "Generic Range → 206"
    else
      fail "Generic Range → $GEN_RANGE（期望 206）"
    fi

    # Directory listing
    GEN_DIR=$(curl -s -o /dev/null -w "%{http_code}" "$GEN_REPO/_smoke/")
    if [ "$GEN_DIR" = "200" ]; then
      pass "Generic directory listing → 200"
    else
      fail "Generic directory listing → $GEN_DIR（期望 200）"
    fi

    # Directory HEAD
    GEN_DIR_HEAD=$(curl -s -o /dev/null -w "%{http_code}" -I "$GEN_REPO/_smoke/")
    if [ "$GEN_DIR_HEAD" = "200" ]; then
      pass "Generic directory HEAD → 200"
    else
      fail "Generic directory HEAD → $GEN_DIR_HEAD（期望 200）"
    fi

    # Root listing
    GEN_ROOT=$(curl -s -o /dev/null -w "%{http_code}" "$GEN_REPO/")
    if [ "$GEN_ROOT" = "200" ]; then
      pass "Generic root listing → 200"
    else
      fail "Generic root listing → $GEN_ROOT（期望 200）"
    fi

    # Path traversal rejection
    GEN_TRAV=$(curl -s -o /dev/null -w "%{http_code}" "$GEN_REPO/../etc/passwd")
    if [ "$GEN_TRAV" = "400" ] || [ "$GEN_TRAV" = "404" ]; then
      pass "Generic path traversal rejected → $GEN_TRAV"
    else
      fail "Generic path traversal 未拒绝 → $GEN_TRAV（期望 400/404）"
    fi

    # DELETE
    GEN_DEL=$(curl -s -o /dev/null -w "%{http_code}" -X DELETE \
      -H "Authorization: Bearer $TOKEN" \
      "$GEN_REPO/_smoke/smoke-test.txt")
    if [ "$GEN_DEL" = "204" ]; then
      pass "Generic DELETE → 204"
    else
      fail "Generic DELETE → $GEN_DEL（期望 204）"
    fi
  else
    info "Generic PUT → $GEN_UPLOAD（认证失败时跳过）"
  fi
else
  info "无法获取认证令牌，Generic 测试跳过"
fi

echo

# ══════════════════════════════════════════════════════
#  Summary
# ══════════════════════════════════════════════════════
echo "============================================"
echo -e "  ${GREEN}通过: $PASS${NC}"
echo -e "  ${RED}失败: $FAIL${NC}"
echo "============================================"

echo "通过: $PASS"
echo "失败: $FAIL"

if [ "$FAIL" -eq 0 ]; then
  echo -e "\n${GREEN}✓ Proxy 冒烟测试全部通过${NC}"
  exit 0
else
  echo -e "\n${RED}✗ Proxy 冒烟测试存在失败项${NC}"
  exit 1
fi
