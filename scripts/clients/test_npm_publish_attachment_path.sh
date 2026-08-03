#!/bin/bash

# =============================================================================
# NPM Publish 附件路径前缀回归测试
# 验证 _attachments key 包含路径前缀时不会触发 "filename must not contain slash" 错误
#
# 背景: 某些 npm 客户端或代理会在 _attachments key 中包含完整路径，
# 如 "@scope/pkg/-/file.tgz"，这会导致 artifact.Filename 包含斜杠而被拒绝。
#
# 修复: plugin.go 在使用 tarballName 前用 path.Base() 提取纯文件名。
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

pass() { echo -e "  ${GREEN}✓ PASS${NC} $1"; PASS_COUNT=$((PASS_COUNT + 1)); }
fail() { echo -e "  ${RED}✗ FAIL${NC} $1"; FAIL_COUNT=$((FAIL_COUNT + 1)); }
info() { echo -e "  ${BLUE}ℹ INFO${NC} $1"; }

echo "============================================"
echo " NPM Publish 附件路径前缀回归测试"
echo " 目标: $BASE_URL"
echo "============================================"
echo

# 获取认证令牌
info "获取认证令牌..."
TOKEN=$(curl -s -X POST "$BASE_URL/api/v1/auth/login" \
    -H "Content-Type: application/json" \
    -d "{\"username\":\"$ADMIN_USER\",\"password\":\"$ADMIN_PASS\"}" | \
    grep -o '"access_token":"[^"]*"' | \
    sed 's/"access_token":"//;s/"//')

if [ -z "$TOKEN" ]; then
    fail "获取令牌失败"
    exit 1
fi
pass "获取令牌成功"

# 创建 hosted npm 仓库
REPO_NAME="npm-attach-test"
info "创建测试仓库 $REPO_NAME..."
CREATE_RESP=$(curl -s -X POST "$BASE_URL/api/v1/repositories" \
    -H "Authorization: Bearer $TOKEN" \
    -H "Content-Type: application/json" \
    -d "{
        \"name\": \"$REPO_NAME\",
        \"format\": \"npm\",
        \"type\": \"hosted\",
        \"storage_config\": {
            \"type\": \"local\"
        }
    }")

if echo "$CREATE_RESP" | grep -q '"code":0'; then
    pass "仓库创建成功"
elif echo "$CREATE_RESP" | grep -q 'already exists'; then
    info "仓库已存在，继续测试"
else
    fail "仓库创建失败: $CREATE_RESP"
    exit 1
fi

# 测试场景 1: 普通文件名（基准）
echo
echo "测试场景 1: 普通 tarball 文件名"
PACKAGE1="test-normal-attach"
TARBALL1="$PACKAGE1-1.0.0.tgz"
TARBALL_DATA=$(echo -n "test-tarball-content" | base64)

PUBLISH_BODY1=$(cat <<EOF
{
  "name": "$PACKAGE1",
  "versions": {
    "1.0.0": {
      "name": "$PACKAGE1",
      "version": "1.0.0",
      "description": "Normal attachment test"
    }
  },
  "_attachments": {
    "$TARBALL1": {
      "content_type": "application/octet-stream",
      "data": "$TARBALL_DATA"
    }
  }
}
EOF
)

PUBLISH_RESP1=$(curl -s -w "\n%{http_code}" -X PUT "$BASE_URL/$REPO_NAME/$PACKAGE1" \
    -H "Authorization: Bearer $TOKEN" \
    -H "Content-Type: application/json" \
    -d "$PUBLISH_BODY1")

HTTP_CODE1=$(echo "$PUBLISH_RESP1" | tail -n1)
BODY1=$(echo "$PUBLISH_RESP1" | head -n-1)

if [ "$HTTP_CODE1" = "201" ] || [ "$HTTP_CODE1" = "200" ]; then
    pass "场景 1 发布成功 (HTTP $HTTP_CODE1)"
else
    fail "场景 1 发布失败 (HTTP $HTTP_CODE1): $BODY1"
fi

# 测试场景 2: 附件 key 包含路径前缀（scoped package 格式）
echo
echo "测试场景 2: 附件 key 包含路径前缀 (@scope/pkg/-/file.tgz)"
PACKAGE2="@test-scope/scoped-attach"
TARBALL2_KEY="@test-scope/scoped-attach/-/scoped-attach-1.0.0.tgz"  # 包含路径前缀
TARBALL2_DATA=$(echo -n "test-scoped-tarball" | base64)

