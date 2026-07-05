#!/bin/bash

set -e

BASE_URL="${1:-http://localhost:9081}"
ADMIN_USER="${ADMIN_USER:-admin}"
ADMIN_PASS="${ADMIN_PASS:-admin123}"
CONCURRENT_USERS="${CONCURRENT_USERS:-10}"
REQUEST_COUNT="${REQUEST_COUNT:-100}"

RED='\033[0;31m'
GREEN='\033[0;32m'
YELLOW='\033[1;33m'
BLUE='\033[0;34m'
CYAN='\033[0;36m'
NC='\033[0m'

PASS_COUNT=0
FAIL_COUNT=0
WARN_COUNT=0

pass() {
    echo -e "  ${GREEN}✓ PASS${NC} $1"
    PASS_COUNT=$((PASS_COUNT + 1))
}

fail() {
    echo -e "  ${RED}✗ FAIL${NC} $1"
    FAIL_COUNT=$((FAIL_COUNT + 1))
}

warn() {
    echo -e "  ${YELLOW}⚠ WARN${NC} $1"
    WARN_COUNT=$((WARN_COUNT + 1))
}

info() {
    echo -e "  ${BLUE}ℹ INFO${NC} $1"
}

section() {
    echo -e "\n${CYAN}════════════════════════════════════════${NC}"
    echo -e "  ${CYAN}$1${NC}"
    echo -e "${CYAN}════════════════════════════════════════${NC}"
}

get_auth_token() {
    curl -s -X POST "$BASE_URL/api/v1/auth/login" \
        -H "Content-Type: application/json" \
        -d "{\"username\":\"$ADMIN_USER\",\"password\":\"$ADMIN_PASS\"}" | \
        grep -o '"access_token":"[^"]*"' | \
        sed 's/"access_token":"//;s/"//'
}

check_ab() {
    if ! command -v ab &> /dev/null; then
        echo -e "${YELLOW}警告: ab (Apache Bench) 未安装${NC}"
        echo -e "${YELLOW}安装方法: brew install httpd (macOS) 或 apt-get install apache2-utils (Ubuntu)${NC}"
        return 1
    fi
    return 0
}

check_wrk() {
    if ! command -v wrk &> /dev/null; then
        echo -e "${YELLOW}警告: wrk 未安装${NC}"
        echo -e "${YELLOW}安装方法: brew install wrk (macOS) 或 apt-get install wrk (Ubuntu)${NC}"
        return 1
    fi
    return 0
}

echo "============================================"
echo " 性能与压力测试"
echo " 目标: $BASE_URL"
echo " 并发用户: $CONCURRENT_USERS"
echo " 请求数量: $REQUEST_COUNT"
echo "============================================"
echo

TOKEN=$(get_auth_token)

if [ -z "$TOKEN" ]; then
    echo -e "${RED}错误: 无法获取认证令牌${NC}"
    exit 1
fi

TEST_DIR="/tmp/perf-test-$$"
mkdir -p "$TEST_DIR"

section "测试 1: 基准性能测试 - 下载小文件"

TEST_FILE="$TEST_DIR/small-test.txt"
echo "Small test content" > "$TEST_FILE"

curl -s -X PUT \
    "$BASE_URL/repository/maven-local/com/test/perf-test/1.0.0/perf-test-1.0.0.txt" \
    -H "Authorization: Bearer $TOKEN" \
    -H "Content-Type: text/plain" \
    --data-binary @"$TEST_FILE" > /dev/null 2>&1

if check_ab; then
    info "使用 Apache Bench 进行基准测试"
    
    AB_OUTPUT=$(ab -n $REQUEST_COUNT -c $CONCURRENT_USERS \
        -H "Authorization: Bearer $TOKEN" \
        "$BASE_URL/repository/maven-local/com/test/perf-test/1.0.0/perf-test-1.0.0.txt" 2>&1)
    
    if echo "$AB_OUTPUT" | grep -qE "Failed requests:\s+0"; then
        pass "基准测试 - 无失败请求"
    else
        FAILED=$(echo "$AB_OUTPUT" | grep "Failed requests:" | awk '{print $NF}')
        warn "基准测试 - $FAILED 个失败请求"
    fi
    
    RPS=$(echo "$AB_OUTPUT" | grep "Requests per second:" | awk '{print $4}')
    info "吞吐量: $RPS requests/sec"
    
    TIME_PER_REQ=$(echo "$AB_OUTPUT" | grep "Time per request:" | head -1 | awk '{print $4}')
    info "平均响应时间: ${TIME_PER_REQ}ms"
    
    P50=$(echo "$AB_OUTPUT" | grep "  50%" | awk '{print $2}')
    P90=$(echo "$AB_OUTPUT" | grep "  90%" | awk '{print $2}')
    P95=$(echo "$AB_OUTPUT" | grep "  95%" | awk '{print $2}')
    P99=$(echo "$AB_OUTPUT" | grep "  99%" | awk '{print $2}')
    
    info "响应时间分布: P50=${P50}ms, P90=${P90}ms, P95=${P95}ms, P99=${P99}ms"
    
    if [ -n "$P99" ] && [ "$(echo "$P99 < 1000" | bc -l 2>/dev/null || echo 0)" = "1" ]; then
        pass "P99 响应时间 < 1000ms"
    else
        warn "P99 响应时间 >= 1000ms"
    fi
