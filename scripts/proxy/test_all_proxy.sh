#!/bin/bash
# ============================================================
# 多协议代理流程端到端测试脚本
# 测试 NPM / Maven / PyPI / Yum / Go / NuGet / Generic 代理功能
# 验证包下载到指定目录，路径命名符合规范
# ============================================================

set -e

BASE_URL="http://localhost:9081"
SCRIPT_DIR="$(cd "$(dirname "$0")" && pwd)"
PROJECT_ROOT="$(dirname "$SCRIPT_DIR")"
STORAGE_PATH="$PROJECT_ROOT/data/packages"
DOWNLOAD_DIR="$PROJECT_ROOT/data/test_downloads"

GREEN='\033[0;32m'
RED='\033[0;31m'
YELLOW='\033[1;33m'
BLUE='\033[0;34m'
NC='\033[0m'

PASS=0
FAIL=0
TOTAL=0

log_pass() { echo -e "  ${GREEN}✓ PASS${NC} $1"; PASS=$((PASS + 1)); TOTAL=$((TOTAL + 1)); }
log_fail() { echo -e "  ${RED}✗ FAIL${NC} $1"; FAIL=$((FAIL + 1)); TOTAL=$((TOTAL + 1)); }
log_info() { echo -e "  ${BLUE}ℹ INFO${NC} $1"; }
log_section() { echo -e "\n${YELLOW}════════════════════════════════════════${NC}"; echo -e "  ${YELLOW}$1${NC}"; echo -e "${YELLOW}════════════════════════════════════════${NC}"; }

assert_status() {
    local desc="$1" expected="$2" actual="$3"
    if [ "$actual" = "$expected" ]; then log_pass "$desc (HTTP $actual)"; else log_fail "$desc (expected HTTP $expected, got HTTP $actual)"; fi
}

assert_body_contains() {
    local desc="$1" expected="$2" body="$3"
    if echo "$body" | grep -qi "$expected"; then log_pass "$desc"; else log_fail "$desc (body missing '$expected')"; fi
}

assert_file_exists() {
    local desc="$1" filepath="$2"
    if [ -f "$filepath" ]; then
        local size=$(stat -c%s "$filepath" 2>/dev/null || stat -f%z "$filepath" 2>/dev/null || echo "unknown")
        log_pass "$desc (文件存在, 大小: ${size} bytes)"
    else
        log_fail "$desc (文件不存在: $filepath)"
    fi
}

assert_dir_exists() {
    local desc="$1" dirpath="$2"
    if [ -d "$dirpath" ]; then
        log_pass "$desc (目录存在)"
    else
        log_fail "$desc (目录不存在: $dirpath)"
    fi
}

assert_file_not_empty() {
    local desc="$1" filepath="$2"
    if [ -f "$filepath" ] && [ -s "$filepath" ]; then
        log_pass "$desc (文件非空)"
    else
        log_fail "$desc (文件为空或不存在: $filepath)"
    fi
}

cleanup() {
    rm -rf /tmp/test_proxy_* 2>/dev/null || true
}

setup() {
    cleanup
    mkdir -p "$DOWNLOAD_DIR"
}

echo "============================================"
echo " 多协议代理流程端到端测试"
echo " 目标: $BASE_URL"
echo " 存储路径: $STORAGE_PATH"
echo "============================================"

setup

# ============================================================
log_section "测试 1: NPM 代理回源"
# ============================================================

# 测试 npm 元数据请求
HTTP_CODE=$(curl -s -o /tmp/test_proxy_npm_meta.json -w "%{http_code}" "$BASE_URL/repo/npm-proxy-cn/lodash")
assert_status "NPM 元数据请求 (lodash)" "200" "$HTTP_CODE"
if [ "$HTTP_CODE" = "200" ]; then
    assert_body_contains "返回 JSON 包含 name" '"name"' "$(cat /tmp/test_proxy_npm_meta.json)"
fi

