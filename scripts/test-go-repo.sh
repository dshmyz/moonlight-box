#!/bin/bash

# Go 本地仓库测试脚本
# 用于测试本地 Go module proxy 的完整功能

set -e

# 颜色定义
RED='\033[0;31m'
GREEN='\033[0;32m'
YELLOW='\033[1;33m'
NC='\033[0m'

# 配置
REGISTRY_URL="${REGISTRY_URL:-http://localhost:9081}"
API_BASE="${REGISTRY_URL}/api/v1"
GO_PROXY="${REGISTRY_URL}/go/go-local"
PASS_COUNT=0
FAIL_COUNT=0
AUTH_TOKEN=""

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
    
    if ! command -v go &> /dev/null; then
        log_error "go 未安装"
        exit 1
    fi
    
    if ! command -v curl &> /dev/null; then
        log_error "curl 未安装"
        exit 1
    fi
    
    GO_VERSION=$(go version)
    log_info "Go 版本: $GO_VERSION"
    
    # 检查服务是否运行
    if ! curl -s "${REGISTRY_URL}/health" > /dev/null 2>&1; then
        log_error "Registry 服务未运行在 ${REGISTRY_URL}"
        exit 1
    fi
    
    log_success "前置条件检查通过"
}

# 测试 1: 创建本地 Go 仓库
test_create_local_repo() {
    log_info "测试 1: 创建本地 Go 仓库..."
    
    RESPONSE=$(http_request POST "${API_BASE}/repositories" '{
        "name": "go-local-test",
        "display_name": "Go Local Test Repository",
        "description": "Local Go module repository for testing",
        "type": "local",
        "package_type": "go",
        "enabled": true
    }')
    
    HTTP_CODE=$(echo "$RESPONSE" | tail -n1)
    BODY=$(echo "$RESPONSE" | sed '$d')
    
    if [ "$HTTP_CODE" = "200" ] || [ "$HTTP_CODE" = "201" ]; then
        log_success "本地 Go 仓库创建成功"
    else
        log_error "本地 Go 仓库创建失败: HTTP $HTTP_CODE - $BODY"
    fi
}

# 测试 2: 创建代理 Go 仓库
test_create_proxy_repo() {
    log_info "测试 2: 创建代理 Go 仓库..."
    
    RESPONSE=$(http_request POST "${API_BASE}/repositories" '{
        "name": "go-proxy-test",
        "display_name": "Go Proxy Test Repository",
        "description": "Proxy Go module repository for testing",
        "type": "proxy",
        "package_type": "go",
        "remote_url": "https://proxy.golang.org",
        "enabled": true,
        "cache_enabled": true,
        "cache_ttl_seconds": 3600
    }')
    
    HTTP_CODE=$(echo "$RESPONSE" | tail -n1)
    BODY=$(echo "$RESPONSE" | sed '$d')
    
    if [ "$HTTP_CODE" = "200" ] || [ "$HTTP_CODE" = "201" ]; then
        log_success "代理 Go 仓库创建成功"
    else
        log_error "代理 Go 仓库创建失败: HTTP $HTTP_CODE - $BODY"
    fi
}

# 测试 3: 创建虚拟 Go 仓库
test_create_virtual_repo() {
    log_info "测试 3: 创建虚拟 Go 仓库..."
    
    RESPONSE=$(http_request POST "${API_BASE}/repositories" '{
        "name": "go-virtual-test",
        "display_name": "Go Virtual Test Repository",
        "description": "Virtual Go module repository for testing",
        "type": "virtual",
        "package_type": "go",
        "enabled": true,
        "members": ["go-local-test", "go-proxy-test"]
    }')
    
    HTTP_CODE=$(echo "$RESPONSE" | tail -n1)
    BODY=$(echo "$RESPONSE" | sed '$d')
    
    if [ "$HTTP_CODE" = "200" ] || [ "$HTTP_CODE" = "201" ]; then
        log_success "虚拟 Go 仓库创建成功"
    else
        log_error "虚拟 Go 仓库创建失败: HTTP $HTTP_CODE - $BODY"
    fi
}

