#!/bin/bash

# =============================================================================
# Maven Hosted 高级 SNAPSHOT / classifier 冒烟测试
# 验证 release metadata、SNAPSHOT 多 build、sources/javadoc classifier、pom/jar extension、metadata checksum
# =============================================================================

set -u

BASE_URL="${1:-http://localhost:9081}"
ADMIN_USER="${ADMIN_USER:-admin}"
ADMIN_PASS="${ADMIN_PASS:-admin123}"
REPO="maven-local"
REPO_URL="$BASE_URL/repository/$REPO"

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
warn() { echo -e "  ${YELLOW}⚠ WARN${NC} $1"; }

echo "============================================"
echo " Maven Hosted 高级冒烟测试"
echo " Release / SNAPSHOT / classifier / checksum"
echo " 目标: $REPO_URL"
echo "============================================"
echo

TOKEN=$(curl -s -X POST "$BASE_URL/api/v1/auth/login" \
  -H "Content-Type: application/json" \
  -d "{\"username\":\"$ADMIN_USER\",\"password\":\"$ADMIN_PASS\"}" | \
  grep -o '"access_token":"[^"]*"' | \
  sed 's/"access_token":"//;s/"//' 2>/dev/null || true)

if [ -z "$TOKEN" ]; then
  warn "无法获取认证令牌，跳过 Maven hosted 冒烟测试"
  echo "通过: $PASS"
  echo "失败: $FAIL"
  exit 0
fi

# 确保 maven-local 仓库存在
HTTP_CODE=$(curl -s -o /dev/null -w "%{http_code}" -H "Authorization: Bearer $TOKEN" "$BASE_URL/api/v1/repositories/$REPO")
if [ "$HTTP_CODE" != "200" ]; then
  warn "$REPO 不存在，请先运行 scripts/setup_test_repos.sh，跳过"
  echo "通过: $PASS"
  echo "失败: $FAIL"
  exit 0
fi

TMP_DIR="/tmp/maven-advanced-smoke-$$"
mkdir -p "$TMP_DIR"
trap 'rm -rf "$TMP_DIR"' EXIT

upload_file() {
  local path="$1"
  local content="$2"
  local file="$TMP_DIR/$(basename "$path")"
  printf "%s" "$content" > "$file"
  curl -s -o /tmp/maven_upload_body.txt -w "%{http_code}" -X PUT \
    -H "Authorization: Bearer $TOKEN" \
    --data-binary "@$file" \
    "$REPO_URL/$path"
}

# ── Release metadata ─────────────────────────────────
echo "═══ Release metadata ═══"
REL_JAR="com/smoke/app/1.0.0/app-1.0.0.jar"
REL_CODE=$(upload_file "$REL_JAR" "release-jar-content")
if [ "$REL_CODE" = "200" ] || [ "$REL_CODE" = "201" ]; then
  pass "Release jar 上传 → $REL_CODE"
else
  fail "Release jar 上传 → $REL_CODE: $(cat /tmp/maven_upload_body.txt)"
fi

REL_META_URL="$REPO_URL/com/smoke/app/maven-metadata.xml"
REL_META=$(curl -s "$REL_META_URL")
if echo "$REL_META" | grep -q "<version>1.0.0</version>" && echo "$REL_META" | grep -q "<release>1.0.0</release>"; then
  pass "Release maven-metadata.xml 自动生成"
else
  fail "Release maven-metadata.xml 内容不完整: $REL_META"
fi

REL_SHA1=$(curl -s "$REL_META_URL.sha1")
if echo "$REL_SHA1" | grep -q "maven-metadata.xml" && [ "${#REL_SHA1}" -gt 40 ]; then
  pass "Release metadata sha1 生成"
else
  fail "Release metadata sha1 异常: $REL_SHA1"
fi

echo

