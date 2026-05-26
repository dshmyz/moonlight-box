#!/bin/bash
# ============================================================
# 虚拟仓库成员配置测试
# 验证虚拟仓库的成员添加、查询、删除功能
# ============================================================

API_BASE="http://localhost:9081/api/v1"
REPO_BASE="http://localhost:9081/repository"
PASS=0
FAIL=0

pass() { PASS=$((PASS+1)); echo -e "  \033[32m✓ PASS\033[0m $1"; }
fail() { FAIL=$((FAIL+1)); echo -e "  \033[31m✗ FAIL\033[0m $1"; }
info() { echo -e "  \033[36mℹ INFO\033[0m $1"; }

RANDOM_SUFFIX=$(date +%s | tail -c 5)
LOCAL_REPO="npm-local-mem-${RANDOM_SUFFIX}"
PROXY_REPO="npm-proxy-mem-${RANDOM_SUFFIX}"
VIRTUAL_REPO="npm-virtual-mem-${RANDOM_SUFFIX}"

echo "=== 虚拟仓库成员配置测试 ==="
echo ""

# 获取 TOKEN
RESP=$(curl -s "$API_BASE/auth/login" -H "Content-Type: application/json" \
    -d '{"username":"admin","password":"admin123"}')
TOKEN=$(echo "$RESP" | python3 -c "import sys,json; print(json.load(sys.stdin)['data']['access_token'])" 2>/dev/null)
if [ -z "$TOKEN" ]; then fail "登录失败"; exit 1; fi
pass "认证成功"

echo ""
echo "1. 创建本地仓库"
HTTP_CODE=$(curl -s -o /tmp/vr_create_local.json -w "%{http_code}" -X POST "${API_BASE}/repositories" \
  -H "Content-Type: application/json" \
  -H "Authorization: Bearer ${TOKEN}" \
  -d "{
    \"name\": \"${LOCAL_REPO}\",
    \"type\": \"local\",
    \"package_type\": \"npm\",
    \"enabled\": true
  }")
if [ "$HTTP_CODE" = "200" ] || [ "$HTTP_CODE" = "201" ]; then
  pass "本地仓库创建成功"
  cat /tmp/vr_create_local.json | python3 -m json.tool 2>/dev/null || true
else
  fail "本地仓库创建失败 (HTTP $HTTP_CODE)"
  cat /tmp/vr_create_local.json 2>/dev/null || true
fi

echo ""
echo "2. 创建代理仓库"
HTTP_CODE=$(curl -s -o /tmp/vr_create_proxy.json -w "%{http_code}" -X POST "${API_BASE}/repositories" \
  -H "Content-Type: application/json" \
  -H "Authorization: Bearer ${TOKEN}" \
  -d "{
    \"name\": \"${PROXY_REPO}\",
    \"type\": \"proxy\",
    \"package_type\": \"npm\",
    \"remote_url\": \"https://registry.npmjs.org\",
    \"enabled\": true
  }")
if [ "$HTTP_CODE" = "200" ] || [ "$HTTP_CODE" = "201" ]; then
  pass "代理仓库创建成功"
  cat /tmp/vr_create_proxy.json | python3 -m json.tool 2>/dev/null || true
else
  fail "代理仓库创建失败 (HTTP $HTTP_CODE)"
  cat /tmp/vr_create_proxy.json 2>/dev/null || true
fi

echo ""
echo "3. 创建虚拟仓库（不带成员）"
HTTP_CODE=$(curl -s -o /tmp/vr_create_virtual.json -w "%{http_code}" -X POST "${API_BASE}/repositories" \
  -H "Content-Type: application/json" \
  -H "Authorization: Bearer ${TOKEN}" \
  -d "{
    \"name\": \"${VIRTUAL_REPO}\",
    \"type\": \"virtual\",
    \"package_type\": \"npm\",
    \"enabled\": true
  }")
if [ "$HTTP_CODE" = "200" ] || [ "$HTTP_CODE" = "201" ]; then
  pass "虚拟仓库创建成功"
  cat /tmp/vr_create_virtual.json | python3 -m json.tool 2>/dev/null || true
else
  fail "虚拟仓库创建失败 (HTTP $HTTP_CODE)"
  cat /tmp/vr_create_virtual.json 2>/dev/null || true
fi

echo ""
echo "4. 向虚拟仓库添加成员（优先级 0）"
HTTP_CODE=$(curl -s -o /tmp/vr_add_member1.json -w "%{http_code}" -X POST "${API_BASE}/repositories/${VIRTUAL_REPO}/members" \
  -H "Content-Type: application/json" \
  -H "Authorization: Bearer ${TOKEN}" \
  -d "{
    \"member_name\": \"${LOCAL_REPO}\",
    \"priority\": 0
  }")