else
    warn "跳过基准测试（需要安装 ab）"
fi

section "测试 2: 大文件上传/下载性能"

LARGE_FILE="$TEST_DIR/large-test-100mb.bin"
info "生成 100MB 测试文件..."
dd if=/dev/zero of="$LARGE_FILE" bs=1M count=100 2>/dev/null

if [ -f "$LARGE_FILE" ]; then
    FILE_SIZE=$(stat -f%z "$LARGE_FILE" 2>/dev/null || stat -c%s "$LARGE_FILE" 2>/dev/null)
    info "测试文件大小: $(echo "scale=2; $FILE_SIZE/1024/1024" | bc)MB"
    
    info "上传大文件..."
    START_TIME=$(date +%s%N)
    
    HTTP_CODE=$(curl -s -o /dev/null -w "%{http_code}" -X PUT \
        "$BASE_URL/repository/maven-local/com/test/large-perf-test/1.0.0/large-perf-test-1.0.0.bin" \
        -H "Authorization: Bearer $TOKEN" \
        -H "Content-Type: application/octet-stream" \
        --data-binary @"$LARGE_FILE")
    
    END_TIME=$(date +%s%N)
    UPLOAD_TIME=$(( (END_TIME - START_TIME) / 1000000 ))
    
    if [ "$HTTP_CODE" = "200" ] || [ "$HTTP_CODE" = "201" ]; then
        pass "大文件上传成功 (HTTP $HTTP_CODE)"
        info "上传耗时: ${UPLOAD_TIME}ms"
        
        UPLOAD_SPEED=$(echo "scale=2; 100 * 1000 / $UPLOAD_TIME" | bc 2>/dev/null || echo "N/A")
        info "上传速度: ${UPLOAD_SPEED}MB/s"
    else
        fail "大文件上传失败 (HTTP $HTTP_CODE)"
    fi
    
    info "下载大文件..."
    START_TIME=$(date +%s%N)
    
    HTTP_CODE=$(curl -s -o "$TEST_DIR/downloaded-large.bin" -w "%{http_code}" \
        "$BASE_URL/repository/maven-local/com/test/large-perf-test/1.0.0/large-perf-test-1.0.0.bin")
    
    END_TIME=$(date +%s%N)
    DOWNLOAD_TIME=$(( (END_TIME - START_TIME) / 1000000 ))
    
    if [ "$HTTP_CODE" = "200" ]; then
        pass "大文件下载成功 (HTTP 200)"
        info "下载耗时: ${DOWNLOAD_TIME}ms"
        
        DOWNLOAD_SPEED=$(echo "scale=2; 100 * 1000 / $DOWNLOAD_TIME" | bc 2>/dev/null || echo "N/A")
        info "下载速度: ${DOWNLOAD_SPEED}MB/s"
        
        DOWNLOADED_SIZE=$(stat -f%z "$TEST_DIR/downloaded-large.bin" 2>/dev/null || stat -c%s "$TEST_DIR/downloaded-large.bin" 2>/dev/null)
        if [ "$FILE_SIZE" = "$DOWNLOADED_SIZE" ]; then
            pass "下载文件大小一致"
        else
            fail "下载文件大小不一致 (期望: $FILE_SIZE, 实际: $DOWNLOADED_SIZE)"
        fi
    else
        fail "大文件下载失败 (HTTP $HTTP_CODE)"
    fi
else
    fail "测试文件生成失败"
fi

section "测试 3: 并发上传测试"

CONCURRENT_COUNT=10
info "启动 $CONCURRENT_COUNT 个并发上传..."

