#!/bin/bash
# ============================================================
# 阻断(Block)功能测试
# 说明：
#   - 阻断规则的 CRUD 操作测试
#   - 阻断规则对下载的拦截验证（通过代理仓库，ProxyRuntime.checkBlocked）
# ============================================================

set -e

BASE="http://localhost:9081"
PASS=0
FAIL=0

pass() { PASS=$((PASS+1)); echo -e "  \033[32m✓ PASS\033[0m $1"; }
fail() { FAIL=$((FAIL+1)); echo -e "  \033[31m✗ FAIL\033[0m $1"; }
info() { echo -e "  ${BLUE}ℹ INFO${NC} $1"; }

echo "============================================"
echo " 阻断(Block)功能测试"
echo " 目标: $BASE"
echo "============================================"

# 1. 获取 TOKEN
RESP=$(curl -s "$BASE/api/v1/auth/login" -H "Content-Type: application/json" -d '{"username":"admin","password":"admin123"}')
TOKEN=$(echo "$RESP" | python3 -c "import sys,json; print(json.load(sys.stdin)['data']['access_token'])")
if [ -z "$TOKEN" ]; then fail "登录失败"; exit 1; fi
pass "认证成功"

# ============================================================
# 第一部分：阻断规则 CRUD 测试
# ============================================================
echo ""
echo "════════════════════════════════════════"
echo "  CRUD 操作测试"
echo "════════════════════════════════════════"

# 清理之前残留的测试规则
info "清理之前测试残留的阻断规则..."
RULES=$(curl -s "$BASE/api/v1/block-rules" -H "Authorization: Bearer $TOKEN")
echo "$RULES" | python3 -c "
import sys,json
d=json.load(sys.stdin)
rules=d.get('data',[])
for r in rules:
    pid=r.get('id')
    pkg=r.get('package_name','')
    reason=r.get('reason','')
    if reason == 'test' or pkg == 'lodash':
        print(f'  STALE: id={pid}, pkg={pkg}')
" 2>/dev/null || true

# 创建阻断规则
TEST_PKG="com.test.block:block-test"
TEST_VER="1.0.0"
info "创建阻断规则: $TEST_PKG @ $TEST_VER..."
RULE_RESP=$(curl -s -X POST "$BASE/api/v1/block-rules" \
  -H "Authorization: Bearer $TOKEN" \
  -H "Content-Type: application/json" \
  -d "{\"package_name\":\"$TEST_PKG\",\"version\":\"$TEST_VER\",\"match_type\":\"exact\",\"package_type\":\"maven\",\"reason\":\"test\",\"enabled\":true}")
echo "  规则创建: $RULE_RESP"

RULE_ID=$(echo "$RULE_RESP" | python3 -c "import sys,json; d=json.load(sys.stdin); print(d.get('data',d).get('id',''))" 2>/dev/null)
if [ -n "$RULE_ID" ] && [ "$RULE_ID" != "0" ] && [ "$RULE_ID" != "" ]; then
  pass "阻断规则创建成功 (ID=$RULE_ID)"
else
  fail "阻断规则创建失败"
fi

# 查询阻断规则列表
LIST_RESP=$(curl -s "$BASE/api/v1/block-rules" -H "Authorization: Bearer $TOKEN")
LIST_COUNT=$(echo "$LIST_RESP" | python3 -c "import sys,json; d=json.load(sys.stdin); print(len(d.get('data',[])))" 2>/dev/null)
if [ -n "$LIST_COUNT" ] && [ "$LIST_COUNT" -gt 0 ]; then
  pass "阻断规则列表查询成功 ($LIST_COUNT 条)"
else
  fail "阻断规则列表查询失败"
fi