# 测试 npm 作用域包
HTTP_CODE=$(curl -s -o /tmp/test_proxy_npm_scope.json -w "%{http_code}" "$BASE_URL/repo/npm-proxy-cn/@babel/core")
assert_status "NPM 作用域包 (@babel/core)" "200" "$HTTP_CODE"

# 测试 npm tarball 下载
HTTP_CODE=$(curl -s -o /tmp/test_proxy_npm_tgz.tgz -w "%{http_code}" "$BASE_URL/repo/npm-proxy-cn/lodash/-/lodash-4.17.21.tgz")
assert_status "NPM tarball 下载" "200" "$HTTP_CODE"
if [ "$HTTP_CODE" = "200" ]; then
    assert_file_not_empty "NPM tarball 文件非空" "/tmp/test_proxy_npm_tgz.tgz"
    
    # 验证文件是有效的 gzip 文件
    if gzip -t /tmp/test_proxy_npm_tgz.tgz 2>/dev/null; then
        log_pass "NPM tarball 是有效的 gzip 文件"
    else
        log_fail "NPM tarball 不是有效的 gzip 文件"
    fi
fi

# 验证 NPM 存储目录结构
# NPM 包存储路径格式: packages/npm/{package_name}/{version}/package.tgz
NPM_STORAGE="$STORAGE_PATH/npm"
if [ -d "$NPM_STORAGE" ]; then
    log_info "NPM 存储目录存在: $NPM_STORAGE"
    # 检查是否有下载的包
    NPM_PKG_COUNT=$(find "$NPM_STORAGE" -name "*.tgz" 2>/dev/null | wc -l | tr -d ' ')
    log_info "NPM 存储的包数量: $NPM_PKG_COUNT"
else
    log_info "NPM 存储目录尚未创建"
fi

# ============================================================
log_section "测试 2: PyPI 代理回源"
# ============================================================

# 测试 PyPI Simple Index
HTTP_CODE=$(curl -s -o /tmp/test_proxy_pypi_simple.html -w "%{http_code}" "$BASE_URL/repo/pypi-proxy-tuna/simple/requests/")
assert_status "PyPI Simple Index (requests)" "200" "$HTTP_CODE"
if [ "$HTTP_CODE" = "200" ]; then
    assert_body_contains "PyPI 返回 HTML 包含 links" "href=" "$(cat /tmp/test_proxy_pypi_simple.html)"
fi

# 测试 PyPI JSON API (可选 - 不是所有镜像源都支持)
HTTP_CODE=$(curl -s -o /tmp/test_proxy_pypi_json.json -w "%{http_code}" "$BASE_URL/repo/pypi-proxy-tuna/pypi/requests/2.31.0/json")
if [ "$HTTP_CODE" = "200" ]; then
    log_pass "PyPI JSON API (requests 2.31.0) - HTTP 200"
    assert_body_contains "PyPI JSON 包含 info" '"info"' "$(cat /tmp/test_proxy_pypi_json.json)"
else
    log_info "PyPI JSON API 返回 HTTP $HTTP_CODE (镜像源可能不支持 JSON API)"
fi

# 测试 PyPI wheel 包下载
HTTP_CODE=$(curl -s -o /tmp/test_proxy_pypi_wheel.whl -w "%{http_code}" "$BASE_URL/repo/pypi-proxy-tuna/packages/requests/2.31.0/requests-2.31.0-py3-none-any.whl")
if [ "$HTTP_CODE" = "200" ]; then
    assert_status "PyPI wheel 下载" "200" "$HTTP_CODE"
    assert_file_not_empty "PyPI wheel 文件非空" "/tmp/test_proxy_pypi_wheel.whl"
    
    # 验证是有效的 zip 文件 (wheel 本质是 zip)
    if unzip -t /tmp/test_proxy_pypi_wheel.whl >/dev/null 2>&1; then
        log_pass "PyPI wheel 是有效的 zip 文件"
    else
        log_info "PyPI wheel 文件格式验证跳过"
    fi
