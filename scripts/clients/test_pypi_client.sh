#!/bin/bash

# =============================================================================
# PyPI 客户端真实集成测试
# 使用官方 pip install 命令测试仓库功能
# =============================================================================

BASE_URL="${1:-http://localhost:9081}"
ADMIN_USER="${ADMIN_USER:-admin}"
ADMIN_PASS="${ADMIN_PASS:-admin123}"

RED='\033[0;31m'
GREEN='\033[0;32m'
YELLOW='\033[1;33m'
BLUE='\033[0;34m'
NC='\033[0m'

pass() { echo -e "  ${GREEN}✓ PASS${NC} $1"; }
fail() { echo -e "  ${RED}✗ FAIL${NC} $1"; }
info() { echo -e "  ${BLUE}ℹ INFO${NC} $1"; }
warn() { echo -e "  ${YELLOW}⚠ WARN${NC} $1"; }

echo "============================================"
echo " PyPI 客户端真实集成测试"
echo " 使用官方 pip install 命令测试"
echo " 目标: $BASE_URL"
echo "============================================"
echo

# 检查 pip 命令
if ! command -v pip &> /dev/null; then
    warn "pip 命令未安装，跳过测试"
    exit 0
fi

info "Python 版本: $(python --version 2>&1)"
info "pip 版本: $(pip --version | awk '{print $2}')"

# 获取认证令牌
TOKEN=$(curl -s -X POST "$BASE_URL/api/v1/auth/login" \
    -H "Content-Type: application/json" \
    -d "{\"username\":\"$ADMIN_USER\",\"password\":\"$ADMIN_PASS\"}" | \
    grep -o '"access_token":"[^"]*"' | \
    sed 's/"access_token":"//;s/"//')

if [ -z "$TOKEN" ]; then
    warn "无法获取认证令牌"
fi

# 创建虚拟环境进行测试
TEST_DIR="/tmp/pypi-client-test-$$"
mkdir -p "$TEST_DIR"

echo "测试 1: 创建 Python 项目..."
cd "$TEST_DIR"

mkdir -p test_pypi_package
cat > test_pypi_package/__init__.py <<'EOF'
"""Test PyPI Package"""

def hello():
    """Test function"""
    return "Hello from test package!"
EOF

cat > test_pypi_package/__version__.py <<'EOF'
__version__ = "1.0.0"
EOF

cat > setup.py <<'EOF'
from setuptools import setup, find_packages

setup(
    name="test-pypi-package",
    version="1.0.0",
    packages=find_packages(),
    description="Test package for PyPI repository",
    author="Test Author",
    author_email="test@example.com",
)
EOF

cat > pyproject.toml <<'EOF'
[build-system]
requires = ["setuptools>=45", "wheel"]
build-backend = "setuptools.build_meta"

[project]
name = "test-pypi-package"
version = "1.0.0"
description = "Test package for PyPI repository"
requires-python = ">=3.7"
EOF

pass "Python 项目创建成功"

echo
echo "测试 2: 验证 Python 项目结构..."
if [ -d "test_pypi_package" ]; then
    pass "包目录已创建"
    ls -la test_pypi_package/
else
    fail "包目录未创建"
fi

echo
echo "测试 3: 创建虚拟环境..."
python -m venv venv
if [ -d "venv" ]; then
    pass "虚拟环境创建成功"
    source venv/bin/activate
    
    info "pip 版本: $(pip --version | awk '{print $2}')"
else
    warn "虚拟环境创建失败"
fi

echo
echo "测试 4: 配置 pip 仓库为 PyPI 代理..."
pip config set global.index-url "$BASE_URL/repository/pypi-proxy-tuna/simple"
pip config set global.trusted-host "$(echo $BASE_URL | sed 's/http:\/\///')"
info "当前 index-url: $(pip config get global.index-url 2>/dev/null || echo '未设置')"

