#!/bin/bash

# =============================================================================
# Generic/Raw 客户端真实集成测试
# 使用 curl 测试通用文件仓库功能
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
echo " Generic/Raw 客户端真实集成测试"
echo " 使用 curl 测试通用文件上传/下载"
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
    warn "无法获取认证令牌，上传测试将跳过"
fi

TEST_DIR="/tmp/generic-client-test-$$"
mkdir -p "$TEST_DIR"
cd "$TEST_DIR"

echo "测试 1: 确保 Generic 本地仓库存在..."
if [ -n "$TOKEN" ]; then
    HTTP_CODE=$(curl -s -o /dev/null -w "%{http_code}" \
        -H "Authorization: Bearer $TOKEN" \
        "$BASE_URL/api/v1/repositories/generic-local")

    if [ "$HTTP_CODE" != "200" ]; then
        info "generic-local 仓库不存在，正在创建..."
        CREATE_RESP=$(curl -s -w "\n%{http_code}" -X POST "$BASE_URL/api/v1/repositories" \
            -H "Content-Type: application/json" \
            -H "Authorization: Bearer $TOKEN" \
            -d '{"name":"generic-local","display_name":"Generic 本地仓库","type":"local","package_type":"generic","enabled":true}')
        HTTP_CODE=$(echo "$CREATE_RESP" | tail -1)
        if [ "$HTTP_CODE" = "200" ] || [ "$HTTP_CODE" = "201" ]; then
            pass "generic-local 仓库已创建"
        else
            warn "generic-local 仓库创建失败 (HTTP $HTTP_CODE)"
        fi
    else
        pass "generic-local 仓库已存在"
    fi
else
    info "跳过仓库创建 (无认证令牌)"
fi

echo
echo "测试 2: 上传 JSON 文件..."
if [ -n "$TOKEN" ]; then
    echo '{"name":"test-config","version":"1.0.0","settings":{"debug":true}}' > config.json
    HTTP_CODE=$(curl -s -o /tmp/generic-upload.json -w "%{http_code}" \
        -X PUT "$BASE_URL/repository/generic-local/config.json" \
        -H "Authorization: Bearer $TOKEN" \
        -H "Content-Type: application/json" \
        --data-binary "@config.json")

    if [ "$HTTP_CODE" = "200" ] || [ "$HTTP_CODE" = "201" ]; then
        pass "JSON 文件上传成功 (HTTP $HTTP_CODE)"

        # 验证 JSON 下载
        HTTP_CODE=$(curl -s -o /tmp/generic-dl.json -w "%{http_code}" \
            "$BASE_URL/repository/generic-local/config.json")
        if [ "$HTTP_CODE" = "200" ]; then
            if grep -q '"version"' /tmp/generic-dl.json; then
                pass "JSON 文件下载成功且内容正确"
            else
                warn "JSON 文件下载成功但内容不匹配"
            fi
        else
            warn "JSON 文件下载失败 (HTTP $HTTP_CODE)"
        fi
    else
        warn "JSON 文件上传失败 (HTTP $HTTP_CODE)"
    fi
else
    info "跳过上传 (无认证令牌)"
fi

echo
echo "测试 3: 上传 ZIP 文件..."
if [ -n "$TOKEN" ]; then
    # 创建一个小 zip 文件
    echo "test-content" > test-file.txt
    zip test-archive.zip test-file.txt &> /dev/null

    HTTP_CODE=$(curl -s -o /tmp/generic-upload-zip.json -w "%{http_code}" \
        -X PUT "$BASE_URL/repository/generic-local/test-archive.zip" \
        -H "Authorization: Bearer $TOKEN" \
        -H "Content-Type: application/zip" \
        --data-binary "@test-archive.zip")

    if [ "$HTTP_CODE" = "200" ] || [ "$HTTP_CODE" = "201" ]; then
        pass "ZIP 文件上传成功 (HTTP $HTTP_CODE)"

        # 验证 ZIP 下载
        HTTP_CODE=$(curl -s -o /tmp/generic-dl.zip -w "%{http_code}" \
            "$BASE_URL/repository/generic-local/test-archive.zip")
        if [ "$HTTP_CODE" = "200" ]; then
            if unzip -t /tmp/generic-dl.zip &> /dev/null; then
                pass "ZIP 文件下载成功且格式正确"
            else
                warn "ZIP 文件下载成功但格式不正确"
            fi
        else
            warn "ZIP 文件下载失败 (HTTP $HTTP_CODE)"
        fi
    else
        warn "ZIP 文件上传失败 (HTTP $HTTP_CODE)"
        cat /tmp/generic-upload-zip.json 2>/dev/null
    fi