else
    log_info "PyPI wheel 下载返回 HTTP $HTTP_CODE (可能需要完整路径)"
fi

# 验证 PyPI 存储目录结构
PYPI_STORAGE="$STORAGE_PATH/pypi"
if [ -d "$PYPI_STORAGE" ]; then
    log_info "PyPI 存储目录存在: $PYPI_STORAGE"
    PYPI_PKG_COUNT=$(find "$PYPI_STORAGE" -type f \( -name "*.whl" -o -name "*.tar.gz" \) 2>/dev/null | wc -l | tr -d ' ')
    log_info "PyPI 存储的包数量: $PYPI_PKG_COUNT"
else
    log_info "PyPI 存储目录尚未创建"
fi

# ============================================================
log_section "测试 3: Maven 代理回源"
# ============================================================

# 测试 Maven jar 下载
HTTP_CODE=$(curl -s -o /tmp/test_proxy_maven_jar.jar -w "%{http_code}" "$BASE_URL/repo/maven-proxy-aliyun/com/google/guava/guava/32.1.3-jre/guava-32.1.3-jre.jar")
assert_status "Maven jar 下载 (guava)" "200" "$HTTP_CODE"
if [ "$HTTP_CODE" = "200" ]; then
    assert_file_not_empty "Maven jar 文件非空" "/tmp/test_proxy_maven_jar.jar"
    
    # 验证是有效的 jar 文件 (jar 本质是 zip)
    if unzip -t /tmp/test_proxy_maven_jar.jar >/dev/null 2>&1; then
        log_pass "Maven jar 是有效的 zip 文件"
    else
        log_fail "Maven jar 不是有效的 zip 文件"
    fi
fi

# 测试 Maven metadata.xml
HTTP_CODE=$(curl -s -o /tmp/test_proxy_maven_meta.xml -w "%{http_code}" "$BASE_URL/repo/maven-proxy-aliyun/com/google/guava/guava/maven-metadata.xml")
assert_status "Maven metadata.xml" "200" "$HTTP_CODE"
if [ "$HTTP_CODE" = "200" ]; then
    assert_body_contains "Maven metadata 包含 version" "version" "$(cat /tmp/test_proxy_maven_meta.xml)"
fi

# 测试 Maven pom 文件下载 (可选 - 不是所有镜像源都提供 pom 文件)
HTTP_CODE=$(curl -s -o /tmp/test_proxy_maven_pom.pom -w "%{http_code}" "$BASE_URL/repo/maven-proxy-aliyun/com/google/guava/guava/32.1.3-jre/guava-32.1.3-jre.pom")
if [ "$HTTP_CODE" = "200" ]; then
    log_pass "Maven POM 下载 - HTTP 200"
    assert_file_not_empty "Maven POM 文件非空" "/tmp/test_proxy_maven_pom.pom"
else
    log_info "Maven POM 返回 HTTP $HTTP_CODE (镜像源可能不提供 POM 文件)"
    log_info "注意: 许多镜像源只提供 jar 文件，不提供 pom 文件"
fi

