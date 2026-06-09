#!/bin/bash
# =============================================================================
# YUM 包命名测试
# 验证 YUM 代理仓库的包搜索结果使用正确的包名，而非文件名
# 同时验证元数据文件不会出现在包列表中
# =============================================================================

set +e

BASE_URL="${1:-http://localhost:9081}"
ADMIN_USER="${ADMIN_USER:-admin}"
ADMIN_PASS="${ADMIN_PASS:-admin123}"

GREEN='\033[0;32m'
RED='\033[0;31m'
YELLOW='\033[1;33m'
BLUE='\033[0;34m'
NC='\033[0m'

PASS=0
FAIL=0
WARN=0
TOTAL=0

log_pass() { echo -e "  ${GREEN}✓ PASS${NC} $1"; PASS=$((PASS + 1)); TOTAL=$((TOTAL + 1)); }
log_fail() { echo -e "  ${RED}✗ FAIL${NC} $1"; FAIL=$((FAIL + 1)); TOTAL=$((TOTAL + 1)); }
log_warn() { echo -e "  ${YELLOW}⚠ WARN${NC} $1"; WARN=$((WARN + 1)); }
log_info() { echo -e "  ${BLUE}ℹ INFO${NC} $1"; }

echo "============================================"
echo " YUM 包命名测试"
echo " 验证包搜索使用包名而非文件名"
echo " 目标: $BASE_URL"
echo "============================================"
echo

# 获取认证令牌
TOKEN=$(curl -s -X POST "$BASE_URL/api/v1/auth/login" \
    -H "Content-Type: application/json" \
    -d "{\"username\":\"$ADMIN_USER\",\"password\":\"$ADMIN_PASS\"}" | \
    grep -o '"access_token":"[^"]*"' | \
    sed 's/"access_token":"//;s/"//')

if [ -z "$TOKEN" ]; then
    log_fail "无法获取认证令牌"
    exit 1
fi

log_pass "获取认证令牌成功"

# 等待批量写入落盘
sleep 2

# ============================================================
# 测试 1: 检查包搜索结果中是否有元数据文件
# ============================================================
echo
echo "测试 1: 验证元数据文件不出现在包列表..."

search_body=$(curl -s "$BASE_URL/api/v1/packages/search?q=&page_size=100&format=yum")