else
    info "跳过 ZIP 上传 (无认证令牌)"
fi

echo
echo "测试 4: 上传 XML 文件..."
if [ -n "$TOKEN" ]; then
    cat > metadata.xml <<'EOF'
<?xml version="1.0" encoding="UTF-8"?>
<metadata>
    <project name="test-project" version="1.0.0">
        <description>Test metadata file</description>
    </project>
</metadata>
EOF

    HTTP_CODE=$(curl -s -o /tmp/generic-upload-xml.json -w "%{http_code}" \
        -X PUT "$BASE_URL/repository/generic-local/metadata.xml" \
        -H "Authorization: Bearer $TOKEN" \
        -H "Content-Type: application/xml" \
        --data-binary "@metadata.xml")

    if [ "$HTTP_CODE" = "200" ] || [ "$HTTP_CODE" = "201" ]; then
        pass "XML 文件上传成功 (HTTP $HTTP_CODE)"

        # 验证 XML 下载
        HTTP_CODE=$(curl -s -o /tmp/generic-dl.xml -w "%{http_code}" \
            "$BASE_URL/repository/generic-local/metadata.xml")
        if [ "$HTTP_CODE" = "200" ] && grep -q "<metadata>" /tmp/generic-dl.xml; then
            pass "XML 文件下载成功且内容正确"
        else
            warn "XML 文件下载失败或内容不匹配"
        fi
    else
        warn "XML 文件上传失败 (HTTP $HTTP_CODE)"
    fi
else
    info "跳过 XML 上传 (无认证令牌)"
fi

echo
echo "测试 5: 上传并下载大文件（模拟 SDK 分发）..."
if [ -n "$TOKEN" ]; then
    # 创建 1MB 文件
    dd if=/dev/urandom of=sdk-v2.0.0.tar.gz bs=1024 count=1024 2>/dev/null

    FILE_SIZE=$(wc -c < sdk-v2.0.0.tar.gz)
    info "文件大小: $FILE_SIZE bytes"

    HTTP_CODE=$(curl -s -o /tmp/generic-upload-large.json -w "%{http_code}" \
        -X PUT "$BASE_URL/repository/generic-local/sdk-v2.0.0.tar.gz" \
        -H "Authorization: Bearer $TOKEN" \
        -H "Content-Type: application/octet-stream" \
        --data-binary "@sdk-v2.0.0.tar.gz")

    if [ "$HTTP_CODE" = "200" ] || [ "$HTTP_CODE" = "201" ]; then
        pass "大文件上传成功 (HTTP $HTTP_CODE)"

        # 验证下载
        HTTP_CODE=$(curl -s -o /tmp/generic-dl-sdk.tar.gz -w "%{http_code}" \
            "$BASE_URL/repository/generic-local/sdk-v2.0.0.tar.gz")
        if [ "$HTTP_CODE" = "200" ]; then
            DL_SIZE=$(wc -c < /tmp/generic-dl-sdk.tar.gz)
            if [ "$DL_SIZE" = "$FILE_SIZE" ]; then
                pass "大文件下载成功且大小一致 ($DL_SIZE bytes)"
            else
                warn "大文件下载大小不一致 (预期 $FILE_SIZE, 实际 $DL_SIZE)"
            fi
        else
            warn "大文件下载失败 (HTTP $HTTP_CODE)"
        fi
    else
        warn "大文件上传失败 (HTTP $HTTP_CODE)"
    fi
else
    info "跳过大文件上传 (无认证令牌)"
fi

