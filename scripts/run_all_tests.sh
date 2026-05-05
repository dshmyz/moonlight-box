#!/bin/bash

BASE_URL="${1:-http://localhost:9081}"
SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"

RED='\033[0;31m'
GREEN='\033[0;32m'
YELLOW='\033[1;33m'
BLUE='\033[0;34m'
NC='\033[0m'

TOTAL_PASS=0
TOTAL_FAIL=0

echo "============================================"
echo " 完整测试套件"
echo " 目标: $BASE_URL"
echo "============================================"
echo

run_test() {
    local test_name="$1"
    local test_script="$2"
    
    echo -e "${BLUE}运行测试: $test_name${NC}"
    echo "----------------------------------------"
    
    if bash "$test_script" > /tmp/test_output_$$ 2>&1; then
        echo -e "${GREEN}✓ $test_name 通过${NC}"
    else
        echo -e "${RED}✗ $test_name 失败${NC}"
    fi
    
    if [ -f /tmp/test_output_$$ ]; then
        PASS=$(grep -c "✓ PASS" /tmp/test_output_$$ 2>/dev/null || echo "0")
        FAIL=$(grep -c "✗ FAIL" /tmp/test_output_$$ 2>/dev/null || echo "0")
        PASS=$(echo "$PASS" | tr -d '[:space:]')
        FAIL=$(echo "$FAIL" | tr -d '[:space:]')
        PASS=${PASS:-0}
        FAIL=${FAIL:-0}
        echo "  通过: $PASS, 失败: $FAIL"
        TOTAL_PASS=$((TOTAL_PASS + PASS))
        TOTAL_FAIL=$((TOTAL_FAIL + FAIL))
        
        rm -f /tmp/test_output_$$
    else
        echo -e "${YELLOW}  警告: 未找到测试输出文件${NC}"
    fi
    echo
}

echo "════════════════════════════════════════"
echo "  1. 代理仓库测试"
echo "════════════════════════════════════════"
run_test "代理仓库测试" "$SCRIPT_DIR/test_all_proxy.sh"

echo "════════════════════════════════════════"
echo "  2. 基础 HTTP 接口测试"
echo "════════════════════════════════════════"
run_test "基础 HTTP 接口测试" "$SCRIPT_DIR/test_basic_http.sh"

echo "════════════════════════════════════════"
echo "  3. 认证与权限测试"
echo "════════════════════════════════════════"
run_test "认证与权限测试" "$SCRIPT_DIR/test_auth.sh"

echo "════════════════════════════════════════"
echo "  4. Maven 生命周期测试"
echo "════════════════════════════════════════"
if command -v mvn &> /dev/null; then
    run_test "Maven 生命周期测试" "$SCRIPT_DIR/test_maven_lifecycle.sh"
else
    echo -e "${YELLOW}跳过 Maven 测试（需要安装 Maven）${NC}"
    echo
fi

echo "════════════════════════════════════════"
echo "  5. npm 生命周期测试"
echo "════════════════════════════════════════"
if command -v npm &> /dev/null; then
    run_test "npm 生命周期测试" "$SCRIPT_DIR/test_npm_lifecycle.sh"
else
    echo -e "${YELLOW}跳过 npm 测试（需要安装 Node.js）${NC}"
    echo
fi

echo "════════════════════════════════════════"
echo "  6. PyPI 生命周期测试"
echo "════════════════════════════════════════"
if command -v pip &> /dev/null; then
    run_test "PyPI 生命周期测试" "$SCRIPT_DIR/test_pypi_lifecycle.sh"
else
    echo -e "${YELLOW}跳过 PyPI 测试（需要安装 Python 和 pip）${NC}"
    echo
fi

echo "============================================"
echo " 总体测试汇总"
echo "============================================"
echo -e "  总通过: ${GREEN}$TOTAL_PASS${NC}"
echo -e "  总失败: ${RED}$TOTAL_FAIL${NC}"
echo -e "  总计: $((TOTAL_PASS + TOTAL_FAIL))"
echo

if [ $TOTAL_FAIL -eq 0 ]; then
    echo -e "${GREEN}所有测试通过! ✓${NC}"
    exit 0
else
    echo -e "${YELLOW}部分测试失败! ❌${NC}"
    exit 1
fi