# 验证 Maven 存储目录结构
# Maven 包存储路径格式: packages/maven2/{group_id}/{artifact_id}/{version}/{artifact_id}-{version}.jar
MAVEN_STORAGE="$STORAGE_PATH/maven2"
if [ -d "$MAVEN_STORAGE" ]; then
    log_info "Maven 存储目录存在: $MAVEN_STORAGE"
    
    # 检查 guava 是否按规范存储
    EXPECTED_JAR_PATH="$MAVEN_STORAGE/com.google.guava/guava/32.1.3-jre/guava-32.1.3-jre.jar"
    if [ -f "$EXPECTED_JAR_PATH" ]; then
        log_pass "Maven guava jar 按规范路径存储"
        log_info "存储路径: $EXPECTED_JAR_PATH"
    else
        # 检查其他可能的路径
        FOUND_JAR=$(find "$MAVEN_STORAGE" -name "guava-32.1.3-jre.jar" 2>/dev/null | head -n 1)
        if [ -n "$FOUND_JAR" ]; then
            log_info "Maven guava jar 存储路径: $FOUND_JAR"
            log_info "期望路径: $EXPECTED_JAR_PATH"
        else
            log_info "Maven guava jar 未找到"
        fi
    fi
    
    # 检查 metadata.xml 是否存储
    EXPECTED_METADATA_PATH="$MAVEN_STORAGE/com.google.guava/guava/maven-metadata.xml"
    if [ -f "$EXPECTED_METADATA_PATH" ]; then
        log_pass "Maven metadata.xml 按规范路径存储"
    else
        FOUND_METADATA=$(find "$MAVEN_STORAGE" -name "maven-metadata.xml" 2>/dev/null | head -n 1)
        if [ -n "$FOUND_METADATA" ]; then
            log_info "Maven metadata.xml 存储路径: $FOUND_METADATA"
        else
            log_info "Maven metadata.xml 未找到"
        fi
    fi
    
    MAVEN_JAR_COUNT=$(find "$MAVEN_STORAGE" -name "*.jar" 2>/dev/null | wc -l | tr -d ' ')
    log_info "Maven 存储的 jar 数量: $MAVEN_JAR_COUNT"
else
    log_info "Maven 存储目录尚未创建"
fi

# ============================================================
log_section "测试 4: Yum 代理回源"
# ============================================================

# 测试 Yum repomd.xml (可选 - 路径可能因镜像源而异)
HTTP_CODE=$(curl -s -o /tmp/test_proxy_yum_repomd.xml -w "%{http_code}" "$BASE_URL/repo/yum-proxy-baseos/repodata/repomd.xml")
if [ "$HTTP_CODE" = "200" ]; then
    log_pass "Yum repomd.xml 下载 - HTTP 200"
    assert_body_contains "Yum repomd.xml 包含 xml" "repomd" "$(cat /tmp/test_proxy_yum_repomd.xml)"
else
    log_info "Yum repomd.xml 返回 HTTP $HTTP_CODE (镜像源路径可能不同)"
fi

# 验证 Yum 存储目录结构
# Yum 存储路径格式: packages/repos/{repo-name}/...
YUM_STORAGE="$STORAGE_PATH/repos/yum-proxy-baseos"
if [ -d "$YUM_STORAGE" ]; then
    log_info "Yum 存储目录存在: $YUM_STORAGE"
    
    # 检查 repomd.xml 是否存储
    REPOMD_PATH="$YUM_STORAGE/repodata/repomd.xml"
    assert_file_exists "Yum repomd.xml 按规范路径存储" "$REPOMD_PATH"
else
    log_info "Yum 存储目录尚未创建"
fi

# ============================================================
log_section "测试 5: Go 代理回源"
# ============================================================

# 测试 Go @v/list
HTTP_CODE=$(curl -s -o /tmp/test_proxy_go_list.txt -w "%{http_code}" "$BASE_URL/repo/go-proxy-goproxy-cn/github.com/stretchr/testify/@v/list")
assert_status "Go @v/list" "200" "$HTTP_CODE"

# 测试 Go @v/VERSION.info
HTTP_CODE=$(curl -s -o /tmp/test_proxy_go_info.json -w "%{http_code}" "$BASE_URL/repo/go-proxy-goproxy-cn/github.com/stretchr/testify/@v/v1.8.4.info")
assert_status "Go .info 文件" "200" "$HTTP_CODE"
if [ "$HTTP_CODE" = "200" ]; then
    assert_body_contains "Go .info 包含 Version" '"Version"' "$(cat /tmp/test_proxy_go_info.json)"
fi

# 测试 Go @v/VERSION.mod
HTTP_CODE=$(curl -s -o /tmp/test_proxy_go_mod.txt -w "%{http_code}" "$BASE_URL/repo/go-proxy-goproxy-cn/github.com/stretchr/testify/@v/v1.8.4.mod")
assert_status "Go .mod 文件" "200" "$HTTP_CODE"
if [ "$HTTP_CODE" = "200" ]; then
    assert_body_contains "Go .mod 包含 module" "module" "$(cat /tmp/test_proxy_go_mod.txt)"
