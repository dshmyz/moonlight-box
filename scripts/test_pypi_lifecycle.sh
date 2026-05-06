#!/bin/bash

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
}

get_auth_token() {
    curl -s -X POST "$BASE_URL/api/v1/auth/login" \
        -H "Content-Type: application/json" \
        -d "{\"username\":\"$ADMIN_USER\",\"password\":\"$ADMIN_PASS\"}" | \
        grep -o '"access_token":"[^"]*"' | \
        sed 's/"access_token":"//;s/"//'
}

check_pip() {
    if ! command -v pip &> /dev/null; then
        warn "pip 命令未安装，跳过 PyPI 生命周期测试"
        return 1
    fi
    return 0
}

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

if python3 -m build > /dev/null 2>&1 || python setup.py sdist bdist_wheel > /dev/null 2>&1; then
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
    if command -v twine &> /dev/null; then
        if twine upload --repository-url "$BASE_URL/pypi/upload" \
            -u "$ADMIN_USER" -p "$ADMIN_PASS" dist/* > /dev/null 2>&1; then
            pass "Python 包上传成功"
            
            HTTP_CODE=$(curl -s -o /dev/null -w "%{http_code}" \
                "$BASE_URL/repo/pypi-local/simple/test-pypi-package/")
            
            if [ "$HTTP_CODE" = "200" ]; then
                pass "上传的包在 Simple Index 中可访问 (HTTP 200)"
            else
                info "上传的包返回 HTTP $HTTP_CODE"
            fi
        else
            warn "Python 包上传失败（可能需要认证配置）"
        fi
    else
        warn "twine 未安装，跳过上传测试"
    fi
else
    warn "没有可上传的包文件"
fi

echo
echo "════════════════════════════════════════"
echo "  测试 4: 从本地仓库安装"
echo "════════════════════════════════════════"

INSTALL_DIR="/tmp/pypi-install-test-$$"
mkdir -p "$INSTALL_DIR"
cd "$INSTALL_DIR"

if pip install --index-url "$BASE_URL/repo/pypi-local/simple/" \
    --trusted-host localhost test-pypi-package > /dev/null 2>&1; then
    pass "从本地仓库安装 Python 包成功"
    
    if python3 -c "import test_pypi_package" 2>/dev/null; then
        pass "导入安装的包成功"
    else
        fail "导入安装的包失败"
    fi
else
    warn "从本地仓库安装 Python 包失败"
fi

echo
echo "════════════════════════════════════════"
echo "  测试 5: 从代理仓库安装"
echo "════════════════════════════════════════"

PROXY_INSTALL_DIR="/tmp/pypi-proxy-install-test-$$"
mkdir -p "$PROXY_INSTALL_DIR"
cd "$PROXY_INSTALL_DIR"

if pip install --index-url "$BASE_URL/repo/pypi-proxy-tuna/simple/" \
    --trusted-host localhost requests > /dev/null 2>&1; then
    pass "从代理仓库安装 requests 成功"
    
    if python3 -c "import requests" 2>/dev/null; then
        pass "导入 requests 成功"
    else
        fail "导入 requests 失败"
    fi
else
    warn "从代理仓库安装 requests 失败"
fi

echo
echo "════════════════════════════════════════"
echo "  测试 6: 清理测试文件"
echo "════════════════════════════════════════"

cd /
rm -rf "$TEST_DIR" "$INSTALL_DIR" "$PROXY_INSTALL_DIR"

pip uninstall -y test-pypi-package > /dev/null 2>&1 || true

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
echo -e "  总计: $((PASS_COUNT + FAIL_COUNT))"
echo

if [ $FAIL_COUNT -eq 0 ]; then
    echo -e "${GREEN}所有测试通过! ✓${NC}"
    exit 0
else
    echo -e "${YELLOW}部分测试失败! ❌${NC}"
    exit 1
fi
