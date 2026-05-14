#!/bin/bash

API_BASE="http://localhost:8080/api/v1"
TOKEN="your-token-here"

echo "=== 虚拟仓库成员配置测试 ==="
echo ""

echo "1. 创建本地仓库"
curl -X POST "${API_BASE}/repositories" \
  -H "Content-Type: application/json" \
  -H "Authorization: Bearer ${TOKEN}" \
  -d '{
    "name": "npm-local-test",
    "type": "local",
    "package_type": "npm",
    "enabled": true
  }' | jq .
echo ""

echo "2. 创建代理仓库"
curl -X POST "${API_BASE}/repositories" \
  -H "Content-Type: application/json" \
  -H "Authorization: Bearer ${TOKEN}" \
  -d '{
    "name": "npm-proxy-test",
    "type": "proxy",
    "package_type": "npm",
    "remote_url": "https://registry.npmjs.org",
    "enabled": true
  }' | jq .
echo ""

echo "3. 创建虚拟仓库（不带成员）"
curl -X POST "${API_BASE}/repositories" \
  -H "Content-Type: application/json" \
  -H "Authorization: Bearer ${TOKEN}" \
  -d '{
    "name": "npm-virtual-test",
    "type": "virtual",
    "package_type": "npm",
    "enabled": true
  }' | jq .
echo ""

echo "4. 向虚拟仓库添加成员（优先级 0）"
curl -X POST "${API_BASE}/repositories/npm-virtual-test/members" \
  -H "Content-Type: application/json" \
  -H "Authorization: Bearer ${TOKEN}" \
  -d '{
    "member_name": "npm-local-test",
    "priority": 0
  }' | jq .
echo ""

echo "5. 向虚拟仓库添加成员（优先级 1）"
curl -X POST "${API_BASE}/repositories/npm-virtual-test/members" \
  -H "Content-Type: application/json" \
  -H "Authorization: Bearer ${TOKEN}" \
  -d '{
    "member_name": "npm-proxy-test",
    "priority": 1
  }' | jq .
echo ""

echo "6. 获取虚拟仓库成员列表"
curl -X GET "${API_BASE}/repositories/npm-virtual-test/members" \
  -H "Authorization: Bearer ${TOKEN}" | jq .
echo ""

echo "7. 移除成员仓库"
curl -X DELETE "${API_BASE}/repositories/npm-virtual-test/members/npm-proxy-test" \
  -H "Authorization: Bearer ${TOKEN}" | jq .
echo ""

echo "8. 再次查看成员列表"
curl -X GET "${API_BASE}/repositories/npm-virtual-test/members" \
  -H "Authorization: Bearer ${TOKEN}" | jq .
echo ""

echo "=== 测试完成 ==="
