#!/bin/bash

set -e

BASE_URL="${1:-http://localhost:9081}"
ADMIN_USER="${ADMIN_USER:-admin}"
ADMIN_PASS="${ADMIN_PASS:-admin123}"
CURL_CONNECT_TIMEOUT="${CURL_CONNECT_TIMEOUT:-5}"
CURL_MAX_TIME="${CURL_MAX_TIME:-30}"
CURL_OPTS=(--connect-timeout "$CURL_CONNECT_TIMEOUT" --max-time "$CURL_MAX_TIME")

RED='\033[0;31m'
GREEN='\033[0;32m'
YELLOW='\033[1;33m'
BLUE='\033[0;34m'
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

info() {
    echo -e "  ${BLUE}ℹ INFO${NC} $1"
}

warn() {
    echo -e "  ${YELLOW}⚠ WARN${NC} $1"
    WARN_COUNT=$((WARN_COUNT + 1))
}

get_auth_token() {
    curl -s "${CURL_OPTS[@]}" -X POST "$BASE_URL/api/v1/auth/login" \
        -H "Content-Type: application/json" \
        -d "{\"username\":\"$ADMIN_USER\",\"password\":\"$ADMIN_PASS\"}" | \
        grep -o '"access_token":"[^"]*"' | \
        sed 's/"access_token":"//;s/"//'
}

check_pip() {
    PIP_CMD=""
    if command -v pip3 &> /dev/null; then
        PIP_CMD="pip3"
    elif command -v pip &> /dev/null; then
        PIP_CMD="pip"
    elif python3 -m pip --version &> /dev/null; then
        PIP_CMD="python3 -m pip"
    fi
    if [ -z "$PIP_CMD" ]; then
        warn "pip/pip3 命令未安装，跳过 PyPI 生命周期测试"
        return 1
    fi
    return 0
}

# cleanup
CLEAN_TEMPS=()
cleanup() { rm -rf "${CLEAN_TEMPS[@]}" 2>/dev/null || true; }
trap cleanup EXIT

echo "============================================"
echo " PyPI 生命周期测试"
echo " 目标: $BASE_URL"
echo "============================================"
echo

if ! check_pip; then
    echo -e "${YELLOW}跳过 PyPI 测试（需要安装 Python 和 pip）${NC}"
    exit 0
fi

TOKEN=$(get_auth_token)

if [ -z "$TOKEN" ]; then
    echo -e "${RED}错误: 无法获取认证令牌${NC}"
    exit 1
fi

TEST_DIR="/tmp/pypi-test-$$"
CLEAN_TEMPS+=("$TEST_DIR")
mkdir -p "$TEST_DIR"

echo "════════════════════════════════════════"
echo "  测试 1: 创建测试 Python 包"
echo "════════════════════════════════════════"

cd "$TEST_DIR"

mkdir -p test_pypi_package
cat > test_pypi_package/__init__.py <<'EOF'
"""Test PyPI Package"""

__version__ = "1.0.0"

def greet(name):
    """Greet someone"""
    return f"Hello, {name}!"
EOF

cat > setup.py <<'EOF'
from setuptools import setup, find_packages

setup(
    name="test-pypi-package",
    version="1.0.0",
    packages=find_packages(),
    author="Test Author",
    author_email="test@example.com",
    description="A test package for PyPI lifecycle testing",
    python_requires=">=3.6",
)
EOF

cat > pyproject.toml <<'EOF'
[build-system]
requires = ["setuptools", "wheel"]
build-backend = "setuptools.build_meta"
EOF

if [ -f "setup.py" ] && [ -d "test_pypi_package" ]; then
    pass "测试 Python 包创建成功"
else
    fail "测试 Python 包创建失败"
fi

echo
echo "════════════════════════════════════════"
echo "  测试 2: 构建 Python 包"
echo "════════════════════════════════════════"

if python3 -m build > /dev/null 2>&1 || python3 setup.py sdist bdist_wheel > /dev/null 2>&1; then
    pass "Python 包构建成功"
    
    WHEEL_FILE=$(find dist -name "*.whl" | head -n 1)
    if [ -n "$WHEEL_FILE" ] && [ -f "$WHEEL_FILE" ]; then
        pass "Wheel 文件生成成功: $(basename $WHEEL_FILE)"
    else
        warn "Wheel 文件未生成"
    fi
else
    warn "Python 包构建失败（可能需要安装 build 工具）"
fi

echo
echo "════════════════════════════════════════"
echo "  测试 3: 上传到 PyPI 仓库"
echo "════════════════════════════════════════"

