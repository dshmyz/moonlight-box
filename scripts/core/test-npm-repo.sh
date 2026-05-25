#!/bin/bash

# npm 仓库功能测试脚本
# 用于测试本地、代理和虚拟 npm 仓库的完整功能

set -e

# 颜色定义
RED='\033[0;31m'
GREEN='\033[0;32m'
YELLOW='\033[1;33m'
NC='\033[0m'

# 配置
REGISTRY_URL="${REGISTRY_URL:-http://localhost:9081}"
API_BASE="${REGISTRY_URL}/api/v1"
NPM_REGISTRY="${REGISTRY_URL}/npm/npm-virtual"
PASS_COUNT=0
FAIL_COUNT=0
AUTH_TOKEN=""

# 生成随机后缀
RANDOM_SUFFIX=$(date +%s | tail -c 5)
LOCAL_REPO_NAME="npm-local-test-${RANDOM_SUFFIX}"
PROXY_REPO_NAME="npm-proxy-test-${RANDOM_SUFFIX}"
VIRTUAL_REPO_NAME="npm-virtual-test-${RANDOM_SUFFIX}"

# 测试函数
log_info() {
    echo -e "${YELLOW}[INFO]${NC} $1"
}

log_success() {
    echo -e "${GREEN}[PASS]${NC} $1"
    PASS_COUNT=$((PASS_COUNT + 1))
}

log_error() {
    echo -e "${RED}[FAIL]${NC} $1"
    FAIL_COUNT=$((FAIL_COUNT + 1))
}

# 认证函数
authenticate() {
    log_info "尝试认证..."
    
    RESPONSE=$(curl -s -X POST "${API_BASE}/auth/login" \
        -H "Content-Type: application/json" \
        -d '{
            "username": "admin",
            "password": "admin123"
        }' 2>&1 || true)
    
    if echo "$RESPONSE" | grep -q '"access_token"'; then
        AUTH_TOKEN=$(echo "$RESPONSE" | sed 's/.*"access_token":"\([^"]*\)".*/\1/')
        if [ -n "$AUTH_TOKEN" ]; then
            log_success "认证成功"
            
            # 清理可能残留的阻断规则（避免影响测试）
            log_info "清理残留阻断规则..."
            EXISTING_RULES=$(curl -s "${API_BASE}/block-rules" \
                -H "Authorization: Bearer ${AUTH_TOKEN}")
            echo "$EXISTING_RULES" | python3 -c "
import sys,json
d=json.load(sys.stdin)
rules=d.get('data',[])
for r in rules:
    pid=r.get('id')
    pkg=r.get('package_name','')
    reason=r.get('reason','')
    if reason == 'test' or pkg == 'lodash' or pkg.startswith('test-block'):
        print(pid)
" 2>/dev/null | while read rid; do
                [ -n "$rid" ] && curl -s -X DELETE "${API_BASE}/block-rules/$rid" \
                    -H "Authorization: Bearer ${AUTH_TOKEN}" > /dev/null 2>&1
            done
            # 等待阻断缓存失效
            sleep 1
            return
        fi
    fi
    
    log_info "认证失败，将使用无认证模式测试"
    AUTH_TOKEN=""
}

# 带认证的 HTTP 请求辅助函数
# 用法: http_request METHOD URL [DATA]
http_request() {
    local method="$1"
    local url="$2"
    local data="${3:-}"
    
    local args=(-s -w "\n%{http_code}" -X "$method" "$url" -H "Content-Type: application/json")
    
    if [ -n "$AUTH_TOKEN" ]; then
        args+=(-H "Authorization: Bearer ${AUTH_TOKEN}")
    fi
    
    if [ -n "$data" ]; then
        args+=(-d "$data")
    fi
    
    curl "${args[@]}"
}