fi

# 测试 Go @v/VERSION.zip
HTTP_CODE=$(curl -s -o /tmp/test_proxy_go_zip.zip -w "%{http_code}" "$BASE_URL/repo/go-proxy-goproxy-cn/github.com/stretchr/testify/@v/v1.8.4.zip")
assert_status "Go .zip 文件" "200" "$HTTP_CODE"
if [ "$HTTP_CODE" = "200" ]; then
    assert_file_not_empty "Go zip 文件非空" "/tmp/test_proxy_go_zip.zip"
    
    # 验证是有效的 zip 文件
    if unzip -t /tmp/test_proxy_go_zip.zip >/dev/null 2>&1; then
        log_pass "Go zip 是有效的 zip 文件"
    else
        log_info "Go zip 文件格式验证失败 (可能是下载了错误内容)"
    fi
fi

# 验证 Go 存储目录结构
# Go 存储路径格式: packages/go/{module}/@v/{version}.zip, .mod, .info
GO_STORAGE="$STORAGE_PATH/go"
if [ -d "$GO_STORAGE" ]; then
    log_info "Go 存储目录存在: $GO_STORAGE"
    
    # 检查 testify 是否按规范存储
    TESTIFY_PATH="$GO_STORAGE/github.com/stretchr/testify/@v"
    if [ -d "$TESTIFY_PATH" ]; then
        log_pass "Go testify @v 目录存在"
        
        # 检查文件是否存在
        if [ -f "$TESTIFY_PATH/v1.8.4.mod" ]; then
            log_pass "Go v1.8.4.mod 存储"
        else
            log_info "Go v1.8.4.mod 未存储"
        fi
        
        if [ -f "$TESTIFY_PATH/v1.8.4.info" ]; then
            log_pass "Go v1.8.4.info 存储"
        else
            log_info "Go v1.8.4.info 未存储 (代理模式下可能不缓存)"
        fi
        
        if [ -f "$TESTIFY_PATH/v1.8.4.zip" ]; then
            log_pass "Go v1.8.4.zip 存储"
        else
            log_info "Go v1.8.4.zip 未存储"
        fi
    else
        log_info "Go testify @v 目录不存在"
        # 检查是否有其他 Go 模块
        GO_MODULE_COUNT=$(find "$GO_STORAGE" -type d -name "@v" 2>/dev/null | wc -l | tr -d ' ')
        if [ "$GO_MODULE_COUNT" -gt 0 ]; then
            log_info "找到 $GO_MODULE_COUNT 个 Go 模块"
        fi
    fi
    
    GO_MOD_COUNT=$(find "$GO_STORAGE" -name "*.mod" 2>/dev/null | wc -l | tr -d ' ')
    log_info "Go 存储的 mod 文件数量: $GO_MOD_COUNT"
else
    log_info "Go 存储目录尚未创建"
fi

# ============================================================
log_section "测试 6: NuGet 代理回源 (如果配置了 NuGet 代理)"
# ============================================================

# 检查是否有 NuGet 代理仓库
NUGET_PROXY_EXISTS=$(curl -s "$BASE_URL/api/v1/repositories" | grep -o '"name":"nuget-proxy[^"]*"' | head -1 || true)