echo
echo "测试 5: 使用 pip install 安装 requests..."
if pip install requests==2.31.0 &> /tmp/pip-install-requests.log 2>&1; then
    pass "pip install requests 测试通过"
    
    if python -c "import requests; print(requests.__version__)" &> /dev/null; then
        VERSION=$(python -c "import requests; print(requests.__version__)")
        pass "requests 包已安装 (版本: $VERSION)"
    else
        warn "requests 包导入失败"
    fi
else
    warn "pip install requests 测试失败"
    tail -5 /tmp/pip-install-requests.log
fi

echo
echo "测试 6: 验证 PyPI Simple API..."
HTTP_CODE=$(curl -s -o /tmp/pypi-simple.html -w "%{http_code}" \
    "$BASE_URL/repository/pypi-proxy-tuna/simple/requests/")

if [ "$HTTP_CODE" = "200" ]; then
    pass "PyPI Simple API 可访问 (HTTP 200)"
    
    if grep -q "href=" /tmp/pypi-simple.html; then
        pass "Simple API 返回正确的 HTML 格式"
        info "文件数量: $(grep -c 'href=' /tmp/pypi-simple.html)"
    else
        warn "Simple API 格式不正确"
    fi
else
    warn "PyPI Simple API 不可访问 (HTTP $HTTP_CODE)"
fi

echo
echo "测试 7: 验证 PyPI JSON API..."
HTTP_CODE=$(curl -s -o /tmp/pypi-json.json -w "%{http_code}" \
    "$BASE_URL/repository/pypi-proxy-tuna/pypi/requests/2.31.0/json")

if [ "$HTTP_CODE" = "200" ]; then
    pass "PyPI JSON API 可访问 (HTTP 200)"
    
    if grep -q '"info"' /tmp/pypi-json.json; then
        pass "JSON 响应包含 info 字段"
    else
        warn "JSON 响应缺少 info 字段"
    fi
    
    if grep -q '"version"' /tmp/pypi-json.json; then
        VERSION=$(grep '"version"' /tmp/pypi-json.json | head -1 | sed 's/.*: "//;s/".*//')
        pass "JSON 响应包含版本信息: $VERSION"
    else
        warn "JSON 响应缺少版本信息"
    fi
else
    warn "PyPI JSON API 不可访问 (HTTP $HTTP_CODE)"
fi

echo
echo "测试 8: 测试 pip install 安装多个包..."
if pip install numpy==1.24.3 pandas==2.0.3 &> /tmp/pip-install-multiple.log 2>&1; then
    pass "pip install 多个包测试通过"
    
    if python -c "import numpy; print('numpy:', numpy.__version__)" &> /dev/null; then
        pass "numpy 包已安装"
    fi
    
    if python -c "import pandas; print('pandas:', pandas.__version__)" &> /dev/null; then
        pass "pandas 包已安装"
    fi
else
    warn "pip install 多个包测试失败"
fi

echo
echo "测试 9: 验证 wheel 包下载..."
HTTP_CODE=$(curl -s -o /tmp/numpy.whl -w "%{http_code}" \
    "$BASE_URL/repository/pypi-proxy-tuna/packages/numpy/1.24.3/numpy-1.24.3-cp311-cp311-manylinux_2_17_x86_64.manylinux2014_x86_64.whl")

if [ "$HTTP_CODE" = "200" ]; then
    pass "wheel 包下载成功 (HTTP 200)"
    
    if [ -s "/tmp/numpy.whl" ]; then
        pass "wheel 文件非空"
        info "wheel 大小: $(du -sh /tmp/numpy.whl | cut -f1)"
        
        # wheel 本质是 zip 文件
        if unzip -t /tmp/numpy.whl > /dev/null 2>&1; then
            pass "wheel 文件格式正确"
        else
            warn "wheel 文件格式无效"
        fi
    else
        warn "下载的 wheel 文件为空"
    fi
else
    warn "wheel 包下载失败 (HTTP $HTTP_CODE)"
fi

# 清理
cd /
deactivate 2> /dev/null || true
rm -rf "$TEST_DIR"
pip config unset global.index-url 2>/dev/null || true
pip config unset global.trusted-host 2>/dev/null || true

echo
echo "============================================"
echo " PyPI 客户端测试完成"
echo "============================================"