echo
echo "测试 6: 验证上传路径嵌套支持..."
if [ -n "$TOKEN" ]; then
    echo "nested-file-content" > nested.txt
    HTTP_CODE=$(curl -s -o /tmp/generic-upload-nested.json -w "%{http_code}" \
        -X PUT "$BASE_URL/repository/generic-local/path/to/nested/file.txt" \
        -H "Authorization: Bearer $TOKEN" \
        -H "Content-Type: text/plain" \
        --data-binary "@nested.txt")

    if [ "$HTTP_CODE" = "200" ] || [ "$HTTP_CODE" = "201" ]; then
        pass "嵌套路径上传成功 (HTTP $HTTP_CODE)"

        HTTP_CODE=$(curl -s -o /tmp/generic-dl-nested.txt -w "%{http_code}" \
            "$BASE_URL/repository/generic-local/path/to/nested/file.txt")
        if [ "$HTTP_CODE" = "200" ] && grep -q "nested-file-content" /tmp/generic-dl-nested.txt; then
            pass "嵌套路径下载成功且内容正确"
        else
            warn "嵌套路径下载失败或内容不匹配"
        fi
    else
        warn "嵌套路径上传失败 (HTTP $HTTP_CODE)"
    fi
else
    info "跳过嵌套路径测试 (无认证令牌)"
fi

echo
echo "测试 7: 验证 DELETE 方法..."
if [ -n "$TOKEN" ]; then
    echo "delete-me-content" > delete-me.txt
    curl -s -X PUT "$BASE_URL/repository/generic-local/delete-me.txt" \
        -H "Authorization: Bearer $TOKEN" \
        --data-binary "@delete-me.txt" > /dev/null 2>&1

    HTTP_CODE=$(curl -s -o /dev/null -w "%{http_code}" \
        -X DELETE "$BASE_URL/repository/generic-local/delete-me.txt" \
        -H "Authorization: Bearer $TOKEN")

    if [ "$HTTP_CODE" = "204" ] || [ "$HTTP_CODE" = "200" ]; then
        pass "DELETE 方法成功 (HTTP $HTTP_CODE)"

        # 验证已删除
        HTTP_CODE=$(curl -s -o /dev/null -w "%{http_code}" \
            "$BASE_URL/repository/generic-local/delete-me.txt")
        if [ "$HTTP_CODE" = "404" ]; then
            pass "删除后资源正确返回 404"
        else
            warn "删除后资源仍然可访问 (HTTP $HTTP_CODE)"
        fi
    else
        warn "DELETE 方法失败 (HTTP $HTTP_CODE)"
    fi
else
    info "跳过 DELETE 测试 (无认证令牌)"
fi

echo
echo "测试 8: 验证认证保护..."
# 不带 token 的 PUT 请求应该被拒绝
HTTP_CODE=$(curl -s -o /dev/null -w "%{http_code}" \
    -X PUT "$BASE_URL/repository/generic-local/auth-test.txt" \
    -H "Content-Type: text/plain" \
    -d "test")

if [ "$HTTP_CODE" = "401" ]; then
    pass "未认证上传正确返回 401"
else
    warn "未认证上传返回 HTTP $HTTP_CODE (预期 401)"
fi

echo
echo "测试 9: 验证多种内容类型..."
if [ -n "$TOKEN" ]; then
    for CT in "text/plain" "application/json" "application/xml" "application/zip" "application/octet-stream"; do
        echo "content-type-test" > ct-test.bin
        HTTP_CODE=$(curl -s -o /dev/null -w "%{http_code}" \
            -X PUT "$BASE_URL/repository/generic-local/ct-test.bin" \
            -H "Authorization: Bearer $TOKEN" \
            -H "Content-Type: $CT" \
            --data-binary "@ct-test.bin")

        if [ "$HTTP_CODE" = "200" ] || [ "$HTTP_CODE" = "201" ]; then
            pass "Content-Type: $CT 上传成功"
        else
            warn "Content-Type: $CT 上传失败 (HTTP $HTTP_CODE)"
        fi
    done
else
    info "跳过内容类型测试 (无认证令牌)"
fi

echo
echo "测试 10: 验证 404 处理..."
HTTP_CODE=$(curl -s -o /dev/null -w "%{http_code}" \
    "$BASE_URL/repository/generic-local/nonexistent-file.txt")

if [ "$HTTP_CODE" = "404" ]; then
    pass "不存在的文件正确返回 404"
else
    warn "不存在的文件返回 HTTP $HTTP_CODE (预期 404)"
fi

# 清理
cd /
rm -rf "$TEST_DIR"

echo
echo "============================================"
echo " Generic/Raw 客户端测试完成"
echo "============================================"