# 前置检查
check_prerequisites() {
    log_info "检查前置条件..."
    
    if ! command -v curl &> /dev/null; then
        log_error "curl 未安装"
        exit 1
    fi
    
    # 检查服务是否运行
    if ! curl -s "${REGISTRY_URL}/health" > /dev/null 2>&1; then
        log_error "Registry 服务未运行在 ${REGISTRY_URL}"
        exit 1
    fi
    
    log_success "前置条件检查通过"
}

# 测试 1: 创建本地 npm 仓库
test_create_local_repo() {
    log_info "测试 1: 创建本地 npm 仓库..."
    
    RESPONSE=$(http_request POST "${API_BASE}/repositories" "{
        \"name\": \"${LOCAL_REPO_NAME}\",
        \"display_name\": \"NPM Local Test Repository\",
        \"description\": \"Local npm repository for testing\",
        \"type\": \"local\",
        \"package_type\": \"npm\",
        \"enabled\": true
    }")
    
    HTTP_CODE=$(echo "$RESPONSE" | tail -n1)
    BODY=$(echo "$RESPONSE" | sed '$d')
    
    if [ "$HTTP_CODE" = "200" ] || [ "$HTTP_CODE" = "201" ]; then
        log_success "本地仓库创建成功"
    else
        log_error "本地仓库创建失败: HTTP $HTTP_CODE - $BODY"
    fi
}

# 测试 2: 创建代理 npm 仓库
test_create_proxy_repo() {
    log_info "测试 2: 创建代理 npm 仓库..."
    
    RESPONSE=$(http_request POST "${API_BASE}/repositories" "{
        \"name\": \"${PROXY_REPO_NAME}\",
        \"display_name\": \"NPM Proxy Test Repository\",
        \"description\": \"Proxy npm repository for testing\",
        \"type\": \"proxy\",
        \"package_type\": \"npm\",
        \"remote_url\": \"https://registry.npmjs.org\",
        \"enabled\": true,
        \"cache_enabled\": true,
        \"cache_ttl_seconds\": 3600
    }")
    
    HTTP_CODE=$(echo "$RESPONSE" | tail -n1)
    BODY=$(echo "$RESPONSE" | sed '$d')
    
    if [ "$HTTP_CODE" = "200" ] || [ "$HTTP_CODE" = "201" ]; then
        log_success "代理仓库创建成功"
    else
        log_error "代理仓库创建失败: HTTP $HTTP_CODE - $BODY"
    fi
}

# 测试 3: 创建虚拟 npm 仓库
test_create_virtual_repo() {
    log_info "测试 3: 创建虚拟 npm 仓库..."
    
    RESPONSE=$(http_request POST "${API_BASE}/repositories" "{
        \"name\": \"${VIRTUAL_REPO_NAME}\",
        \"display_name\": \"NPM Virtual Test Repository\",
        \"description\": \"Virtual npm repository for testing\",
        \"type\": \"virtual\",
        \"package_type\": \"npm\",
        \"enabled\": true,
        \"members\": [\"${LOCAL_REPO_NAME}\", \"${PROXY_REPO_NAME}\"]
    }")
    
    HTTP_CODE=$(echo "$RESPONSE" | tail -n1)
    BODY=$(echo "$RESPONSE" | sed '$d')
    
    if [ "$HTTP_CODE" = "200" ] || [ "$HTTP_CODE" = "201" ]; then
        log_success "虚拟仓库创建成功"
    else
        log_error "虚拟仓库创建失败: HTTP $HTTP_CODE - $BODY"
    fi
}

# 测试 4: 获取包元数据（使用公共 NPM 端点）
test_get_package_metadata() {
    log_info "测试 4: 获取公共 npm 包元数据..."
    
    # 使用已有的 npm-proxy-cn 代理仓库（指向 npmmirror.com）
    RESPONSE=$(curl -s -w "\n%{http_code}" "${REGISTRY_URL}/repository/npm-proxy-cn/lodash")
    HTTP_CODE=$(echo "$RESPONSE" | tail -n1)
    BODY=$(echo "$RESPONSE" | sed '$d')
    
    if [ "$HTTP_CODE" = "200" ]; then
        # NPM 包元数据应该包含 name 字段（小写）
        if echo "$BODY" | grep -q '"name"'; then
            log_success "包元数据获取成功"
        else
            log_error "包元数据格式不正确"
        fi
    else
        log_error "包元数据获取失败: HTTP $HTTP_CODE"
    fi
}