for i in $(seq 1 $CONCURRENT_COUNT); do
    (
        CONCURRENT_FILE="$TEST_DIR/concurrent-upload-$i.bin"
        dd if=/dev/zero of="$CONCURRENT_FILE" bs=1K count=1024 2>/dev/null
        
        START_TIME=$(date +%s%N)
        HTTP_CODE=$(curl -s -o /dev/null -w "%{http_code}" -X PUT \
            "$BASE_URL/repository/maven-local/com/test/concurrent-test/1.0.0/concurrent-test-$i.bin" \
            -H "Authorization: Bearer $TOKEN" \
            -H "Content-Type: application/octet-stream" \
            --data-binary @"$CONCURRENT_FILE")
        END_TIME=$(date +%s%N)
        ELAPSED=$(( (END_TIME - START_TIME) / 1000000 ))
        
        echo "$HTTP_CODE $ELAPSED" > "$TEST_DIR/result-$i.txt"
    ) &
done

wait

SUCCESS_COUNT=0
FAIL_UPLOAD_COUNT=0
TOTAL_TIME=0

for i in $(seq 1 $CONCURRENT_COUNT); do
    if [ -f "$TEST_DIR/result-$i.txt" ]; then
        RESULT=$(cat "$TEST_DIR/result-$i.txt")
        CODE=$(echo "$RESULT" | awk '{print $1}')
        TIME=$(echo "$RESULT" | awk '{print $2}')
        
        TOTAL_TIME=$((TOTAL_TIME + TIME))
        
        if [ "$CODE" = "200" ] || [ "$CODE" = "201" ]; then
            SUCCESS_COUNT=$((SUCCESS_COUNT + 1))
        else
            FAIL_UPLOAD_COUNT=$((FAIL_UPLOAD_COUNT + 1))
        fi
    fi
done

AVG_TIME=$((TOTAL_TIME / CONCURRENT_COUNT))

if [ "$SUCCESS_COUNT" = "$CONCURRENT_COUNT" ]; then
    pass "并发上传 - 全部成功 ($SUCCESS_COUNT/$CONCURRENT_COUNT)"
else
    fail "并发上传 - $FAIL_UPLOAD_COUNT 个失败 ($SUCCESS_COUNT/$CONCURRENT_COUNT 成功)"
fi

info "平均上传时间: ${AVG_TIME}ms"

section "测试 4: 并发下载测试"

info "启动 $CONCURRENT_COUNT 个并发下载..."

for i in $(seq 1 $CONCURRENT_COUNT); do
    (
        START_TIME=$(date +%s%N)
        HTTP_CODE=$(curl -s -o /dev/null -w "%{http_code}" \
            "$BASE_URL/repository/maven-local/com/test/concurrent-test/1.0.0/concurrent-test-$i.bin")
        END_TIME=$(date +%s%N)
        ELAPSED=$(( (END_TIME - START_TIME) / 1000000 ))
        
        echo "$HTTP_CODE $ELAPSED" > "$TEST_DIR/download-result-$i.txt"
    ) &
done

wait

SUCCESS_DOWNLOAD_COUNT=0
FAIL_DOWNLOAD_COUNT=0
TOTAL_DOWNLOAD_TIME=0

for i in $(seq 1 $CONCURRENT_COUNT); do
    if [ -f "$TEST_DIR/download-result-$i.txt" ]; then
        RESULT=$(cat "$TEST_DIR/download-result-$i.txt")
        CODE=$(echo "$RESULT" | awk '{print $1}')
        TIME=$(echo "$RESULT" | awk '{print $2}')
        
        TOTAL_DOWNLOAD_TIME=$((TOTAL_DOWNLOAD_TIME + TIME))
        
        if [ "$CODE" = "200" ]; then
            SUCCESS_DOWNLOAD_COUNT=$((SUCCESS_DOWNLOAD_COUNT + 1))
        else
            FAIL_DOWNLOAD_COUNT=$((FAIL_DOWNLOAD_COUNT + 1))
        fi
    fi
done

AVG_DOWNLOAD_TIME=$((TOTAL_DOWNLOAD_TIME / CONCURRENT_COUNT))

if [ "$SUCCESS_DOWNLOAD_COUNT" = "$CONCURRENT_COUNT" ]; then
    pass "并发下载 - 全部成功 ($SUCCESS_DOWNLOAD_COUNT/$CONCURRENT_COUNT)"
else
    fail "并发下载 - $FAIL_DOWNLOAD_COUNT 个失败 ($SUCCESS_DOWNLOAD_COUNT/$CONCURRENT_COUNT 成功)"
fi

info "平均下载时间: ${AVG_DOWNLOAD_TIME}ms"

section "测试 5: 持续负载测试"