# 检查是否有 repomd.xml、primary.xml.gz 等元数据文件
metadata_names=$(echo "$search_body" | python3 -c "
import sys, json
try:
    data = json.load(sys.stdin)
    items = data.get('data', {}).get('list', [])
    metadata_files = []
    for item in items:
        name = item.get('name', '')
        if name in ['repomd.xml', 'primary.xml.gz', 'filelists.xml.gz', 'other.xml.gz']:
            metadata_files.append(name)
    print('|'.join(metadata_files) if metadata_files else '')
except Exception as e:
    print('ERROR:' + str(e), file=sys.stderr)
    sys.exit(1)
" 2>/dev/null)

if [ -z "$metadata_names" ]; then
    log_pass "包列表中没有元数据文件（repomd.xml、primary.xml.gz 等）"
else
    log_fail "包列表中包含元数据文件: $metadata_names"
fi

# ============================================================
# 测试 2: 检查包名是否为正确的包名（而非文件名）
# ============================================================
echo
echo "测试 2: 验证包名使用包名而非文件名..."

# 获取 YUM 包列表
yum_packages=$(echo "$search_body" | python3 -c "
import sys, json
try:
    data = json.load(sys.stdin)
    items = data.get('data', {}).get('list', [])
    for item in items:
        name = item.get('name', '')
        # 检查是否为 RPM 文件名格式（如 nginx-1.20.1-1.el8.x86_64.rpm）
        if name.endswith('.rpm'):
            print('FILE_NAME:' + name)
        # 检查是否为正确的包名格式（如 nginx、kernel-tools 等）
        elif name and not name.endswith('.xml') and not name.endswith('.gz'):
            print('PACKAGE:' + name)
except Exception as e:
    print('ERROR:' + str(e), file=sys.stderr)
    sys.exit(1)
" 2>/dev/null)

# 分析结果
file_name_count=0
package_name_count=0

while IFS= read -r line; do
    if [[ "$line" == FILE_NAME:* ]]; then
        file_name_count=$((file_name_count + 1))
        log_warn "发现文件名格式: ${line#FILE_NAME:}"
    elif [[ "$line" == PACKAGE:* ]]; then
        package_name_count=$((package_name_count + 1))
    fi
done <<< "$yum_packages"

if [ $file_name_count -gt 0 ]; then
    log_fail "包列表中包含 $file_name_count 个文件名格式的包名（应该是包名）"
elif [ $package_name_count -eq 0 ]; then
    log_warn "未找到任何 YUM 包（可能是新仓库，尚未有代理回源数据）"
else
    log_pass "所有 $package_name_count 个 YUM 包都使用正确的包名格式"
fi

# ============================================================
# 测试 3: 搜索特定包名验证结果
# ============================================================
echo
echo "测试 3: 搜索特定包名验证返回结果..."

# 搜索一个常见的包名
search_query="kernel"
search_result=$(curl -s "$BASE_URL/api/v1/packages/search?q=$search_query&page_size=10&format=yum")

kernel_found=$(echo "$search_result" | python3 -c "
import sys, json
try:
    data = json.load(sys.stdin)
    items = data.get('data', {}).get('list', [])
    for item in items:
        name = item.get('name', '')
        if 'kernel' in name.lower() and not name.endswith('.rpm') and not name.endswith('.xml'):
            print('FOUND:' + name)
            sys.exit(0)
    print('NOT_FOUND')
except Exception as e:
    print('ERROR:' + str(e), file=sys.stderr)
    sys.exit(1)
" 2>/dev/null)

if [[ "$kernel_found" == FOUND:* ]]; then
    log_pass "搜索 '$search_query' 找到包: ${kernel_found#FOUND:}"
else
    log_info "搜索 '$search_query' 未找到（可能是新仓库或上游无此包）"
fi

# ============================================================
# 测试 4: 验证包详情中的版本信息
# ============================================================
echo
echo "测试 4: 验证包详情中的版本信息..."

# 获取第一个 YUM 包的详情
first_package=$(echo "$search_body" | python3 -c "
import sys, json
try:
    data = json.load(sys.stdin)
    items = data.get('data', {}).get('list', [])
    for item in items:
        name = item.get('name', '')
        if name and not name.endswith('.rpm') and not name.endswith('.xml') and not name.endswith('.gz'):
            print(name)
            sys.exit(0)
    print('')
except Exception as e:
    print('ERROR:' + str(e), file=sys.stderr)
    sys.exit(1)
" 2>/dev/null)

if [ -n "$first_package" ]; then
    # 获取该包的版本列表
    versions_result=$(curl -s "$BASE_URL/api/v1/packages/$first_package/versions?format=yum")

    versions_count=$(echo "$versions_result" | python3 -c "
import sys, json
try:
    data = json.load(sys.stdin)
    versions = data.get('data', {}).get('list', [])
    print(str(len(versions)))
except Exception as e:
    print('0')
" 2>/dev/null)

    if [ "$versions_count" -gt 0 ]; then
        log_pass "包 '$first_package' 有 $versions_count 个版本记录"
    else
        log_warn "包 '$first_package' 暂无版本记录"
    fi
else
    log_info "暂无 YUM 包数据可供测试版本详情"
fi

# ============================================================
# 总结
# ============================================================
echo
echo "============================================"
echo " YUM 包命名测试完成"
echo "============================================"
echo -e "  通过: ${GREEN}$PASS${NC}"
echo -e "  失败: ${RED}$FAIL${NC}"
echo -e "  警告: ${YELLOW}$WARN${NC}"
echo

if [ $FAIL -gt 0 ]; then
    exit 1
fi
exit 0