PUBLISH_BODY2=$(cat <<EOF
{
  "name": "$PACKAGE2",
  "versions": {
    "1.0.0": {
      "name": "$PACKAGE2",
      "version": "1.0.0",
      "description": "Scoped attachment with path prefix test"
    }
  },
  "_attachments": {
    "$TARBALL2_KEY": {
      "content_type": "application/octet-stream",
      "data": "$TARBALL2_DATA"
    }
  }
}
EOF
)

PUBLISH_RESP2=$(curl -s -w "\n%{http_code}" -X PUT "$BASE_URL/$REPO_NAME/$PACKAGE2" \
    -H "Authorization: Bearer $TOKEN" \
    -H "Content-Type: application/json" \
    -d "$PUBLISH_BODY2")

HTTP_CODE2=$(echo "$PUBLISH_RESP2" | tail -n1)
BODY2=$(echo "$PUBLISH_RESP2" | head -n-1)

if [ "$HTTP_CODE2" = "201" ] || [ "$HTTP_CODE2" = "200" ]; then
    pass "场景 2 发布成功 (HTTP $HTTP_CODE2) - 路径前缀已正确处理"

    # 额外验证: 检查是否有 "filename must not contain slash" 错误
    if echo "$BODY2" | grep -qi "filename must not contain slash"; then
        fail "场景 2 响应包含斜杠错误 (修复失效)"
    else
        pass "场景 2 无斜杠错误"
    fi
else
    fail "场景 2 发布失败 (HTTP $HTTP_CODE2): $BODY2"

    # 检查是否是目标错误
    if echo "$BODY2" | grep -qi "filename must not contain slash"; then
        fail "❌ 关键回归失败: 仍然触发 'filename must not contain slash' 错误"
    fi
fi

# 测试场景 3: 附件 key 包含多级路径（极端情况）
echo
echo "测试场景 3: 附件 key 包含多级路径"
PACKAGE3="deep-path-attach"
TARBALL3_KEY="some/deep/path/to/$PACKAGE3-1.0.0.tgz"  # 多级路径
TARBALL3_DATA=$(echo -n "test-deep-path" | base64)

PUBLISH_BODY3=$(cat <<EOF
{
  "name": "$PACKAGE3",
  "versions": {
    "1.0.0": {
      "name": "$PACKAGE3",
      "version": "1.0.0",
      "description": "Deep path attachment test"
    }
  },
  "_attachments": {
    "$TARBALL3_KEY": {
      "content_type": "application/octet-stream",
      "data": "$TARBALL3_DATA"
    }
  }
}
EOF
)

PUBLISH_RESP3=$(curl -s -w "\n%{http_code}" -X PUT "$BASE_URL/$REPO_NAME/$PACKAGE3" \
    -H "Authorization: Bearer $TOKEN" \
    -H "Content-Type: application/json" \
    -d "$PUBLISH_BODY3")

HTTP_CODE3=$(echo "$PUBLISH_RESP3" | tail -n1)
BODY3=$(echo "$PUBLISH_RESP3" | head -n-1)

if [ "$HTTP_CODE3" = "201" ] || [ "$HTTP_CODE3" = "200" ]; then
    pass "场景 3 发布成功 (HTTP $HTTP_CODE3)"
else
    fail "场景 3 发布失败 (HTTP $HTTP_CODE3): $BODY3"
fi

# 验证下载
echo
echo "验证发布的包可正常下载"
DOWNLOAD_RESP=$(curl -s -w "\n%{http_code}" "$BASE_URL/$REPO_NAME/$PACKAGE2/-/scoped-attach-1.0.0.tgz" \
    -H "Authorization: Bearer $TOKEN")

DOWNLOAD_CODE=$(echo "$DOWNLOAD_RESP" | tail -n1)
if [ "$DOWNLOAD_CODE" = "200" ]; then
    pass "场景 2 包下载成功"
else
    fail "场景 2 包下载失败 (HTTP $DOWNLOAD_CODE)"
fi

# 清理（可选）
echo
info "清理测试仓库 $REPO_NAME..."
curl -s -X DELETE "$BASE_URL/api/v1/repositories/$REPO_NAME" \
    -H "Authorization: Bearer $TOKEN" > /dev/null

# 汇总
echo
echo "============================================"
echo " 测试完成"
echo "============================================"
echo -e " ${GREEN}通过${NC}: $PASS_COUNT"
echo -e " ${RED}失败${NC}: $FAIL_COUNT"
echo "============================================"

if [ $FAIL_COUNT -eq 0 ]; then
    echo -e "${GREEN}✓ 所有测试通过${NC}"
    exit 0
else
    echo -e "${RED}✗ 存在失败测试${NC}"
    exit 1
fi