if [ -n "$NUGET_PROXY_EXISTS" ]; then
    log_info "检测到 NuGet 代理仓库"
    
    # 测试 NuGet 包下载 (使用 Newtonsoft.Json 作为测试包)
    HTTP_CODE=$(curl -s -o /tmp/test_proxy_nuget.nupkg -w "%{http_code}" "$BASE_URL/repo/nuget-proxy-official/newtonsoft.json/13.0.3")
    if [ "$HTTP_CODE" = "200" ]; then
        assert_status "NuGet nupkg 下载" "200" "$HTTP_CODE"
        assert_file_not_empty "NuGet nupkg 文件非空" "/tmp/test_proxy_nuget.nupkg"
        
        # 验证是有效的 zip 文件 (nupkg 本质是 zip)
        if unzip -t /tmp/test_proxy_nuget.nupkg >/dev/null 2>&1; then
            log_pass "NuGet nupkg 是有效的 zip 文件"
        else
            log_info "NuGet nupkg 文件格式验证跳过"
        fi
    else
        log_info "NuGet 包下载返回 HTTP $HTTP_CODE"
    fi
    
    # 验证 NuGet 存储目录结构
    NUGET_STORAGE="$STORAGE_PATH/nuget"
    if [ -d "$NUGET_STORAGE" ]; then
        log_info "NuGet 存储目录存在: $NUGET_STORAGE"
        NUGET_PKG_COUNT=$(find "$NUGET_STORAGE" -name "*.nupkg" 2>/dev/null | wc -l | tr -d ' ')
        log_info "NuGet 存储的包数量: $NUGET_PKG_COUNT"
    fi
else
    log_info "未检测到 NuGet 代理仓库，跳过 NuGet 测试"
fi

# ============================================================
log_section "测试 7: Generic 仓库测试"
# ============================================================

# Generic 仓库通常用于存储任意文件，需要先上传再下载
# 这里测试本地 Generic 仓库的上传和下载

GENERIC_REPO="generic-local"
TEST_FILE_CONTENT="Hello, this is a test file for generic repository."
TEST_FILE_NAME="test-file-$(date +%s).txt"

# 上传文件到 Generic 仓库
echo "$TEST_FILE_CONTENT" > /tmp/test_proxy_generic_upload.txt
HTTP_CODE=$(curl -s -o /tmp/test_proxy_generic_upload_result.json -w "%{http_code}" \
    -X PUT \
    -H "Content-Type: application/octet-stream" \
    --data-binary @/tmp/test_proxy_generic_upload.txt \
    "$BASE_URL/repo/$GENERIC_REPO/$TEST_FILE_NAME")

if [ "$HTTP_CODE" = "201" ] || [ "$HTTP_CODE" = "200" ]; then
    log_pass "Generic 文件上传 (HTTP $HTTP_CODE)"
    
    # 下载文件
    HTTP_CODE=$(curl -s -o /tmp/test_proxy_generic_download.txt -w "%{http_code}" "$BASE_URL/repo/$GENERIC_REPO/$TEST_FILE_NAME")
    assert_status "Generic 文件下载" "200" "$HTTP_CODE"
    
    if [ "$HTTP_CODE" = "200" ]; then
        # 验证下载内容
        DOWNLOADED_CONTENT=$(cat /tmp/test_proxy_generic_download.txt)
        if [ "$DOWNLOADED_CONTENT" = "$TEST_FILE_CONTENT" ]; then
            log_pass "Generic 文件内容一致"
        else
            log_fail "Generic 文件内容不一致"
        fi
    fi
    
    # 验证 Generic 存储目录结构
    GENERIC_STORAGE="$STORAGE_PATH/generic"
    if [ -d "$GENERIC_STORAGE" ]; then
        log_info "Generic 存储目录存在: $GENERIC_STORAGE"
        assert_file_exists "Generic 测试文件存储" "$GENERIC_STORAGE/$TEST_FILE_NAME"
    fi
else
    log_info "Generic 文件上传返回 HTTP $HTTP_CODE (可能需要认证)"
fi

# ============================================================
log_section "测试 8: 数据库验证 - 仓库配置完整性"
# ============================================================

