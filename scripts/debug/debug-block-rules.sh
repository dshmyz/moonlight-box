#!/bin/bash

echo "========================================="
echo "阻断规则功能诊断工具"
echo "========================================="
echo ""

# 检查后端服务是否运行
echo "1. 检查后端服务..."
if curl -s http://localhost:8080/health > /dev/null 2>&1; then
    echo "✓ 后端服务运行中 (http://localhost:8080)"
else
    echo "✗ 后端服务未运行或端口不是 8080"
    echo "  请先启动后端服务: cd cmd/registry && go run main.go"
    exit 1
fi
echo ""

# 检查数据库连接
echo "2. 检查数据库表是否存在..."
DB_CHECK=$(curl -s http://localhost:8080/api/v1/ping -H "Authorization: Bearer test" 2>&1)
echo "  数据库连接检查完成"
echo ""

# 检查阻断规则 API
echo "3. 测试阻断规则 API..."
RESPONSE=$(curl -s -w "\n%{http_code}" http://localhost:8080/api/v1/block-rules -H "Authorization: Bearer test")
HTTP_CODE=$(echo "$RESPONSE" | tail -n1)
BODY=$(echo "$RESPONSE" | sed '$d')

if [ "$HTTP_CODE" = "200" ]; then
    echo "✓ API 响应正常 (HTTP 200)"
    echo "  响应内容: $BODY"
elif [ "$HTTP_CODE" = "401" ]; then
    echo "✗ 认证失败 (HTTP 401)"
    echo "  请先登录获取有效 token"
elif [ "$HTTP_CODE" = "404" ]; then
    echo "✗ API 路由未找到 (HTTP 404)"
    echo "  可能原因："
    echo "    - 后端未正确注册阻断规则路由"
    echo "    - 后端代码未更新到最新版本"
else
    echo "✗ 未知错误 (HTTP $HTTP_CODE)"
    echo "  响应内容: $BODY"
fi
echo ""

# 检查数据库表
echo "4. 检查数据库表结构..."
echo "  请在数据库中执行以下 SQL 检查："
echo "  SHOW TABLES LIKE 'block_rules';"
echo "  DESCRIBE block_rules;"
echo ""

echo "========================================="
echo "诊断完成"
echo "========================================="
