#!/bin/bash
# Moonlight Registry E2E — 仓库部署策略（allow_overwrite / allow_delete）回归测试
# 用法: ./scripts/e2e/test_policy_flags.sh
#
# 在隔离实例上验证（不触碰现有 data/ 与 9081 端口）：
#   1. allow_overwrite=false 时重复上传同一内容 artifact → 409
#   2. 聚合 metadata 重写豁免（npm 全 packument 发布新版本、maven-metadata.xml）→ 201
#   3. allow_delete=false 时协议 DELETE 已存在制品 → 403；删除不存在制品 → 404（404 优先）
#   4. npm dist-tag add/remove 在 allow_overwrite=false 下仍正常（metadata 豁免）
#
# 退出码: 0 = 全部通过, 非0 = 失败数量
# 环境变量: PORT（默认 19082）

set -uo pipefail

SCRIPT_DIR="$(cd "$(dirname "$0")/../.." && pwd)"
cd "$SCRIPT_DIR"

PORT="${PORT:-19082}"
BASE_URL="http://localhost:${PORT}"
WORK_DIR="$(mktemp -d /tmp/mlbox-policy-e2e.XXXXXX)"
SERVER_PID=""

FAIL_COUNT=0
PASS_COUNT=0
TOTAL=0

GREEN='\033[0;32m'
RED='\033[0;31m'
NC='\033[0m'

pass() { echo -e "  ${GREEN}PASS${NC} $1"; }
fail() { FAIL_COUNT=$((FAIL_COUNT + 1)); echo -e "  ${RED}FAIL${NC} $1"; }
section() { echo ""; echo "========================================"; echo "$1"; echo "========================================"; }
assert_code() { # desc expected actual body
    TOTAL=$((TOTAL + 1))
    if [ "$2" = "$3" ]; then
        PASS_COUNT=$((PASS_COUNT + 1)); pass "$1 (HTTP $3)"
    else
        fail "$1 — 期望 $2 实际 $3 body='$4'"
    fi
}

cleanup() {
    if [ -n "$SERVER_PID" ] && kill -0 "$SERVER_PID" 2>/dev/null; then
        kill "$SERVER_PID" 2>/dev/null
        wait "$SERVER_PID" 2>/dev/null
    fi
    rm -rf "$WORK_DIR"
}
trap cleanup EXIT

# ── 构建 + 启动隔离实例 ──
section "构建并启动隔离实例 (port $PORT)"
echo "工作目录: $WORK_DIR"
go build -o "$WORK_DIR/moonlight-box" ./cmd/registry || { echo -e "${RED}构建失败${NC}"; exit 1; }

cat > "$WORK_DIR/config.yaml" <<EOF
server:
  port: ${PORT}
database:
  dsn: ${WORK_DIR}/registry.db
auth:
  jwt_secret: e2e-policy-test-secret-please-change
logging:
  level: warn
seed:
  load_test_data: false
EOF

"$WORK_DIR/moonlight-box" -config "$WORK_DIR/config.yaml" > "$WORK_DIR/server.log" 2>&1 &
SERVER_PID=$!

for i in $(seq 1 30); do
    if curl -sf "$BASE_URL/api/v1/auth/login" > /dev/null 2>&1; then break; fi
    sleep 1
    if [ "$i" = 30 ]; then
        echo -e "${RED}服务未在 $BASE_URL 就绪，日志尾部：${NC}"; tail -5 "$WORK_DIR/server.log"; exit 1
    fi
done

TOKEN=$(curl -s -X POST "$BASE_URL/api/v1/auth/login" \
    -H "Content-Type: application/json" \
    -d '{"username":"admin","password":"admin123"}' \
    | python3 -c "import json,sys; print(json.load(sys.stdin)['data']['access_token'])" 2>/dev/null)
if [ -z "$TOKEN" ]; then echo -e "${RED}登录失败${NC}"; exit 1; fi

AUTH=(-H "Authorization: Bearer $TOKEN" -H "Content-Type: application/json")
H=(-H "Authorization: Bearer $TOKEN")

# ── TEST A: maven overwrite / metadata 豁免 / delete ──
section "TEST A: maven 覆盖守卫 + 聚合 metadata 豁免 + 删除语义"
curl -s -X POST "$BASE_URL/api/v1/repositories" "${AUTH[@]}" \
    -d '{"name":"pol-mvn","display_name":"pol-mvn","type":"local","package_type":"maven","allow_overwrite":false,"allow_delete":false,"enabled":true}' > /dev/null

JAR="$BASE_URL/repository/pol-mvn/com/example/app/1.0.0/app-1.0.0.jar"
META="$BASE_URL/repository/pol-mvn/com/example/app/maven-metadata.xml"
NEWJAR="$BASE_URL/repository/pol-mvn/com/example/app/1.0.1/app-1.0.1.jar"

code=$(curl -s -o /dev/null -w "%{http_code}" -X PUT "$JAR" "${H[@]}" -H 'Content-Type: application/octet-stream' --data-binary 'old')
assert_code "首次上传 jar" 201 "$code" ""
code=$(curl -s -o /dev/null -w "%{http_code}" -X PUT "$JAR" "${H[@]}" -H 'Content-Type: application/octet-stream' --data-binary 'new')
assert_code "allow_overwrite=false 重复上传同一 jar → 409" 409 "$code" ""
code=$(curl -s -o /dev/null -w "%{http_code}" -X PUT "$META" "${H[@]}" -H 'Content-Type: application/xml' --data-binary '<metadata><v>1.0.0</v></metadata>')
assert_code "首次上传 maven-metadata.xml" 201 "$code" ""
code=$(curl -s -o /dev/null -w "%{http_code}" -X PUT "$META" "${H[@]}" -H 'Content-Type: application/xml' --data-binary '<metadata><v>1.0.1</v></metadata>')
assert_code "maven-metadata.xml 重写豁免（maven 更新语义返回 200，不再 409）" 200 "$code" ""
code=$(curl -s -o /dev/null -w "%{http_code}" -X PUT "$NEWJAR" "${H[@]}" -H 'Content-Type: application/octet-stream' --data-binary 'v101')
assert_code "部署新版本 jar（新身份）→ 201" 201 "$code" ""