if [ -d "dist" ] && [ "$(ls -A dist 2>/dev/null)" ]; then
    UPLOAD_FILE="$WHEEL_FILE"
    if [ -z "$UPLOAD_FILE" ] || [ ! -f "$UPLOAD_FILE" ]; then
        UPLOAD_FILE=$(find dist -type f \( -name "*.whl" -o -name "*.tar.gz" \) | head -n 1)
    fi

    if [ -n "$UPLOAD_FILE" ] && [ -f "$UPLOAD_FILE" ]; then
        UPLOAD_NAME=$(basename "$UPLOAD_FILE")
        UPLOAD_LOG="$TEST_DIR/pypi-upload-response.log"
        HTTP_CODE=$(curl -s "${CURL_OPTS[@]}" -o "$UPLOAD_LOG" -w "%{http_code}" \
            -X PUT "$BASE_URL/repository/pypi-local/packages/$UPLOAD_NAME" \
            -H "Authorization: Bearer $TOKEN" \
            -H "Content-Type: application/octet-stream" \
            --data-binary @"$UPLOAD_FILE")

        if [ "$HTTP_CODE" = "200" ] || [ "$HTTP_CODE" = "201" ]; then
            pass "Python 包上传成功 (HTTP $HTTP_CODE)"

            HTTP_CODE=$(curl -s "${CURL_OPTS[@]}" -o /dev/null -w "%{http_code}" \
                "$BASE_URL/repository/pypi-local/simple/test-pypi-package/")

            if [ "$HTTP_CODE" = "200" ]; then
                pass "上传的包在 Simple Index 中可访问 (HTTP 200)"
            else
                fail "上传的包返回 HTTP $HTTP_CODE"
            fi
        else
            warn "Python 包上传失败 (HTTP $HTTP_CODE)"
            info "上传错误摘要: $(tail -n 5 "$UPLOAD_LOG" | tr '\n' ' ' | sed 's/  */ /g')"
        fi
    else
        warn "没有找到可上传的 wheel/sdist 文件"
    fi
else
    warn "没有可上传的包文件"
fi

echo
echo "════════════════════════════════════════"
echo "  测试 4: 从本地仓库安装"
echo "════════════════════════════════════════"

INSTALL_DIR="/tmp/pypi-install-test-$$"
CLEAN_TEMPS+=("$INSTALL_DIR")
mkdir -p "$INSTALL_DIR"
cd "$INSTALL_DIR"

if python3 -m venv "$INSTALL_DIR/venv" > "$INSTALL_DIR/venv.log" 2>&1; then
    LOCAL_PIP="$INSTALL_DIR/venv/bin/python -m pip"
    LOCAL_PYTHON="$INSTALL_DIR/venv/bin/python"
else
    warn "创建本地安装测试虚拟环境失败"
    info "venv 错误摘要: $(tail -n 5 "$INSTALL_DIR/venv.log" | tr '\n' ' ' | sed 's/  */ /g')"
    LOCAL_PIP="$PIP_CMD"
    LOCAL_PYTHON="python3"
fi

if $LOCAL_PIP install --index-url "$BASE_URL/repository/pypi-local/simple/" \
    --trusted-host localhost --no-cache-dir --force-reinstall test-pypi-package > "$INSTALL_DIR/pip-local-install.log" 2>&1; then
    pass "从本地仓库安装 Python 包成功"
    
    if $LOCAL_PYTHON -c "import test_pypi_package" 2>/dev/null; then
        pass "导入安装的包成功"
    else
        fail "导入安装的包失败"
    fi
else
    warn "从本地仓库安装 Python 包失败"
    info "pip 错误摘要: $(tail -n 5 "$INSTALL_DIR/pip-local-install.log" | tr '\n' ' ' | sed 's/  */ /g')"
fi

echo
echo "════════════════════════════════════════"
echo "  测试 5: 从代理仓库安装"
echo "════════════════════════════════════════"

PROXY_INSTALL_DIR="/tmp/pypi-proxy-install-test-$$"
CLEAN_TEMPS+=("$PROXY_INSTALL_DIR")
mkdir -p "$PROXY_INSTALL_DIR"
cd "$PROXY_INSTALL_DIR"

if python3 -m venv "$PROXY_INSTALL_DIR/venv" > "$PROXY_INSTALL_DIR/venv.log" 2>&1; then
    PROXY_PIP="$PROXY_INSTALL_DIR/venv/bin/python -m pip"
    PROXY_PYTHON="$PROXY_INSTALL_DIR/venv/bin/python"
else
    warn "创建代理安装测试虚拟环境失败"
    info "venv 错误摘要: $(tail -n 5 "$PROXY_INSTALL_DIR/venv.log" | tr '\n' ' ' | sed 's/  */ /g')"
    PROXY_PIP="$PIP_CMD"
    PROXY_PYTHON="python3"
fi

if $PROXY_PIP install --index-url "$BASE_URL/repository/pypi-proxy-tuna/simple/" \
    --trusted-host localhost --no-cache-dir requests > "$PROXY_INSTALL_DIR/pip-proxy-install.log" 2>&1; then
    pass "从代理仓库安装 requests 成功"
    
    if $PROXY_PYTHON -c "import requests" 2>/dev/null; then
        pass "导入 requests 成功"
    else
        fail "导入 requests 失败"
    fi
else
    warn "从代理仓库安装 requests 失败"
    info "pip 错误摘要: $(tail -n 5 "$PROXY_INSTALL_DIR/pip-proxy-install.log" | tr '\n' ' ' | sed 's/  */ /g')"
fi

echo
echo "════════════════════════════════════════"
echo "  测试 6: 清理测试文件"
echo "════════════════════════════════════════"

cd /
rm -rf "$TEST_DIR" "$INSTALL_DIR" "$PROXY_INSTALL_DIR"

if [ ! -d "$TEST_DIR" ] && [ ! -d "$INSTALL_DIR" ] && [ ! -d "$PROXY_INSTALL_DIR" ]; then
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
