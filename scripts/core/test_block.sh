#!/bin/bash
set -e

BASE="http://localhost:9081"
PASS=0
FAIL=0

pass() { PASS=$((PASS+1)); echo -e "  \033[32m✓ PASS\033[0m $1"; }
fail() { FAIL=$((FAIL+1)); echo -e "  \033[31m✗ FAIL\033[0m $1"; }
info() { echo -e "  \033[36mℹ INFO\033[0m $1"; }

echo "============================================"
echo " 阻断(Block)功能测试"
echo " 目标: $BASE"
echo "============================================"

# 1. 获取 TOKEN
RESP=$(curl -s "$BASE/api/v1/auth/login" -H "Content-Type: application/json" -d '{"username":"admin","password":"admin123"}')
TOKEN=$(echo "$RESP" | python3 -c "import sys,json; print(json.load(sys.stdin)['data']['access_token'])")
echo "TOKEN: ${TOKEN:0:30}..."

# --- 用 maven 测试：先上传一个测试 jar，然后阻断它 ---
GROUP_ID="com/test/block"
ARTIFACT_ID="block-test"
VERSION="1.0.0"
JAR_PATH="$GROUP_ID/$ARTIFACT_ID/$VERSION/$ARTIFACT_ID-$VERSION.jar"
POM_PATH="$GROUP_ID/$ARTIFACT_ID/$VERSION/$ARTIFACT_ID-$VERSION.pom"

# 2. 上传测试 POM
info "上传测试 pom 文件到 maven-local..."
POM_CONTENT='<project><modelVersion>4.0.0</modelVersion><groupId>com.test.block</groupId><artifactId>block-test</artifactId><version>1.0.0</version><packaging>jar</packaging></project>'
UPLOAD_POM=$(curl -s -o /dev/null -w "%{http_code}" -X PUT "$BASE/repo/maven-local/$POM_PATH" \
  -H "Authorization: Bearer $TOKEN" \
  -H "Content-Type: application/xml" \
  -d "$POM_CONTENT")
echo "  POM 上传 HTTP $UPLOAD_POM"

# 上传测试 JAR (minimal jar)
info "上传测试 jar 文件..."
echo "dummy jar content" > /tmp/test-block.jar
UPLOAD_JAR=$(curl -s -o /dev/null -w "%{http_code}" -X PUT "$BASE/repo/maven-local/$JAR_PATH" \
  -H "Authorization: Bearer $TOKEN" \
  -T /tmp/test-block.jar)
echo "  JAR 上传 HTTP $UPLOAD_JAR"

# 3. 下载测试（确认能正常下载）
DOWNLOAD_OK=$(curl -s -o /dev/null -w "%{http_code}" "$BASE/repo/maven-local/$JAR_PATH" \
  -H "Authorization: Bearer $TOKEN")
if [ "$DOWNLOAD_OK" = "200" ]; then
  pass "阻断前下载正常 (HTTP 200)"
else
  fail "阻断前下载异常 (HTTP $DOWNLOAD_OK)"
fi

# 4. 创建阻断规则（精确匹配包名+版本）
# 注意：Maven 的 Name 是 groupId:artifactId 格式
info "创建阻断规则..."
RULE_RESP=$(curl -s -X POST "$BASE/api/v1/block-rules" \
  -H "Authorization: Bearer $TOKEN" \
  -H "Content-Type: application/json" \
  -d "{\"package_name\":\"com.test.block:block-test\",\"version\":\"1.0.0\",\"match_type\":\"exact\",\"package_type\":\"maven\",\"reason\":\"安全漏洞测试\",\"enabled\":true}")
echo "  规则创建: $RULE_RESP"

RULE_ID=$(echo "$RULE_RESP" | python3 -c "import sys,json; d=json.load(sys.stdin); print(d.get('data',d).get('id',''))" 2>/dev/null)

if [ -n "$RULE_ID" ] && [ "$RULE_ID" != "0" ]; then
  pass "阻断规则创建成功 (ID=$RULE_ID)"
else
  fail "阻断规则创建失败"
fi

# 5. 再次下载测试（应被阻断返回 403）
DOWNLOAD_BLOCKED=$(curl -s -o /dev/null -w "%{http_code}" "$BASE/repo/maven-local/$JAR_PATH" \
  -H "Authorization: Bearer $TOKEN")
if [ "$DOWNLOAD_BLOCKED" = "403" ]; then
  pass "阻断后下载返回 403 (符合预期)"
else
  fail "阻断后下载未返回 403 (HTTP $DOWNLOAD_BLOCKED)"
fi

# 6. 查看阻断响应内容
BLOCK_MSG=$(curl -s "$BASE/repo/maven-local/$JAR_PATH" -H "Authorization: Bearer $TOKEN")
echo "  阻断消息: $BLOCK_MSG"

# 7. 阻断后删除文件（应仍然被阻断 - 阻断优先于存储）
# 实际上删除不应被阻断影响
DELETE_OK=$(curl -s -o /dev/null -w "%{http_code}" -X DELETE "$BASE/repo/maven-local/$JAR_PATH" \
  -H "Authorization: Bearer $TOKEN")
echo "  阻断后删除 HTTP $DELETE_OK"

# 8. 删除阻断规则
if [ -n "$RULE_ID" ] && [ "$RULE_ID" != "0" ]; then
  DELETE_RESP=$(curl -s -X DELETE "$BASE/api/v1/block-rules/$RULE_ID" \
    -H "Authorization: Bearer $TOKEN")
  echo "  删除规则: $DELETE_RESP"
  pass "阻断规则已清理"
fi

# 9. 删除后验证下载恢复（但文件已被删，所以应该是 404）
DOWNLOAD_AFTER=$(curl -s -o /dev/null -w "%{http_code}" "$BASE/repo/maven-local/$JAR_PATH")
echo "  规则删除后下载 HTTP $DOWNLOAD_AFTER (文件已被删=404合理)"

# 总结
echo ""
echo "============================================"
echo " 测试汇总"
echo "============================================"
echo -e "  通过: $PASS"
echo -e "  失败: $FAIL"
echo -e "  总计: $((PASS+FAIL))"

if [ $FAIL -eq 0 ]; then
  echo -e "\033[32m✅ Block 功能正常! 所有测试通过\033[0m"
  rm -f /tmp/test-block.jar
  exit 0
else
  echo -e "\033[31m❌ 部分测试失败\033[0m"
  rm -f /tmp/test-block.jar
  exit 1
fi