if [ "$HTTP_CODE" = "200" ] || [ "$HTTP_CODE" = "201" ]; then
  pass "添加成员 ${LOCAL_REPO} 成功"
  cat /tmp/vr_add_member1.json | python3 -m json.tool 2>/dev/null || true
else
  fail "添加成员失败 (HTTP $HTTP_CODE)"
  cat /tmp/vr_add_member1.json 2>/dev/null || true
fi

echo ""
echo "5. 向虚拟仓库添加成员（优先级 1）"
HTTP_CODE=$(curl -s -o /tmp/vr_add_member2.json -w "%{http_code}" -X POST "${API_BASE}/repositories/${VIRTUAL_REPO}/members" \
  -H "Content-Type: application/json" \
  -H "Authorization: Bearer ${TOKEN}" \
  -d "{
    \"member_name\": \"${PROXY_REPO}\",
    \"priority\": 1
  }")
if [ "$HTTP_CODE" = "200" ] || [ "$HTTP_CODE" = "201" ]; then
  pass "添加成员 ${PROXY_REPO} 成功"
  cat /tmp/vr_add_member2.json | python3 -m json.tool 2>/dev/null || true
else
  fail "添加成员失败 (HTTP $HTTP_CODE)"
  cat /tmp/vr_add_member2.json 2>/dev/null || true
fi

echo ""
echo "6. 获取虚拟仓库成员列表"
HTTP_CODE=$(curl -s -o /tmp/vr_member_list.json -w "%{http_code}" -X GET "${API_BASE}/repositories/${VIRTUAL_REPO}/members" \
  -H "Authorization: Bearer ${TOKEN}")
if [ "$HTTP_CODE" = "200" ]; then
  pass "成员列表获取成功"
  cat /tmp/vr_member_list.json | python3 -m json.tool 2>/dev/null || true
else
  fail "成员列表获取失败 (HTTP $HTTP_CODE)"
  cat /tmp/vr_member_list.json 2>/dev/null || true
fi

echo ""
echo "7. 移除成员仓库"
HTTP_CODE=$(curl -s -o /tmp/vr_remove_member.json -w "%{http_code}" -X DELETE "${API_BASE}/repositories/${VIRTUAL_REPO}/members/${PROXY_REPO}" \
  -H "Authorization: Bearer ${TOKEN}")
if [ "$HTTP_CODE" = "200" ] || [ "$HTTP_CODE" = "204" ]; then
  pass "成员移除成功 (HTTP $HTTP_CODE)"
else
  fail "成员移除失败 (HTTP $HTTP_CODE)"
  cat /tmp/vr_remove_member.json 2>/dev/null || true
fi

echo ""
echo "8. 再次查看成员列表"
HTTP_CODE=$(curl -s -o /tmp/vr_member_list2.json -w "%{http_code}" -X GET "${API_BASE}/repositories/${VIRTUAL_REPO}/members" \
  -H "Authorization: Bearer ${TOKEN}")
if [ "$HTTP_CODE" = "200" ]; then
  MEMBER_COUNT=$(python3 -c "
import sys,json
d=json.load(sys.stdin)
data=d.get('data',[])
if isinstance(data,list): print(len(data))
else: print(0)
" < /tmp/vr_member_list2.json 2>/dev/null || echo "0")
  pass "成员列表获取成功，当前 $MEMBER_COUNT 个成员"
  cat /tmp/vr_member_list2.json | python3 -m json.tool 2>/dev/null || true
else
  fail "成员列表获取失败 (HTTP $HTTP_CODE)"
fi

echo ""
echo "=== 清理 ==="
# 删除仓库
curl -s -X DELETE "${API_BASE}/repositories/${VIRTUAL_REPO}" \
  -H "Authorization: Bearer ${TOKEN}" > /dev/null 2>&1 && pass "虚拟仓库已清理" || true
curl -s -X DELETE "${API_BASE}/repositories/${LOCAL_REPO}" \
  -H "Authorization: Bearer ${TOKEN}" > /dev/null 2>&1 && pass "本地仓库已清理" || true
curl -s -X DELETE "${API_BASE}/repositories/${PROXY_REPO}" \
  -H "Authorization: Bearer ${TOKEN}" > /dev/null 2>&1 && pass "代理仓库已清理" || true

echo ""
echo "=== 测试完成 ==="
echo -e "  通过: $PASS"
echo -e "  失败: $FAIL"
[ $FAIL -eq 0 ] && exit 0 || exit 1