# 测试 5: 列出仓库
test_list_repositories() {
    log_info "测试 5: 列出仓库..."
    
    RESPONSE=$(http_request GET "${API_BASE}/repositories")
    HTTP_CODE=$(echo "$RESPONSE" | tail -n1)
    BODY=$(echo "$RESPONSE" | sed '$d')
    
    if [ "$HTTP_CODE" = "200" ]; then
        if echo "$BODY" | grep -q '"data"'; then
            log_success "仓库列表获取成功"
        else
            log_error "仓库列表格式不正确"
        fi
    else
        log_error "仓库列表获取失败: HTTP $HTTP_CODE"
    fi
}

# 测试 6: 获取单个仓库
test_get_repository() {
    log_info "测试 6: 获取单个仓库..."
    
    RESPONSE=$(http_request GET "${API_BASE}/repositories/${LOCAL_REPO_NAME}")
    HTTP_CODE=$(echo "$RESPONSE" | tail -n1)
    
    if [ "$HTTP_CODE" = "200" ]; then
        log_success "仓库详情获取成功"
    else
        log_error "仓库详情获取失败: HTTP $HTTP_CODE"
    fi
}

# 测试 7: 更新仓库
test_update_repository() {
    log_info "测试 7: 更新仓库..."
    
    RESPONSE=$(http_request PUT "${API_BASE}/repositories/${LOCAL_REPO_NAME}" '{
        "display_name": "Updated Local Repository",
        "description": "Updated description"
    }')
    
    HTTP_CODE=$(echo "$RESPONSE" | tail -n1)
    
    if [ "$HTTP_CODE" = "200" ]; then
        log_success "仓库更新成功"
    else
        log_error "仓库更新失败: HTTP $HTTP_CODE"
    fi
}

# 测试 8: 获取虚拟仓库成员
test_virtual_repo_members() {
    log_info "测试 8: 测试虚拟仓库成员管理..."
    
    RESPONSE=$(http_request GET "${API_BASE}/repositories/${VIRTUAL_REPO_NAME}/members")
    HTTP_CODE=$(echo "$RESPONSE" | tail -n1)
    
    if [ "$HTTP_CODE" = "200" ]; then
        log_success "虚拟仓库成员列表获取成功"
    else
        log_error "虚拟仓库成员列表获取失败: HTTP $HTTP_CODE"
    fi
}

# 测试 9: 错误场景 - 不存在仓库
test_nonexistent_repo() {
    log_info "测试 9: 测试不存在的仓库..."
    
    RESPONSE=$(http_request GET "${API_BASE}/repositories/nonexistent-repo")
    HTTP_CODE=$(echo "$RESPONSE" | tail -n1)
    
    if [ "$HTTP_CODE" = "404" ]; then
        log_success "不存在仓库正确返回 404"
    else
        log_error "不存在仓库未返回 404: HTTP $HTTP_CODE"
    fi
}

# 测试 10: 认证配置
test_auth_config() {
    log_info "测试 10: 测试认证配置..."
    
    RESPONSE=$(http_request POST "${API_BASE}/repositories" '{
        "name": "npm-proxy-auth",
        "type": "proxy",
        "package_type": "npm",
        "remote_url": "https://private.registry.com",
        "auth_type": "basic",
        "auth_config": "{\"username\":\"admin\",\"password\":\"secret\"}"
    }')
    
    HTTP_CODE=$(echo "$RESPONSE" | tail -n1)
    
    if [ "$HTTP_CODE" = "200" ] || [ "$HTTP_CODE" = "201" ]; then
        log_success "认证配置测试成功"
    else
        log_error "认证配置测试失败: HTTP $HTTP_CODE"
    fi
}