# ── SNAPSHOT multi-build ─────────────────────────────
echo "═══ SNAPSHOT 多 build ═══"
SNAP_OLD="com/smoke/app/1.0-SNAPSHOT/app-1.0-20260609.100000-1.jar"
SNAP_NEW="com/smoke/app/1.0-SNAPSHOT/app-1.0-20260609.120000-2.jar"
for path in "$SNAP_OLD" "$SNAP_NEW"; do
  CODE=$(upload_file "$path" "$path")
  if [ "$CODE" = "200" ] || [ "$CODE" = "201" ]; then
    pass "SNAPSHOT 上传 $(basename "$path") → $CODE"
  else
    fail "SNAPSHOT 上传 $(basename "$path") → $CODE"
  fi
done

SNAP_META_URL="$REPO_URL/com/smoke/app/1.0-SNAPSHOT/maven-metadata.xml"
SNAP_META=$(curl -s "$SNAP_META_URL")
if echo "$SNAP_META" | grep -q "<timestamp>20260609.120000</timestamp>" && \
   echo "$SNAP_META" | grep -q "<buildNumber>2</buildNumber>" && \
   echo "$SNAP_META" | grep -q "<value>1.0-20260609.120000-2</value>"; then
  pass "SNAPSHOT metadata 选择最新 build"
else
  fail "SNAPSHOT metadata 未选择最新 build: $SNAP_META"
fi

echo

# ── SNAPSHOT classifier / pom ────────────────────────
echo "═══ SNAPSHOT classifier / pom ═══"
for path in \
  "com/smoke/app/1.0-SNAPSHOT/app-1.0-20260609.120000-2-sources.jar" \
  "com/smoke/app/1.0-SNAPSHOT/app-1.0-20260609.120000-2-javadoc.jar" \
  "com/smoke/app/1.0-SNAPSHOT/app-1.0-20260609.120000-2.pom"; do
  CODE=$(upload_file "$path" "$path")
  if [ "$CODE" = "200" ] || [ "$CODE" = "201" ]; then
    pass "SNAPSHOT classifier/pom 上传 $(basename "$path") → $CODE"
  else
    fail "SNAPSHOT classifier/pom 上传 $(basename "$path") → $CODE"
  fi
done

SNAP_META=$(curl -s "$SNAP_META_URL")
for marker in \
  "<classifier>sources</classifier>" \
  "<classifier>javadoc</classifier>" \
  "<extension>jar</extension>" \
  "<extension>pom</extension>"; do
  if echo "$SNAP_META" | grep -q "$marker"; then
    pass "SNAPSHOT metadata 包含 $marker"
  else
    fail "SNAPSHOT metadata 缺少 $marker: $SNAP_META"
  fi
done

echo

# ── checksum exactness ───────────────────────────────
echo "═══ metadata checksum 精确性 ═══"
META_FILE="$TMP_DIR/maven-metadata.xml"
SHA_FILE="$TMP_DIR/maven-metadata.xml.sha1"
curl -s "$REL_META_URL" > "$META_FILE"
curl -s "$REL_META_URL.sha1" > "$SHA_FILE"
LOCAL_SHA=$(shasum -a 1 "$META_FILE" | awk '{print $1}')
REMOTE_SHA=$(awk '{print $1}' "$SHA_FILE")
if [ "$LOCAL_SHA" = "$REMOTE_SHA" ]; then
  pass "metadata sha1 与实际 metadata bytes 一致"
else
  fail "metadata sha1 不一致: local=$LOCAL_SHA remote=$REMOTE_SHA body=$(cat "$SHA_FILE")"
fi

echo

echo "============================================"
echo -e "  ${GREEN}通过: $PASS${NC}"
echo -e "  ${RED}失败: $FAIL${NC}"
echo "============================================"
echo "通过: $PASS"
echo "失败: $FAIL"

if [ "$FAIL" -eq 0 ]; then
  echo -e "\n${GREEN}✓ Maven Hosted 高级冒烟测试通过${NC}"
  exit 0
else
  echo -e "\n${RED}✗ Maven Hosted 高级冒烟测试存在失败项${NC}"
  exit 1
fi