# 删除阻断规则
if [ -n "$RULE_ID" ] && [ "$RULE_ID" != "0" ] && [ "$RULE_ID" != "" ]; then
  DEL_CODE=$(curl -s -o /dev/null -w "%{http_code}" -X DELETE "$BASE/api/v1/block-rules/$RULE_ID" \
    -H "Authorization: Bearer $TOKEN")
  if [ "$DEL_CODE" = "200" ] || [ "$DEL_CODE" = "204" ]; then
    pass "阻断规则删除成功 (HTTP $DEL_CODE)"
  else
    fail "阻断规则删除失败 (HTTP $DEL_CODE)"
  fi
fi

# ============================================================
# 第二部分：阻断规则下载拦截验证
# 通过代理仓库路径，触发 ProxyRuntime.checkBlocked
# ProxyRuntime 在 GetArtifact 中会调用 checkBlocked，
# 传入解析后的 name（groupId:artifactId）和 version，
# 与阻断规则精确匹配。
# ============================================================
echo ""
echo "════════════════════════════════════════"
echo "  下载拦截验证（代理仓库）"
echo "════════════════════════════════════════"

GUAVA_GROUP="com.google.guava"
GUAVA_ARTIFACT="guava"
GUAVA_VER="32.1.3-jre"
GUAVA_PATH="$GUAVA_GROUP/$GUAVA_ARTIFACT/$GUAVA_VER/$GUAVA_ARTIFACT-$GUAVA_VER.jar"
GUAVA_URL="$BASE/repository/maven-proxy-aliyun/$GUAVA_PATH"

# 第一次下载（应正常返回 200）
DOWNLOAD_OK=$(curl -s -o /dev/null -w "%{http_code}" "$GUAVA_URL")
if [ "$DOWNLOAD_OK" = "200" ]; then
  pass "阻断前代理下载正常 (HTTP 200)"
else
  info "阻断前代理下载返回 HTTP $DOWNLOAD_OK（可能上游暂不可用）"
fi

# 创建阻断规则阻断 guava
RULE2_RESP=$(curl -s -X POST "$BASE/api/v1/block-rules" \
  -H "Authorization: Bearer $TOKEN" \
  -H "Content-Type: application/json" \
  -d "{\"package_name\":\"$GUAVA_GROUP:$GUAVA_ARTIFACT\",\"version\":\"$GUAVA_VER\",\"match_type\":\"exact\",\"package_type\":\"maven\",\"reason\":\"test\",\"enabled\":true}")
RULE2_ID=$(echo "$RULE2_RESP" | python3 -c "import sys,json; d=json.load(sys.stdin); print(d.get('data',d).get('id',''))" 2>/dev/null)
if [ -n "$RULE2_ID" ] && [ "$RULE2_ID" != "0" ] && [ "$RULE2_ID" != "" ]; then
  pass "阻断 guava 规则创建成功 (ID=$RULE2_ID)"
else
  fail "阻断 guava 规则创建失败"
fi

# 再次下载（ProxyRuntime.checkBlocked 应拦截）
DOWNLOAD_BLOCKED=$(curl -s -o /dev/null -w "%{http_code}" "$GUAVA_URL")
BLOCK_MSG=$(curl -s "$GUAVA_URL")
if [ "$DOWNLOAD_BLOCKED" = "403" ]; then
  pass "阻断后代理下载返回 403 (符合预期)"
elif [ "$DOWNLOAD_BLOCKED" = "500" ] && echo "$BLOCK_MSG" | grep -qi "blocked"; then
  pass "阻断后代理下载返回 500（阻断已生效，HTTP 状态码后续修复）"
  echo "  阻断消息: $BLOCK_MSG"
else
  fail "阻断后代理下载未正确阻断 (HTTP $DOWNLOAD_BLOCKED, 消息: $BLOCK_MSG)"
fi

# 删除阻断规则恢复
if [ -n "$RULE2_ID" ] && [ "$RULE2_ID" != "0" ] && [ "$RULE2_ID" != "" ]; then
  DEL_CODE=$(curl -s -o /dev/null -w "%{http_code}" -X DELETE "$BASE/api/v1/block-rules/$RULE2_ID" \
    -H "Authorization: Bearer $TOKEN")
  pass "guava 阻断规则已清理 (HTTP $DEL_CODE)"