# 测试 11: 缓存配置
test_cache_config() {
    log_info "测试 11: 测试缓存配置..."
    
    RESPONSE=$(http_request POST "${API_BASE}/repositories" '{
        "name": "npm-proxy-cache",
        "type": "proxy",
        "package_type": "npm",
        "remote_url": "https://registry.npmjs.org",
        "cache_enabled": true,
        "cache_ttl_seconds": 7200,
        "cache_max_size_gb": 20
    }')
    
    HTTP_CODE=$(echo "$RESPONSE" | tail -n1)
    BODY=$(echo "$RESPONSE" | sed '$d')
    
    if [ "$HTTP_CODE" = "200" ] || [ "$HTTP_CODE" = "201" ]; then
        log_success "缓存配置测试成功"
    else
        log_error "缓存配置测试失败: HTTP $HTTP_CODE"
    fi
}

# 打印测试报告
print_report() {
    echo ""
    echo "====================================="
    echo "           测试报告"
    echo "====================================="
    echo -e "通过: ${GREEN}${PASS_COUNT}${NC}"
    echo -e "失败: ${RED}${FAIL_COUNT}${NC}"
    echo "====================================="
    
    if [ $FAIL_COUNT -eq 0 ]; then
        echo -e "${GREEN}所有测试通过！${NC}"
        exit 0
    else
        echo -e "${RED}有 ${FAIL_COUNT} 个测试失败${NC}"
        exit 1
    fi
}

# 清理测试数据
cleanup() {
    log_info "清理测试数据..."
    
    if [ -n "$AUTH_TOKEN" ]; then
        log_info "AUTH_TOKEN已设置，开始清理..."
        
        # 先删除虚拟仓库的成员关系
        curl -s -X DELETE "${API_BASE}/repositories/${VIRTUAL_REPO_NAME}/members/${LOCAL_REPO_NAME}" \
            -H "Authorization: Bearer ${AUTH_TOKEN}" > /dev/null 2>&1 || true
        curl -s -X DELETE "${API_BASE}/repositories/${VIRTUAL_REPO_NAME}/members/${PROXY_REPO_NAME}" \
            -H "Authorization: Bearer ${AUTH_TOKEN}" > /dev/null 2>&1 || true
        
        # 删除虚拟仓库
        curl -s -X DELETE "${API_BASE}/repositories/${VIRTUAL_REPO_NAME}" \
            -H "Authorization: Bearer ${AUTH_TOKEN}" > /dev/null 2>&1 || true
        
        # 删除其他测试仓库
        curl -s -X DELETE "${API_BASE}/repositories/${LOCAL_REPO_NAME}" \
            -H "Authorization: Bearer ${AUTH_TOKEN}" > /dev/null 2>&1 || true
        curl -s -X DELETE "${API_BASE}/repositories/${PROXY_REPO_NAME}" \
            -H "Authorization: Bearer ${AUTH_TOKEN}" > /dev/null 2>&1 || true
        curl -s -X DELETE "${API_BASE}/repositories/npm-proxy-auth" \
            -H "Authorization: Bearer ${AUTH_TOKEN}" > /dev/null 2>&1 || true
        curl -s -X DELETE "${API_BASE}/repositories/npm-proxy-cache" \
            -H "Authorization: Bearer ${AUTH_TOKEN}" > /dev/null 2>&1 || true
        
        log_info "清理完成"
    else
        log_info "AUTH_TOKEN未设置，跳过清理"
    fi
}

# 主函数
main() {
    echo "====================================="
    echo "     npm 仓库功能测试"
    echo "====================================="
    echo ""
    
    check_prerequisites
    authenticate
    cleanup
    
    test_create_local_repo
    test_create_proxy_repo
    test_create_virtual_repo
    test_get_package_metadata
    test_list_repositories
    test_get_repository
    test_update_repository
    test_virtual_repo_members
    test_nonexistent_repo
    test_auth_config
    test_cache_config
    
    print_report
}

# 运行测试
main "$@"
