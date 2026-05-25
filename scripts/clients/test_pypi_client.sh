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

# 检查 python3 命令（macOS 默认没有 python）
PYTHON_CMD=""
if command -v python3 &> /dev/null; then
    PYTHON_CMD="python3"
elif command -v python &> /dev/null; then
    PYTHON_CMD="python"
else
    warn "python 命令未安装，跳过测试"
    exit 0
fi

# 检查 pip 命令
PIP_CMD=""
if command -v pip3 &> /dev/null; then
    PIP_CMD="pip3"
elif command -v pip &> /dev/null; then
    PIP_CMD="pip"
else
    PIP_CMD="$PYTHON_CMD -m pip"
fi

info "Python 版本: $($PYTHON_CMD --version 2>&1)"
info "pip 版本: $($PIP_CMD --version 2>&1 | awk '{print $2}')"

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
$PYTHON_CMD -m venv venv
if [ -d "venv" ]; then
    pass "虚拟环境创建成功"
    source venv/bin/activate

    info "pip 版本: $(pip --version 2>&1 | awk '{print $2}')"
else
    warn "虚拟环境创建失败"
fi

echo
echo "测试 4: 配置 pip 仓库为 PyPI 代理..."
pip config set global.index-url "$BASE_URL/repository/pypi-proxy-tuna/simple" 2>/dev/null || true
pip config set global.trusted-host "$(echo $BASE_URL | sed 's|http://||;s|https://||')" 2>/dev/null || true
info "当前 index-url: $(pip config get global.index-url 2>/dev/null || echo '未设置')"

echo
echo "测试 5: 使用 pip install 安装 requests..."
if pip install requests==2.31.0 --no-cache-dir &> /tmp/pip-install-requests.log 2>&1; then
    pass "pip install requests 测试通过"

    if $PYTHON_CMD -c "import requests; print(requests.__version__)" &> /dev/null; then
        VERSION=$($PYTHON_CMD -c "import requests; print(requests.__version__)")
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
if pip install requests==2.31.0 click==8.1.7 --no-cache-dir &> /tmp/pip-install-multiple.log 2>&1; then
    pass "pip install 多个包测试通过"

    if $PYTHON_CMD -c "import requests; print('requests:', requests.__version__)" &> /dev/null; then
        pass "requests 包已安装"
    else
        warn "requests 包导入失败"
    fi

    if $PYTHON_CMD -c "import click; print('click:', click.__version__)" &> /dev/null; then
        pass "click 包已安装"
    else
        warn "click 包导入失败"
    fi
else
    warn "pip install 多个包测试失败"
    tail -5 /tmp/pip-install-multiple.log
fi

echo
echo "测试 9: 验证 wheel 包下载..."
# 从 Simple API 获取真实的 wheel 下载 URL
WHEEL_URL=$(curl -s "$BASE_URL/repository/pypi-proxy-tuna/simple/requests/" | \
    grep -o 'href="[^"]*\.whl[^"]*"' | head -1 | sed 's/href="//;s/"//')

if [ -n "$WHEEL_URL" ]; then
    # 将相对路径转换为绝对 URL
    FULL_URL="$BASE_URL/repository/pypi-proxy-tuna/packages/$(echo $WHEEL_URL | sed 's|.*/packages/||')"
    info "下载 URL: $FULL_URL"

    HTTP_CODE=$(curl -s -o /tmp/pypi-wheel.whl -w "%{http_code}" "$FULL_URL")

    if [ "$HTTP_CODE" = "200" ]; then
        pass "wheel 包下载成功 (HTTP 200)"

        if [ -s "/tmp/pypi-wheel.whl" ]; then
            pass "wheel 文件非空"
            info "wheel 大小: $(du -sh /tmp/pypi-wheel.whl | cut -f1)"

            # wheel 本质是 zip 文件
            if unzip -t /tmp/pypi-wheel.whl > /dev/null 2>&1; then
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
else
    warn "无法从 Simple API 解析 wheel URL"
fi

echo
echo "测试 10: 测试 pip install 带认证的私有仓库..."
if [ -n "$TOKEN" ]; then
    # 创建一个测试发布包并构建
    pip install build --no-cache-dir &> /tmp/pip-install-build.log 2>&1
    if $PYTHON_CMD -m build --wheel &> /tmp/pip-build.log 2>&1; then
        WHEEL_FILE=$(ls dist/*.whl 2>/dev/null | head -1)
        if [ -n "$WHEEL_FILE" ]; then
            pass "成功构建 wheel 包: $(basename $WHEEL_FILE)"

            # 使用 twine 上传到私有仓库
            if command -v twine &> /dev/null || pip install twine --no-cache-dir &> /dev/null 2>&1; then
                if twine upload --repository-url "$BASE_URL/repository/pypi-local/simple" \
                    -u "$ADMIN_USER" -p "$ADMIN_PASS" "$WHEEL_FILE" &> /tmp/twine-upload.log 2>&1; then
                    pass "twine upload 发布包成功"
                else
                    warn "twine upload 发布包失败"
                    tail -3 /tmp/twine-upload.log
                fi
            else
                info "跳过上传测试 (twine 不可用)"
            fi
        else
            warn "构建 wheel 包失败"
        fi
    else
        warn "构建 wheel 包失败"
        tail -3 /tmp/pip-build.log
    fi
else
    info "跳过认证发布测试 (无认证令牌)"
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