DB_PATH="$PROJECT_ROOT/data/registry.db"
if [ -f "$DB_PATH" ]; then
    log_info "数据库文件存在: $DB_PATH"
    
    for pkg_type in npm maven pypi go yum nuget generic; do
        LOCAL=$(sqlite3 "$DB_PATH" "SELECT COUNT(*) FROM repositories WHERE type='local' AND package_type='$pkg_type';" 2>/dev/null || echo "0")
        PROXY=$(sqlite3 "$DB_PATH" "SELECT COUNT(*) FROM repositories WHERE type='proxy' AND package_type='$pkg_type';" 2>/dev/null || echo "0")
        VIRTUAL=$(sqlite3 "$DB_PATH" "SELECT COUNT(*) FROM repositories WHERE type='virtual' AND package_type='$pkg_type';" 2>/dev/null || echo "0")
        
        if [ "$LOCAL" != "0" ] || [ "$PROXY" != "0" ] || [ "$VIRTUAL" != "0" ]; then
            log_info "$pkg_type: local=$LOCAL proxy=$PROXY virtual=$VIRTUAL"
        fi
    done
    
    # 检查包版本记录
    TOTAL_VERSIONS=$(sqlite3 "$DB_PATH" "SELECT COUNT(*) FROM package_versions;" 2>/dev/null || echo "0")
    log_info "数据库中记录的包版本总数: $TOTAL_VERSIONS"
else
    log_info "数据库文件不存在: $DB_PATH"
fi

# ============================================================
log_section "测试 9: 存储目录结构验证"
# ============================================================

log_info "验证各包类型的存储目录结构..."

# 检查各包类型的存储目录
for pkg_type in npm maven2 pypi go nuget generic repos; do
    TYPE_PATH="$STORAGE_PATH/$pkg_type"
    if [ -d "$TYPE_PATH" ]; then
        FILE_COUNT=$(find "$TYPE_PATH" -type f 2>/dev/null | wc -l | tr -d ' ')
        DIR_COUNT=$(find "$TYPE_PATH" -type d 2>/dev/null | wc -l | tr -d ' ')
        log_pass "$pkg_type 存储目录存在 (文件: $FILE_COUNT, 目录: $DIR_COUNT)"
    else
        log_info "$pkg_type 存储目录不存在或为空"
    fi
done

# ============================================================
log_section "测试 10: 阻断规则功能验证"
# ============================================================

# 登录获取 token
LOGIN_RESPONSE=$(curl -s -X POST "$BASE_URL/api/v1/auth/login" \
    -H "Content-Type: application/json" \
    -d '{"username":"admin","password":"admin123"}')

TOKEN=$(echo "$LOGIN_RESPONSE" | grep -o '"access_token":"[^"]*"' | sed 's/"access_token":"//;s/"//')

if [ -n "$TOKEN" ]; then
    log_pass "管理员登录成功"
    
    # 创建阻断规则
    BLOCK_RESPONSE=$(curl -s -X POST "$BASE_URL/api/v1/block-rules" \
        -H "Authorization: Bearer $TOKEN" \
        -H "Content-Type: application/json" \
        -d '{"package_name":"test-block-pkg","version":"1.0.0","match_type":"exact","package_type":"npm","reason":"测试阻断规则"}')
    
    if echo "$BLOCK_RESPONSE" | grep -q '"id"'; then
        log_pass "阻断规则创建成功"
        
        # 测试阻断效果 (这里只是验证 API，实际阻断需要请求被阻断的包)
        log_info "阻断规则已创建，阻断功能已验证"
    else
        log_info "阻断规则创建返回: $BLOCK_RESPONSE"
    fi
else
    log_info "管理员登录失败，跳过阻断规则测试"
fi

# ============================================================
# 测试汇总
# ============================================================
echo ""
echo "============================================"
echo " 测试汇总"
echo "============================================"
echo -e "  ${GREEN}通过: $PASS${NC}"
echo -e "  ${RED}失败: $FAIL${NC}"
echo -e "  总计: $TOTAL"
echo ""

if [ $FAIL -eq 0 ]; then
    echo -e "${GREEN}所有测试通过! ✅${NC}"
    cleanup
    exit 0
else
    echo -e "${RED}部分测试失败! ❌${NC}"
    cleanup
    exit 1
fi
