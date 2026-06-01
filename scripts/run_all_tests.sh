#!/bin/bash

set -e

BASE_URL="${1:-http://localhost:9081}"
ADMIN_USER="${ADMIN_USER:-admin}"
ADMIN_PASS="${ADMIN_PASS:-admin123}"
TEST_SUITE="${TEST_SUITE:-all}"
TEST_TIMEOUT="${TEST_TIMEOUT:-180}"  # per-test timeout (seconds)

RED='\033[0;31m'
GREEN='\033[0;32m'
YELLOW='\033[1;33m'
BLUE='\033[0;34m'
CYAN='\033[0;36m'
BOLD='\033[1m'
NC='\033[0m'

SCRIPT_DIR="$(cd "$(dirname "$0")" && pwd)"

TOTAL_PASS=0
TOTAL_FAIL=0
TOTAL_SKIP=0

# temp files to clean up on exit
CLEANUP_FILES=()

cleanup() {
    if [ ${#CLEANUP_FILES[@]} -gt 0 ]; then
        rm -rf "${CLEANUP_FILES[@]}" 2>/dev/null || true
    fi
}
trap cleanup EXIT

TOTAL_PASS=0
TOTAL_FAIL=0
TOTAL_SKIP=0

print_header() {
    echo -e "\n${CYAN}╔══════════════════════════════════════════════════════════╗${NC}"
    echo -e "${CYAN}║${BOLD}  制品仓库完整能力测试套件${NC}${CYAN}                          ║${NC}"
    echo -e "${CYAN}║${BOLD}  Artifact Repository Test Suite${NC}${CYAN}                      ║${NC}"
    echo -e "${CYAN}╚══════════════════════════════════════════════════════════╝${NC}"
    echo ""
    echo -e "  目标地址: ${BLUE}$BASE_URL${NC}"
    echo -e "  测试套件: ${BLUE}$TEST_SUITE${NC}"
    echo -e "  执行时间: $(date '+%Y-%m-%d %H:%M:%S')"
    echo ""
}

print_section() {
    echo -e "\n${YELLOW}═══════════════════════════════════════════════════════${NC}"
    echo -e "  ${YELLOW}$1${NC}"
    echo -e "${YELLOW}═══════════════════════════════════════════════════════${NC}"
}

run_test() {
    local test_name="$1"
    local test_script="$2"
    local enabled="$3"

    if [ "$enabled" != "true" ]; then
        echo -e "  ${YELLOW}⊘ SKIP${NC} $test_name"
        TOTAL_SKIP=$((TOTAL_SKIP + 1))
        return 0
    fi

    echo -e "\n${BLUE}▶ 执行: $test_name${NC}"
    echo -e "  脚本: $test_script"

    if [ ! -f "$test_script" ]; then
        echo -e "  ${RED}✗ 错误: 测试脚本不存在${NC}"
        TOTAL_FAIL=$((TOTAL_FAIL + 1))
        return 1
    fi

    chmod +x "$test_script"

    # 带超时执行（macOS 需要 gtimeout），超时算 FAIL
    USE_TIMEOUT=""
    if command -v timeout &>/dev/null; then
        USE_TIMEOUT="timeout"
    elif command -v gtimeout &>/dev/null; then
        USE_TIMEOUT="gtimeout"
    fi
    if [ -n "$USE_TIMEOUT" ]; then
        if "$USE_TIMEOUT" "$TEST_TIMEOUT" bash "$test_script" "$BASE_URL"; then
            TOTAL_PASS=$((TOTAL_PASS + 1))
        else
            local rc=$?
            if [ $rc -eq 124 ]; then
                echo -e "  ${RED}✗ 超时: $test_name 超过 ${TEST_TIMEOUT}s${NC}"
            fi
            TOTAL_FAIL=$((TOTAL_FAIL + 1))
        fi
    else
        if bash "$test_script" "$BASE_URL"; then
            TOTAL_PASS=$((TOTAL_PASS + 1))
        else
            TOTAL_FAIL=$((TOTAL_FAIL + 1))
        fi
    fi
    return 0
}

check_prerequisites() {
    echo -e "${BOLD}检查测试环境...${NC}"
    
    if ! curl -s "$BASE_URL/api/v1/health" > /dev/null 2>&1; then
        echo -e "${RED}错误: 无法连接到制品仓库服务 ($BASE_URL)${NC}"
        echo -e "${YELLOW}请确保服务已启动并运行在 $BASE_URL${NC}"
        exit 1
    fi
    
    echo -e "  ${GREEN}✓${NC} 服务连接正常"
    
    if command -v mvn &> /dev/null; then
        echo -e "  ${GREEN}✓${NC} Maven 已安装: $(mvn -version 2>&1 | head -1)"
    else
        echo -e "  ${YELLOW}⚠${NC} Maven 未安装（部分测试将跳过）"
    fi
    
    if command -v npm &> /dev/null; then
        echo -e "  ${GREEN}✓${NC} npm 已安装: $(npm -version)"
    else
        echo -e "  ${YELLOW}⚠${NC} npm 未安装（部分测试将跳过）"
    fi
    
    if command -v go &> /dev/null; then
        echo -e "  ${GREEN}✓${NC} Go 已安装: $(go version)"
    elif [ -x /usr/local/go/bin/go ]; then
        echo -e "  ${GREEN}✓${NC} Go 已安装: $(/usr/local/go/bin/go version)"
        export PATH="/usr/local/go/bin:$PATH"
    else
        echo -e "  ${YELLOW}⚠${NC} Go 未安装（部分测试将跳过）"
    fi
    
    if command -v pip &> /dev/null; then
        echo -e "  ${GREEN}✓${NC} pip 已安装: $(pip --version | awk '{print $2}')"
    elif command -v pip3 &> /dev/null; then
        echo -e "  ${GREEN}✓${NC} pip3 已安装: $(pip3 --version | awk '{print $2}')"
    elif python3 -m pip --version &> /dev/null; then
        echo -e "  ${GREEN}✓${NC} python3 -m pip 可用: $(python3 -m pip --version | awk '{print $2}')"
    else
        echo -e "  ${YELLOW}⚠${NC} pip 未安装（部分测试将跳过）"
    fi
    
    if command -v ab &> /dev/null; then
        echo -e "  ${GREEN}✓${NC} Apache Bench 已安装"
    else
        echo -e "  ${YELLOW}⚠${NC} Apache Bench 未安装（性能测试将跳过）"
    fi
    
    echo ""
}

print_summary() {
    echo -e "\n${CYAN}╔══════════════════════════════════════════════════════════╗${NC}"
    echo -e "${CYAN}║${BOLD}  测试执行摘要${NC}${CYAN}                                          ║${NC}"
    echo -e "${CYAN}╚══════════════════════════════════════════════════════════╝${NC}"
    echo ""
    echo -e "  ${GREEN}通过: $TOTAL_PASS${NC}"
    echo -e "  ${RED}失败: $TOTAL_FAIL${NC}"
    echo -e "  ${YELLOW}跳过: $TOTAL_SKIP${NC}"
    echo -e "  总计: $((TOTAL_PASS + TOTAL_FAIL + TOTAL_SKIP))"
    echo ""
    
    if [ $TOTAL_FAIL -eq 0 ]; then
        echo -e "${GREEN}${BOLD}✓ 所有测试通过!${NC}"
        exit 0
    else
        echo -e "${YELLOW}${BOLD}⚠ 部分测试失败，请查看上方输出${NC}"
        exit 1
    fi
}

print_header
check_prerequisites

print_section "0/0 初始化测试仓库"
if [ -f "$SCRIPT_DIR/setup_test_repos.sh" ]; then
    bash "$SCRIPT_DIR/setup_test_repos.sh" "$BASE_URL"
else
    echo -e "  ${YELLOW}⚠  setup_test_repos.sh 不存在，跳过仓库初始化${NC}"
fi

case "$TEST_SUITE" in
    all)
        print_section "第一阶段: 基础 HTTP 接口测试"
        run_test "基础 HTTP 接口" "$SCRIPT_DIR/core/test_basic_http.sh" "true"
        
        print_section "第二阶段: 认证与权限测试"
        run_test "认证与权限" "$SCRIPT_DIR/core/test_auth.sh" "true"
        
        print_section "第三阶段: Maven 完整生命周期"
        run_test "Maven Release 版本" "$SCRIPT_DIR/lifecycle/test_maven_lifecycle.sh" "true"
        run_test "Maven SNAPSHOT 版本" "$SCRIPT_DIR/lifecycle/test_maven_snapshot.sh" "true"
        
        print_section "第四阶段: npm 完整生命周期"
        run_test "npm 生命周期" "$SCRIPT_DIR/lifecycle/test_npm_lifecycle.sh" "true"
        
        print_section "第五阶段: Go 模块完整生命周期"
        run_test "Go 模块生命周期" "$SCRIPT_DIR/lifecycle/test_go_lifecycle.sh" "true"
        
        print_section "第六阶段: PyPI 完整生命周期"
        run_test "PyPI 生命周期" "$SCRIPT_DIR/lifecycle/test_pypi_lifecycle.sh" "true"
        
        print_section "第七阶段: 代理仓库能力"
        run_test "多协议代理" "$SCRIPT_DIR/proxy/test_all_proxy.sh" "true"
        
        print_section "第八阶段: 仓库组能力"
        run_test "仓库组（Group）" "$SCRIPT_DIR/core/test_group_repository.sh" "true"
        
        print_section "第九阶段: 性能与压力测试"
        run_test "性能与压力" "$SCRIPT_DIR/performance/test_performance.sh" "true"
        
        print_section "第十阶段: 异常场景测试"
        run_test "异常场景" "$SCRIPT_DIR/exception/test_exception_scenarios.sh" "true"
        
        # ── 新增：数据准确性与架构合规测试 ──────────────────────
        print_section "第十一阶段: 路由日志准确性测试"
        run_test "路由日志准确性" "$SCRIPT_DIR/core/test_router_logging.sh" "true"
        
        print_section "第十二阶段: QueryArtifacts 回源路径测试"
        run_test "QueryArtifacts 回源" "$SCRIPT_DIR/core/test_queryartifacts.sh" "true"
        ;;
    
    basic)
        print_section "基础测试套件"
        run_test "基础 HTTP 接口" "$SCRIPT_DIR/core/test_basic_http.sh" "true"
        run_test "认证与权限" "$SCRIPT_DIR/core/test_auth.sh" "true"
        ;;
    
    maven)
        print_section "Maven 测试套件"
        run_test "Maven Release 版本" "$SCRIPT_DIR/lifecycle/test_maven_lifecycle.sh" "true"
        run_test "Maven SNAPSHOT 版本" "$SCRIPT_DIR/lifecycle/test_maven_snapshot.sh" "true"
        ;;
    
    npm)
        print_section "npm 测试套件"
        run_test "npm 生命周期" "$SCRIPT_DIR/lifecycle/test_npm_lifecycle.sh" "true"
        ;;
    
    go)
        print_section "Go 测试套件"
        run_test "Go 模块生命周期" "$SCRIPT_DIR/lifecycle/test_go_lifecycle.sh" "true"
        ;;
    
    pypi)
        print_section "PyPI 测试套件"
        run_test "PyPI 生命周期" "$SCRIPT_DIR/lifecycle/test_pypi_lifecycle.sh" "true"
        ;;
    
    proxy)
        print_section "代理仓库测试套件"
        run_test "多协议代理" "$SCRIPT_DIR/proxy/test_all_proxy.sh" "true"
        ;;
    
    group)
        print_section "仓库组测试套件"
        run_test "仓库组（Group）" "$SCRIPT_DIR/core/test_group_repository.sh" "true"
        ;;
    
    performance)
        print_section "性能测试套件"
        run_test "性能与压力" "$SCRIPT_DIR/performance/test_performance.sh" "true"
        ;;
    
    exception)
        print_section "异常场景测试套件"
        run_test "异常场景" "$SCRIPT_DIR/exception/test_exception_scenarios.sh" "true"
        ;;
    
    *)
        echo -e "${RED}错误: 未知的测试套件 '$TEST_SUITE'${NC}"
        echo ""
        echo "可用的测试套件:"
        echo "  all         - 执行所有测试（默认）"
        echo "  basic       - 基础 HTTP 和认证测试"
        echo "  maven       - Maven 完整生命周期测试"
        echo "  npm         - npm 完整生命周期测试"
        echo "  go          - Go 模块完整生命周期测试"
        echo "  pypi        - PyPI 完整生命周期测试"
        echo "  proxy       - 代理仓库能力测试"
        echo "  group       - 仓库组能力测试"
        echo "  performance - 性能与压力测试"
        echo "  exception   - 异常场景测试"
        exit 1
        ;;
esac

print_summary
