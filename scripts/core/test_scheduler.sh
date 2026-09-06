#!/bin/bash
# Moonlight Registry E2E — 定时任务（TaskScheduler）API 确定性验证
# 用法: ./scripts/core/test_scheduler.sh
#
# 隔离实例验证（不触碰现有 data/ 与 9081 端口）：
#   1. 任务列表默认 @every <全局间隔> + 各任务 config 字段
#   2. 非法调度（@every 0s / 负间隔 / 非法 cron）→ 400
#   3. 设置 cron + 配置持久化 → custom=true、cron 原样、keep_last 回读
#   4. 全局间隔热更新 → 非 custom 任务同步变，custom 任务保留 cron
#   5. run-now → 200；旧 log 清理接口仍可用；snapshot 清理接口已移除
# 注：cron"真实到点触发"由 Go 单测 TestTaskSchedulerCronScheduleRuns 覆盖，
#     时序断言不适合 e2e（慢且 flaky）。
# 退出码: 0 = 全部通过, 非0 = 失败数量

set -uo pipefail

SCRIPT_DIR="$(cd "$(dirname "$0")/../.." && pwd)"
cd "$SCRIPT_DIR"

PORT="${PORT:-19083}"
BASE_URL="http://localhost:${PORT}"
WORK_DIR="$(mktemp -d /tmp/mlbox-sched-e2e.XXXXXX)"
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
# 取任务列表中某字段（用 python 解析，避免 jq 依赖）。$1 为引用 t 的 python 表达式。
task_field() { # pyexpr task_name
    python3 -c "
import json,sys
d = json.load(sys.stdin)['data']['tasks']
t = next(x for x in d if x['name']=='$2')
print($1)
" 2>/dev/null
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
go build -o "$WORK_DIR/moonlight-box" ./cmd/registry || { echo -e "${RED}构建失败${NC}"; exit 1; }
cat > "$WORK_DIR/config.yaml" <<EOF
server:
  port: ${PORT}
database:
  dsn: ${WORK_DIR}/registry.db
auth:
  jwt_secret: sched-e2e-secret-please-change
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
    if [ "$i" = 30 ]; then echo -e "${RED}服务未就绪${NC}"; tail -5 "$WORK_DIR/server.log"; exit 1; fi
done

TOKEN=$(curl -s -X POST "$BASE_URL/api/v1/auth/login" -H 'Content-Type: application/json' \
    -d '{"username":"admin","password":"admin123"}' \
    | python3 -c "import json,sys; print(json.load(sys.stdin)['data']['access_token'])" 2>/dev/null)
if [ -z "$TOKEN" ]; then echo -e "${RED}登录失败${NC}"; exit 1; fi
AUTH=(-H "Authorization: Bearer $TOKEN" -H "Content-Type: application/json")

# ── TEST 1: 默认任务列表 ──
section "TEST 1: 任务列表默认调度 + config 字段"
LIST=$(curl -s "$BASE_URL/api/v1/scheduler/tasks" "${AUTH[@]}")
COUNT=$(echo "$LIST" | python3 -c "import json,sys; print(len(json.load(sys.stdin)['data']['tasks']))")
assert_code "任务数为 3" 3 "$COUNT" "$LIST"
for name in maven_snapshot proxy_metadata_cache_gc log_cleanup; do
    SCH=$(echo "$LIST" | task_field "t['schedule']" "$name")
    if [[ "$SCH" == @every* ]]; then
        TOTAL=$((TOTAL+1)); PASS_COUNT=$((PASS_COUNT+1)); pass "$name 默认调度 $SCH"
    else
        fail "$name 调度=$SCH 应 @every 前缀"
    fi
    CFG=$(echo "$LIST" | task_field "len(t.get('config',[]))" "$name")
    if [ "$CFG" -ge 1 ] 2>/dev/null; then
        TOTAL=$((TOTAL+1)); PASS_COUNT=$((PASS_COUNT+1)); pass "$name 有 config 字段"
    else
        fail "$name 无 config 字段"
    fi
done

# ── TEST 2: 非法调度拒绝 ──
section "TEST 2: 非法调度（@every 0s / 负间隔 / 非法 cron）→ 400"
for bad in "@every 0s" "@every -5s" "not-a-cron"; do
    code=$(curl -s -o /dev/null -w "%{http_code}" -X PUT "$BASE_URL/api/v1/scheduler/tasks/maven_snapshot" "${AUTH[@]}" -d "{\"schedule\":\"$bad\"}")
    assert_code "拒绝调度 '$bad'" 400 "$code" ""
done

# ── TEST 3: 设置 cron + 配置持久化 ──
section "TEST 3: cron + 配置持久化"
code=$(curl -s -o /dev/null -w "%{http_code}" -X PUT "$BASE_URL/api/v1/scheduler/tasks/maven_snapshot" "${AUTH[@]}" \
    -d '{"schedule":"0 3 * * 1","config":{"enabled":true,"keep_last":3,"max_age_days":60}}')
assert_code "设置 cron + 配置" 200 "$code" ""
LIST=$(curl -s "$BASE_URL/api/v1/scheduler/tasks" "${AUTH[@]}")
SCH=$(echo "$LIST" | task_field "t['schedule']" maven_snapshot)
if [ "$SCH" = "0 3 * * 1" ]; then
    TOTAL=$((TOTAL+1)); PASS_COUNT=$((PASS_COUNT+1)); pass "cron 原样回读"
else
    fail "cron 回读=$SCH"
fi
CUSTOM=$(echo "$LIST" | task_field "t['custom']" maven_snapshot)
if [ "$CUSTOM" = "True" ]; then
    TOTAL=$((TOTAL+1)); PASS_COUNT=$((PASS_COUNT+1)); pass "custom=true"
else
    fail "custom=$CUSTOM"
fi
KEEP=$(echo "$LIST" | task_field "next(f['value'] for f in t['config'] if f['key']=='keep_last')" maven_snapshot)
if [ "$KEEP" = "3" ]; then
    TOTAL=$((TOTAL+1)); PASS_COUNT=$((PASS_COUNT+1)); pass "keep_last 持久化=3"
else
    fail "keep_last=$KEEP"
fi

# ── TEST 4: 全局间隔热更新 ──
section "TEST 4: 全局间隔热更新（custom 任务保留 cron）"
code=$(curl -s -o /dev/null -w "%{http_code}" -X PUT "$BASE_URL/api/v1/scheduler/interval" "${AUTH[@]}" -d '{"interval":"12h"}')
assert_code "设置全局间隔 12h" 200 "$code" ""
LIST=$(curl -s "$BASE_URL/api/v1/scheduler/tasks" "${AUTH[@]}")
for name in proxy_metadata_cache_gc log_cleanup; do
    SCH=$(echo "$LIST" | task_field "t['schedule']" "$name")
    if [ "$SCH" = "@every 12h" ]; then
        TOTAL=$((TOTAL+1)); PASS_COUNT=$((PASS_COUNT+1)); pass "$name 全局默认变 12h"
    else
        fail "$name 调度=$SCH 应 @every 12h"
    fi
done
SCH=$(echo "$LIST" | task_field "t['schedule']" maven_snapshot)
if [ "$SCH" = "0 3 * * 1" ]; then
    TOTAL=$((TOTAL+1)); PASS_COUNT=$((PASS_COUNT+1)); pass "custom 任务保留 cron"
else
    fail "maven_snapshot 调度=$SCH 应保留 0 3 * * 1"
fi

# ── TEST 5: run-now + 旧接口 ──
section "TEST 5: run-now + 旧 log 清理接口 + snapshot 接口已移除"
code=$(curl -s -o /dev/null -w "%{http_code}" -X POST "$BASE_URL/api/v1/scheduler/tasks/maven_snapshot/run" "${AUTH[@]}")
assert_code "run-now" 200 "$code" ""
code=$(curl -s -o /dev/null -w "%{http_code}" "$BASE_URL/api/v1/download-logs/cleanup/config" "${AUTH[@]}")
assert_code "旧 log 清理 GetConfig" 200 "$code" ""
code=$(curl -s -o /dev/null -w "%{http_code}" -X PUT "$BASE_URL/api/v1/download-logs/cleanup/config" "${AUTH[@]}" \
    -d '{"enabled":true,"retention_days":30,"interval":"24h"}')
assert_code "旧 log 清理 UpdateConfig" 200 "$code" ""
CT=$(curl -s -o /dev/null -w "%{content_type}" "$BASE_URL/api/v1/download-logs/snapshot-cleanup/config" "${AUTH[@]}")
if [[ "$CT" == application/json* ]]; then
    fail "snapshot-cleanup 接口应已移除（返回 JSON=$CT）"
else
    TOTAL=$((TOTAL+1)); PASS_COUNT=$((PASS_COUNT+1)); pass "snapshot-cleanup 接口已移除（$CT）"
fi

# ── 汇总 ──
section "汇总"
echo "通过: $PASS_COUNT / $TOTAL"
if [ "$FAIL_COUNT" -gt 0 ]; then
    echo -e "${RED}失败: $FAIL_COUNT${NC}"; echo "服务日志: $WORK_DIR/server.log"; exit "$FAIL_COUNT"
else
    echo -e "${GREEN}全部通过${NC}"; exit 0
fi