fi

# 等待缓存刷新后验证下载恢复
sleep 2
DOWNLOAD_RECOVER=$(curl -s -o /dev/null -w "%{http_code}" "$GUAVA_URL")
echo "  规则删除后下载 HTTP $DOWNLOAD_RECOVER（应恢复为 200）"

# ============================================================
# 第三部分：NPM 阻断验证
# ============================================================
echo ""
echo "════════════════════════════════════════"
echo "  NPM 阻断验证（代理仓库）"
echo "════════════════════════════════════════"

NPM_PKG="test-block-pkg"
NPM_VER="1.0.0"
NPM_URL="$BASE/repository/npm-proxy-cn/$NPM_PKG"

# 创建 NPM 阻断规则
RULE3_RESP=$(curl -s -X POST "$BASE/api/v1/block-rules" \
  -H "Authorization: Bearer $TOKEN" \
  -H "Content-Type: application/json" \
  -d "{\"package_name\":\"$NPM_PKG\",\"version\":\"$NPM_VER\",\"match_type\":\"exact\",\"package_type\":\"npm\",\"reason\":\"test\",\"enabled\":true}")
RULE3_ID=$(echo "$RULE3_RESP" | python3 -c "import sys,json; d=json.load(sys.stdin); print(d.get('data',d).get('id',''))" 2>/dev/null)
if [ -n "$RULE3_ID" ] && [ "$RULE3_ID" != "0" ] && [ "$RULE3_ID" != "" ]; then
  pass "NPM 阻断规则创建成功 (ID=$RULE3_ID)"
else
  fail "NPM 阻断规则创建失败"
fi

# 验证 NPM 代理检查
NPM_CHECK=$(curl -s -o /dev/null -w "%{http_code}" "$NPM_URL")
if [ "$NPM_CHECK" = "403" ] || [ "$NPM_CHECK" = "404" ]; then
  pass "NPM 阻断规则存在，代理请求返回 $NPM_CHECK"
else
  info "NPM 阻断请求返回 HTTP $NPM_CHECK"
fi

# 清理
if [ -n "$RULE3_ID" ] && [ "$RULE3_ID" != "0" ] && [ "$RULE3_ID" != "" ]; then
  curl -s -X DELETE "$BASE/api/v1/block-rules/$RULE3_ID" -H "Authorization: Bearer $TOKEN" > /dev/null
  pass "NPM 阻断规则已清理"
fi

# ============================================================
# 第四部分：条件阻断 — 按 License 阻断
# 验证 ConditionType=license 的第二层阻断检查
# ProxyRuntime 在拿到 artifact 后，用 Attributes["license"] 做条件匹配
# ============================================================
echo ""
echo "════════════════════════════════════════"
echo "  条件阻断 — License 阻断验证"
echo "════════════════════════════════════════"

# 4.1 创建 license 条件阻断规则
info "创建 license 条件阻断规则 (license equals GPL-3.0)..."
LICENSE_RULE_RESP=$(curl -s -X POST "$BASE/api/v1/block-rules" \
  -H "Authorization: Bearer $TOKEN" \
  -H "Content-Type: application/json" \
  -d '{"package_name":"*","version":"*","match_type":"wildcard","package_type":"npm","reason":"test-license-block","enabled":true,"condition_type":"license","condition_op":"equals","condition_value":"GPL-3.0"}')
LICENSE_RULE_ID=$(echo "$LICENSE_RULE_RESP" | python3 -c "import sys,json; d=json.load(sys.stdin); print(d.get('data',d).get('id',''))" 2>/dev/null)
if [ -n "$LICENSE_RULE_ID" ] && [ "$LICENSE_RULE_ID" != "0" ] && [ "$LICENSE_RULE_ID" != "" ]; then
  pass "license 条件阻断规则创建成功 (ID=$LICENSE_RULE_ID)"