# 测试 4: 列出仓库
test_list_repositories() {
    log_info "测试 4: 列出仓库..."
    
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

# 测试 5: 获取单个仓库
test_get_repository() {
    log_info "测试 5: 获取单个仓库..."
    
    RESPONSE=$(http_request GET "${API_BASE}/repositories/go-local-test")
    HTTP_CODE=$(echo "$RESPONSE" | tail -n1)
    
    if [ "$HTTP_CODE" = "200" ]; then
        log_success "仓库详情获取成功"
    else
        log_error "仓库详情获取失败: HTTP $HTTP_CODE"
    fi
}

# 测试 6: 获取虚拟仓库成员
test_virtual_repo_members() {
    log_info "测试 6: 测试虚拟仓库成员管理..."
    
    RESPONSE=$(http_request GET "${API_BASE}/repositories/go-virtual-test/members")
    HTTP_CODE=$(echo "$RESPONSE" | tail -n1)
    
    if [ "$HTTP_CODE" = "200" ]; then
        log_success "虚拟仓库成员列表获取成功"
    else
        log_error "虚拟仓库成员列表获取失败: HTTP $HTTP_CODE"
    fi
}

# 测试 7: 测试 Go Proxy 路由响应
test_go_proxy_route() {
    log_info "测试 7: 测试 Go Proxy 路由..."
    
    # 测试通过 /go 路由访问 module（即使不存在，路由也应该被正确处理）
    MODULE_PATH=$(echo "example.com/test/module" | sed 's/\//%2F/g')
    
    # 构建 curl 命令
    CMD="curl -s -o /dev/null -w '%{http_code}'"
    if [ -n "$AUTH_TOKEN" ]; then
        CMD="$CMD -H 'Authorization: Bearer ${AUTH_TOKEN}'"
    fi
    CMD="$CMD '${REGISTRY_URL}/go/${MODULE_PATH}/@v/list'"
    
    HTTP_CODE=$(eval $CMD)
    
    # Go proxy 路由应该响应（即使模块不存在返回 404，也说明路由工作正常）
    if [ "$HTTP_CODE" = "404" ]; then
        log_success "Go proxy 路由响应正常（模块不存在返回 404）"
    elif [ "$HTTP_CODE" = "200" ]; then
        log_success "Go proxy 路由功能正常"
    else
        log_error "Go proxy 路由异常: HTTP $HTTP_CODE"
    fi
}

# 测试 8: 错误场景 - 不存在仓库
test_nonexistent_repo() {
    log_info "测试 8: 测试不存在的仓库..."
    
    RESPONSE=$(http_request GET "${API_BASE}/repositories/nonexistent-repo")
    HTTP_CODE=$(echo "$RESPONSE" | tail -n1)
    
    if [ "$HTTP_CODE" = "404" ]; then
        log_success "不存在仓库正确返回 404"
    else
        log_error "不存在仓库未返回 404: HTTP $HTTP_CODE"
    fi
}

# 测试 9: 更新仓库
test_update_repository() {
    log_info "测试 9: 更新仓库..."
    
    RESPONSE=$(http_request PUT "${API_BASE}/repositories/go-local-test" '{
        "display_name": "Updated Go Local Repository",
        "description": "Updated description"
    }')
    
    HTTP_CODE=$(echo "$RESPONSE" | tail -n1)
    
    if [ "$HTTP_CODE" = "200" ]; then
        log_success "仓库更新成功"
    else
        log_error "仓库更新失败: HTTP $HTTP_CODE"
    fi
}

# 打印测试报告
print_report() {
    echo ""
    echo "====================================="
    echo "           Go 仓库测试报告"
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
    
    # 删除测试仓库
    http_request DELETE "${API_BASE}/repositories/go-local-test" > /dev/null 2>&1
    http_request DELETE "${API_BASE}/repositories/go-proxy-test" > /dev/null 2>&1
    http_request DELETE "${API_BASE}/repositories/go-virtual-test" > /dev/null 2>&1
}

# 主函数
main() {
    echo "====================================="
    echo "     Go 仓库功能测试"
    echo "====================================="
    echo ""
    
    check_prerequisites
    authenticate
    cleanup
    
    test_create_local_repo
    test_create_proxy_repo
    test_create_virtual_repo
    test_list_repositories
    test_get_repository
    test_update_repository
    test_virtual_repo_members
    test_go_proxy_route
    test_nonexistent_repo
    
    print_report
}

# 运行测试
main "$@"