if check_ab; then
    DURATION=30
    info "执行 ${DURATION} 秒持续负载测试..."
    
    AB_OUTPUT=$(ab -n $REQUEST_COUNT -c $CONCURRENT_USERS -t $DURATION \
        -H "Authorization: Bearer $TOKEN" \
        "$BASE_URL/repository/maven-local/com/test/perf-test/1.0.0/perf-test-1.0.0.txt" 2>&1 || true)
    
    if echo "$AB_OUTPUT" | grep -q "Complete requests:"; then
        COMPLETE=$(echo "$AB_OUTPUT" | grep "Complete requests:" | awk '{print $3}')
        FAILED=$(echo "$AB_OUTPUT" | grep "Failed requests:" | awk '{print $3}')
        
        info "完成请求: $COMPLETE"
        info "失败请求: $FAILED"
        
        if [ "$FAILED" = "0" ]; then
            pass "持续负载测试 - 无失败请求"
        else
            warn "持续负载测试 - $FAILED 个失败请求"
        fi
        
        RPS=$(echo "$AB_OUTPUT" | grep "Requests per second:" | awk '{print $4}')
        info "持续吞吐量: $RPS requests/sec"
    else
        warn "持续负载测试输出解析失败"
    fi
else
    warn "跳过持续负载测试（需要安装 ab）"
fi

section "测试 6: 内存泄漏检测"

info "监控服务内存使用情况..."

if command -v ps &> /dev/null; then
    PID=$(pgrep -f "moonlight-box" | head -1 || pgrep -f "registry" | head -1)
    
    if [ -n "$PID" ]; then
        MEMORY_BEFORE=$(ps -o rss= -p "$PID" 2>/dev/null | tr -d ' ')
        
        if [ -n "$MEMORY_BEFORE" ]; then
            info "测试前内存使用: $(echo "scale=2; $MEMORY_BEFORE/1024" | bc)MB"
            
            for i in $(seq 1 50); do
                curl -s -o /dev/null \
                    "$BASE_URL/repository/maven-local/com/test/perf-test/1.0.0/perf-test-1.0.0.txt" > /dev/null 2>&1
            done
            
            sleep 2
            
            MEMORY_AFTER=$(ps -o rss= -p "$PID" 2>/dev/null | tr -d ' ')
            
            if [ -n "$MEMORY_AFTER" ]; then
                info "测试后内存使用: $(echo "scale=2; $MEMORY_AFTER/1024" | bc)MB"
                
                MEMORY_DIFF=$((MEMORY_AFTER - MEMORY_BEFORE))
                
                if [ "$MEMORY_DIFF" -lt 10240 ]; then
                    pass "内存增长 < 10MB（无明显内存泄漏）"
                else
                    warn "内存增长 $(echo "scale=2; $MEMORY_DIFF/1024" | bc)MB（可能存在内存泄漏）"
                fi
            fi
        fi
    else
        warn "无法找到服务进程"
    fi
else
    warn "跳过内存检测（ps 命令不可用）"
fi

section "测试 7: 连接池测试"

if check_ab; then
    info "测试连接池复用..."
    
    CONNECTIONS=50
    AB_OUTPUT=$(ab -n $REQUEST_COUNT -c $CONNECTIONS \
        -H "Authorization: Bearer $TOKEN" \
        "$BASE_URL/repository/maven-local/com/test/perf-test/1.0.0/perf-test-1.0.0.txt" 2>&1)
    
    if echo "$AB_OUTPUT" | grep -q "Keep-Alive"; then
        KEEP_ALIVE=$(echo "$AB_OUTPUT" | grep "Keep-Alive requests:" | awk '{print $3}')
        info "Keep-Alive 请求数: $KEEP_ALIVE"
        
        if [ "$KEEP_ALIVE" -gt 0 ]; then
            pass "连接池复用正常"
        else
            warn "连接池未复用"
        fi
    else
        info "Keep-Alive 信息不可用"
    fi
else
    warn "跳过连接池测试（需要安装 ab）"
fi

section "测试 8: 清理测试文件"

cd /
rm -rf "$TEST_DIR"

if [ ! -d "$TEST_DIR" ]; then
    pass "测试文件清理成功"
else
    warn "测试文件清理可能不完整"
fi

echo
echo "============================================"
echo " 测试汇总"
echo "============================================"
echo -e "  通过: ${GREEN}$PASS_COUNT${NC}"
echo -e "  失败: ${RED}$FAIL_COUNT${NC}"
echo -e "  警告: ${YELLOW}$WARN_COUNT${NC}"
echo -e "  总计: $((PASS_COUNT + FAIL_COUNT + WARN_COUNT))"
echo

if [ $FAIL_COUNT -eq 0 ]; then
    echo -e "${GREEN}所有测试通过! ✓${NC}"
    exit 0
else
    echo -e "${YELLOW}部分测试失败! ❌${NC}"
    exit 1
fi