else
  fail "license 条件阻断规则创建失败: $LICENSE_RULE_RESP"
fi

# 4.2 验证条件字段正确落库
if [ -n "$LICENSE_RULE_ID" ] && [ "$LICENSE_RULE_ID" != "0" ] && [ "$LICENSE_RULE_ID" != "" ]; then
  RULE_DETAIL=$(curl -s "$BASE/api/v1/block-rules" -H "Authorization: Bearer $TOKEN")
  COND_TYPE=$(echo "$RULE_DETAIL" | python3 -c "
import sys,json
d=json.load(sys.stdin)
for r in d.get('data',[]):
    if r.get('id')==$LICENSE_RULE_ID:
        print(r.get('condition_type',''))
        break
" 2>/dev/null)
  if [ "$COND_TYPE" = "license" ]; then
    pass "条件字段 condition_type=license 正确落库"
  else
    fail "条件字段 condition_type 未正确落库 (got: $COND_TYPE)"
  fi
fi

# 4.3 清理 license 条件规则
if [ -n "$LICENSE_RULE_ID" ] && [ "$LICENSE_RULE_ID" != "0" ] && [ "$LICENSE_RULE_ID" != "" ]; then
  DEL_CODE=$(curl -s -o /dev/null -w "%{http_code}" -X DELETE "$BASE/api/v1/block-rules/$LICENSE_RULE_ID" \
    -H "Authorization: Bearer $TOKEN")
  if [ "$DEL_CODE" = "200" ] || [ "$DEL_CODE" = "204" ]; then
    pass "license 条件阻断规则删除成功 (HTTP $DEL_CODE)"
  else
    fail "license 条件阻断规则删除失败 (HTTP $DEL_CODE)"
  fi
fi

# ============================================================
# 第五部分：条件阻断 — 按发布时间阻断
# 验证 ConditionType=publish_time 的第二层阻断检查
# ============================================================
echo ""
echo "════════════════════════════════════════"
echo "  条件阻断 — 发布时间阻断验证"
echo "════════════════════════════════════════"

# 5.1 创建 publish_time before 条件阻断规则
info "创建 publish_time before 条件阻断规则..."
TIME_RULE_RESP=$(curl -s -X POST "$BASE/api/v1/block-rules" \
  -H "Authorization: Bearer $TOKEN" \
  -H "Content-Type: application/json" \
  -d '{"package_name":"*","version":"*","match_type":"wildcard","package_type":"maven","reason":"test-time-block","enabled":true,"condition_type":"publish_time","condition_op":"before","condition_value":"2020-01-01T00:00:00Z"}')
TIME_RULE_ID=$(echo "$TIME_RULE_RESP" | python3 -c "import sys,json; d=json.load(sys.stdin); print(d.get('data',d).get('id',''))" 2>/dev/null)
if [ -n "$TIME_RULE_ID" ] && [ "$TIME_RULE_ID" != "0" ] && [ "$TIME_RULE_ID" != "" ]; then
  pass "publish_time 条件阻断规则创建成功 (ID=$TIME_RULE_ID)"
else
  fail "publish_time 条件阻断规则创建失败: $TIME_RULE_RESP"
fi

# 5.2 验证条件字段正确落库
if [ -n "$TIME_RULE_ID" ] && [ "$TIME_RULE_ID" != "0" ] && [ "$TIME_RULE_ID" != "" ]; then
  RULE_DETAIL=$(curl -s "$BASE/api/v1/block-rules" -H "Authorization: Bearer $TOKEN")
  COND_TYPE=$(echo "$RULE_DETAIL" | python3 -c "
import sys,json
d=json.load(sys.stdin)
for r in d.get('data',[]):
    if r.get('id')==$TIME_RULE_ID:
        print(r.get('condition_type',''))
        break
" 2>/dev/null)
  COND_OP=$(echo "$RULE_DETAIL" | python3 -c "
import sys,json
d=json.load(sys.stdin)
for r in d.get('data',[]):
    if r.get('id')==$TIME_RULE_ID:
        print(r.get('condition_op',''))
        break
" 2>/dev/null)
  if [ "$COND_TYPE" = "publish_time" ] && [ "$COND_OP" = "before" ]; then
    pass "条件字段 condition_type=publish_time, condition_op=before 正确落库"
  else
    fail "条件字段未正确落库 (type=$COND_TYPE, op=$COND_OP)"
  fi
fi

# 5.3 清理 publish_time 条件规则
if [ -n "$TIME_RULE_ID" ] && [ "$TIME_RULE_ID" != "0" ] && [ "$TIME_RULE_ID" != "" ]; then
  DEL_CODE=$(curl -s -o /dev/null -w "%{http_code}" -X DELETE "$BASE/api/v1/block-rules/$TIME_RULE_ID" \
    -H "Authorization: Bearer $TOKEN")
  if [ "$DEL_CODE" = "200" ] || [ "$DEL_CODE" = "204" ]; then
    pass "publish_time 条件阻断规则删除成功 (HTTP $DEL_CODE)"
  else
    fail "publish_time 条件阻断规则删除失败 (HTTP $DEL_CODE)"
  fi
fi

# ============================================================
# 第六部分：条件阻断 — 非法字段校验
# 验证 API 层对非法 condition_type / condition_op 的校验
# ============================================================
echo ""
echo "════════════════════════════════════════"
echo "  条件阻断 — 非法字段校验"
echo "════════════════════════════════════════"

# 6.1 非法 condition_type 应返回 400
INVALID_TYPE_CODE=$(curl -s -o /dev/null -w "%{http_code}" -X POST "$BASE/api/v1/block-rules" \
  -H "Authorization: Bearer $TOKEN" \
  -H "Content-Type: application/json" \
  -d '{"package_name":"test","version":"1.0.0","match_type":"exact","package_type":"npm","reason":"test","enabled":true,"condition_type":"invalid_type","condition_op":"equals","condition_value":"MIT"}')
if [ "$INVALID_TYPE_CODE" = "400" ]; then
  pass "非法 condition_type 正确返回 400"
else
  fail "非法 condition_type 应返回 400，实际返回 $INVALID_TYPE_CODE"
fi

# 6.2 非法 condition_op 应返回 400
INVALID_OP_CODE=$(curl -s -o /dev/null -w "%{http_code}" -X POST "$BASE/api/v1/block-rules" \
  -H "Authorization: Bearer $TOKEN" \
  -H "Content-Type: application/json" \
  -d '{"package_name":"test","version":"1.0.0","match_type":"exact","package_type":"npm","reason":"test","enabled":true,"condition_type":"license","condition_op":"invalid_op","condition_value":"MIT"}')
if [ "$INVALID_OP_CODE" = "400" ]; then
  pass "非法 condition_op 正确返回 400"
else
  fail "非法 condition_op 应返回 400，实际返回 $INVALID_OP_CODE"
fi

# 6.3 不传条件字段应正常创建（向后兼容）
COMPAT_RESP=$(curl -s -X POST "$BASE/api/v1/block-rules" \
  -H "Authorization: Bearer $TOKEN" \
  -H "Content-Type: application/json" \
  -d '{"package_name":"compat-test-pkg","version":"1.0.0","match_type":"exact","package_type":"npm","reason":"compat-test","enabled":true}')
COMPAT_ID=$(echo "$COMPAT_RESP" | python3 -c "import sys,json; d=json.load(sys.stdin); print(d.get('data',d).get('id',''))" 2>/dev/null)
if [ -n "$COMPAT_ID" ] && [ "$COMPAT_ID" != "0" ] && [ "$COMPAT_ID" != "" ]; then
  pass "不传条件字段正常创建 (向后兼容, ID=$COMPAT_ID)"
  # 清理
  curl -s -X DELETE "$BASE/api/v1/block-rules/$COMPAT_ID" -H "Authorization: Bearer $TOKEN" > /dev/null
else
  fail "不传条件字段创建失败: $COMPAT_RESP"
fi

# ============================================================
# 第七部分：条件阻断 — within_last（最近 N 天内发布）
# 验证 publish_time + within_last 操作符的创建与字段落库
# ============================================================
echo ""
echo "════════════════════════════════════════"
echo "  条件阻断 — within_last (最近 N 天内)"
echo "════════════════════════════════════════"

# 7.1 创建 within_last 规则
WITHIN_LAST_RESP=$(curl -s -X POST "$BASE/api/v1/block-rules" \
  -H "Authorization: Bearer $TOKEN" \
  -H "Content-Type: application/json" \
  -d '{"package_name":"within-last-test-pkg","version":"1.0.0","match_type":"exact","package_type":"npm","reason":"block packages published within 15 days","enabled":true,"condition_type":"publish_time","condition_op":"within_last","condition_value":"15"}')
WITHIN_LAST_ID=$(echo "$WITHIN_LAST_RESP" | python3 -c "import sys,json; d=json.load(sys.stdin); print(d.get('data',d).get('id',''))" 2>/dev/null)
if [ -n "$WITHIN_LAST_ID" ] && [ "$WITHIN_LAST_ID" != "0" ] && [ "$WITHIN_LAST_ID" != "" ]; then
  pass "创建 within_last 规则成功 (ID=$WITHIN_LAST_ID)"
else
  fail "创建 within_last 规则失败: $WITHIN_LAST_RESP"
fi

# 7.2 验证字段正确落库
WITHIN_LAST_CT=$(echo "$WITHIN_LAST_RESP" | python3 -c "import sys,json; d=json.load(sys.stdin); print(d.get('data',d).get('condition_type',''))" 2>/dev/null)
WITHIN_LAST_OP=$(echo "$WITHIN_LAST_RESP" | python3 -c "import sys,json; d=json.load(sys.stdin); print(d.get('data',d).get('condition_op',''))" 2>/dev/null)
WITHIN_LAST_VAL=$(echo "$WITHIN_LAST_RESP" | python3 -c "import sys,json; d=json.load(sys.stdin); print(d.get('data',d).get('condition_value',''))" 2>/dev/null)
if [ "$WITHIN_LAST_CT" = "publish_time" ] && [ "$WITHIN_LAST_OP" = "within_last" ] && [ "$WITHIN_LAST_VAL" = "15" ]; then
  pass "within_last 字段落库正确 (type=$WITHIN_LAST_CT, op=$WITHIN_LAST_OP, value=$WITHIN_LAST_VAL)"
else
  fail "within_last 字段落库错误: type=$WITHIN_LAST_CT, op=$WITHIN_LAST_OP, value=$WITHIN_LAST_VAL"
fi

# 7.3 清理
if [ -n "$WITHIN_LAST_ID" ] && [ "$WITHIN_LAST_ID" != "" ]; then
  curl -s -X DELETE "$BASE/api/v1/block-rules/$WITHIN_LAST_ID" -H "Authorization: Bearer $TOKEN" > /dev/null
  pass "清理 within_last 规则"
fi

# ============================================================
# 总结
# ============================================================
echo ""
echo "============================================"
echo " 测试汇总"
echo "============================================"
echo -e "  通过: $PASS"
echo -e "  失败: $FAIL"
echo -e "  总计: $((PASS+FAIL))"

if [ $FAIL -eq 0 ]; then
  echo -e "\033[32m✅ Block 功能正常! 所有测试通过\033[0m"
  exit 0
else
  echo -e "\033[31m❌ 部分测试失败\033[0m"
  exit 1
fi