curl -s -X PUT "$BASE_URL/api/v1/repositories/pol-mvn" "${AUTH[@]}" -d '{"allow_overwrite":true}' > /dev/null
code=$(curl -s -o /dev/null -w "%{http_code}" -X PUT "$JAR" "${H[@]}" -H 'Content-Type: application/octet-stream' --data-binary 'new')
assert_code "开启 allow_overwrite 后重传同一 jar → 200" 200 "$code" ""

# 删除语义（allow_delete 仍为 false）
code=$(curl -s -o /dev/null -w "%{http_code}" -X DELETE "$JAR" "${H[@]}")
assert_code "allow_delete=false 删除已存在 jar → 403" 403 "$code" ""
code=$(curl -s -o /dev/null -w "%{http_code}" -X DELETE "$BASE_URL/repository/pol-mvn/com/example/app/9.9.9/missing.jar" "${H[@]}")
assert_code "allow_delete=false 删除不存在制品 → 404（404 优先）" 404 "$code" ""
curl -s -X PUT "$BASE_URL/api/v1/repositories/pol-mvn" "${AUTH[@]}" -d '{"allow_delete":true}' > /dev/null
code=$(curl -s -o /dev/null -w "%{http_code}" -X DELETE "$JAR" "${H[@]}")
assert_code "开启 allow_delete 后删除 jar → 204" 204 "$code" ""
code=$(curl -s -o /dev/null -w "%{http_code}" "$JAR" "${H[@]}")
assert_code "删除后 GET jar → 404" 404 "$code" ""

# ── TEST B: npm 覆盖守卫 + 新版本发布 + dist-tag 豁免 ──
section "TEST B: npm 覆盖守卫 + 全 packument 新版本 + dist-tag 豁免"
curl -s -X POST "$BASE_URL/api/v1/repositories" "${AUTH[@]}" \
    -d '{"name":"pol-npm","display_name":"pol-npm","type":"local","package_type":"npm","allow_overwrite":false,"allow_delete":false,"enabled":true}' > /dev/null

NPMPUT="$BASE_URL/repository/pol-npm/pkg"
# 发布 1.0.0（单版本）
code=$(curl -s -o /dev/null -w "%{http_code}" -X PUT "$NPMPUT" "${H[@]}" \
    -d '{"name":"pkg","version":"1.0.0","_id":"pkg","versions":{"1.0.0":{"name":"pkg","version":"1.0.0"}},"dist-tags":{"latest":"1.0.0"},"_attachments":{"pkg-1.0.0.tgz":{"content_type":"application/octet-stream","data":"aGVsbG8="}}}')
assert_code "npm 发布 pkg@1.0.0" 201 "$code" ""
# 全 packument 发布 2.0.0（含已有 1.0.0）→ 回归 #1：加新版本不应被 409
code=$(curl -s -o /dev/null -w "%{http_code}" -X PUT "$NPMPUT" "${H[@]}" \
    -d '{"name":"pkg","version":"2.0.0","_id":"pkg","versions":{"1.0.0":{"name":"pkg","version":"1.0.0"},"2.0.0":{"name":"pkg","version":"2.0.0"}},"dist-tags":{"latest":"2.0.0"},"_attachments":{"pkg-2.0.0.tgz":{"content_type":"application/octet-stream","data":"d29ybGQ="}}}')
assert_code "全 packument 发布新版本 2.0.0 → 201（metadata 豁免）" 201 "$code" ""
# 重复发布同一版本 → 409
code=$(curl -s -o /dev/null -w "%{http_code}" -X PUT "$NPMPUT" "${H[@]}" \
    -d '{"name":"pkg","version":"1.0.0","_id":"pkg","versions":{"1.0.0":{"name":"pkg","version":"1.0.0"}},"dist-tags":{"latest":"1.0.0"},"_attachments":{"pkg-1.0.0.tgz":{"content_type":"application/octet-stream","data":"aGVsbG8="}}}')
assert_code "allow_overwrite=false 重复发布同一版本 → 409" 409 "$code" ""
# dist-tag add/remove（metadata 重写豁免）
code=$(curl -s -o /dev/null -w "%{http_code}" -X PUT "$BASE_URL/repository/pol-npm/-/package/pkg/dist-tags/beta" "${H[@]}" -d '1.0.0')
assert_code "dist-tag add 豁免 → 201" 201 "$code" ""
code=$(curl -s -o /dev/null -w "%{http_code}" -X DELETE "$BASE_URL/repository/pol-npm/-/package/pkg/dist-tags/beta" "${H[@]}")
assert_code "dist-tag remove 豁免 → 200" 200 "$code" ""

# ── 汇总 ──
section "汇总"
echo "通过: $PASS_COUNT / $TOTAL"
if [ "$FAIL_COUNT" -gt 0 ]; then
    echo -e "${RED}失败: $FAIL_COUNT${NC}"
    echo "服务日志: $WORK_DIR/server.log"
    exit "$FAIL_COUNT"
else
    echo -e "${GREEN}全部通过${NC}"
    exit 0
fi
