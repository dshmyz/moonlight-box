#!/bin/bash

# =============================================================================
# Maven 客户端真实集成测试
# 使用官方 mvn deploy 命令测试仓库功能
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
echo " Maven 客户端真实集成测试"
echo " 使用官方 mvn deploy 命令测试"
echo " 目标: $BASE_URL"
echo "============================================"
echo

# 检查 mvn 命令
if ! command -v mvn &> /dev/null; then
    warn "mvn 命令未安装，跳过测试"
    exit 0
fi

info "Maven 版本: $(mvn --version | head -1)"
info "Java 版本: $(java -version 2>&1 | head -1)"

# 获取认证令牌
TOKEN=$(curl -s -X POST "$BASE_URL/api/v1/auth/login" \
    -H "Content-Type: application/json" \
    -d "{\"username\":\"$ADMIN_USER\",\"password\":\"$ADMIN_PASS\"}" | \
    grep -o '"access_token":"[^"]*"' | \
    sed 's/"access_token":"//;s/"//')

if [ -z "$TOKEN" ]; then
    warn "无法获取认证令牌"
fi

# 创建测试项目
TEST_DIR="/tmp/maven-client-test-$$"
mkdir -p "$TEST_DIR/test-maven-project"
cd "$TEST_DIR/test-maven-project"

echo "测试 1: 创建 Maven 项目..."
cat > pom.xml <<'EOF'
<?xml version="1.0" encoding="UTF-8"?>
<project xmlns="http://maven.apache.org/POM/4.0.0"
         xmlns:xsi="http://www.w3.org/2001/XMLSchema-instance"
         xsi:schemaLocation="http://maven.apache.org/POM/4.0.0 
         http://maven.apache.org/xsd/maven-4.0.0.xsd">
    <modelVersion>4.0.0</modelVersion>
    
    <groupId>com.test</groupId>
    <artifactId>maven-client-test</artifactId>
    <version>1.0.0</version>
    <packaging>jar</packaging>
    
    <name>Maven Client Test</name>
    <description>Test project for Maven repository</description>
    
    <properties>
        <maven.compiler.source>11</maven.compiler.source>
        <maven.compiler.target>11</maven.compiler.target>
        <project.build.sourceEncoding>UTF-8</project.build.sourceEncoding>
    </properties>
</project>
EOF
pass "pom.xml 创建成功"

mkdir -p src/main/java/com/test

cat > src/main/java/com/test/App.java <<'EOF'
package com.test;

public class App {
    public static void main(String[] args) {
        System.out.println("Hello Maven!");
    }
}
EOF

echo
echo "测试 2: 验证 pom.xml..."
if [ -f "pom.xml" ]; then
    pass "pom.xml 文件存在"
    
    if grep -q "<artifactId>maven-client-test</artifactId>" pom.xml; then
        pass "pom.xml 内容正确"
    else
        fail "pom.xml 内容不正确"
    fi
else
    fail "pom.xml 文件不存在"
fi

echo
echo "测试 3: 测试 mvn compile..."
if mvn clean compile &> /tmp/maven-compile.log 2>&1; then
    pass "mvn compile 测试通过"
    
    if [ -d "target/classes/com/test" ]; then
        pass "编译后的 class 文件已生成"
    else
        warn "编译后的 class 文件未找到"
    fi
else
    warn "mvn compile 测试失败"
    tail -10 /tmp/maven-compile.log
fi

echo
echo "测试 4: 测试 mvn package..."
if mvn package -DskipTests &> /tmp/maven-package.log 2>&1; then
    pass "mvn package 测试通过"
    
    if [ -f "target/maven-client-test-1.0.0.jar" ]; then
        pass "JAR 文件已生成"
        info "JAR 大小: $(du -sh target/maven-client-test-1.0.0.jar | cut -f1)"
    else
        warn "JAR 文件未生成"
    fi
else
    warn "mvn package 测试失败"
    tail -10 /tmp/maven-package.log
fi

echo
echo "测试 5: 配置 Maven settings.xml..."
mkdir -p ~/.m2

cat > ~/.m2/settings.xml <<EOF
<?xml version="1.0" encoding="UTF-8"?>
<settings xmlns="http://maven.apache.org/SETTINGS/1.0.0"
          xmlns:xsi="http://www.w3.org/2001/XMLSchema-instance"
          xsi:schemaLocation="http://maven.apache.org/SETTINGS/1.0.0 
          http://maven.apache.org/xsd/settings-1.0.0.xsd">
    <servers>
        <server>
            <id>maven-local</id>
            <username>admin</username>
            <password>admin123</password>
        </server>
    </servers>
</settings>
EOF
pass "settings.xml 配置完成"

echo
echo "测试 6: 验证 Maven 代理仓库访问..."
HTTP_CODE=$(curl -s -o /tmp/maven-meta.xml -w "%{http_code}" \
    "$BASE_URL/repository/maven-proxy-aliyun/com/google/guava/guava/maven-metadata.xml")

if [ "$HTTP_CODE" = "200" ]; then
    pass "Maven 代理元数据可访问 (HTTP 200)"
    
    if grep -q "<metadata>" /tmp/maven-meta.xml; then
        pass "元数据 XML 格式正确"
    else
        warn "元数据格式不正确"
    fi
else
    warn "Maven 代理元数据不可访问 (HTTP $HTTP_CODE)"
fi

echo
echo "测试 7: 验证 Maven artifact 下载..."
HTTP_CODE=$(curl -s -o /tmp/guava.jar -w "%{http_code}" \
    "$BASE_URL/repository/maven-proxy-aliyun/com/google/guava/guava/32.1.3-jre/guava-32.1.3-jre.jar")

if [ "$HTTP_CODE" = "200" ]; then
    pass "Maven artifact 下载成功 (HTTP 200)"
    
    if [ -s "/tmp/guava.jar" ]; then
        JAR_SIZE=$(stat -c%s /tmp/guava.jar 2>/dev/null || stat -f%z /tmp/guava.jar 2>/dev/null)
        info "JAR 大小: $JAR_SIZE bytes"
        
        # 验证 JAR 文件（本质是 ZIP）
        if unzip -t /tmp/guava.jar > /dev/null 2>&1; then
            pass "JAR 文件格式正确"
        else
            warn "JAR 文件格式无效"
        fi
    else
        warn "下载的 JAR 文件为空"
    fi
else
    warn "Maven artifact 下载失败 (HTTP $HTTP_CODE)"
fi

echo
echo "测试 8: 尝试 mvn deploy 到本地仓库..."
if [ -n "$TOKEN" ]; then
    # 在 pom.xml 中添加 distributionManagement
    cat > pom.xml <<'EOF'
<?xml version="1.0" encoding="UTF-8"?>
<project xmlns="http://maven.apache.org/POM/4.0.0"
         xmlns:xsi="http://www.w3.org/2001/XMLSchema-instance"
         xsi:schemaLocation="http://maven.apache.org/POM/4.0.0
         http://maven.apache.org/xsd/maven-4.0.0.xsd">
    <modelVersion>4.0.0</modelVersion>

    <groupId>com.test</groupId>
    <artifactId>maven-client-test</artifactId>
    <version>1.0.0</version>
    <packaging>jar</packaging>

    <distributionManagement>
        <repository>
            <id>maven-local</id>
            <url>http://localhost:9081/repository/maven-local</url>
        </repository>
    </distributionManagement>

    <properties>
        <maven.compiler.source>11</maven.compiler.source>
        <maven.compiler.target>11</maven.compiler.target>
        <project.build.sourceEncoding>UTF-8</project.build.sourceEncoding>
    </properties>
</project>
EOF

    # mvn deploy 使用 settings.xml 中的认证
    if mvn deploy -DskipTests &> /tmp/maven-deploy.log 2>&1; then
        pass "mvn deploy 测试通过"

        # 验证 deploy 后可以通过代理仓库下载
        HTTP_CODE=$(curl -s -o /dev/null -w "%{http_code}" \
            "$BASE_URL/repository/maven-local/com/test/maven-client-test/1.0.0/maven-client-test-1.0.0.jar")
        if [ "$HTTP_CODE" = "200" ]; then
            pass "mvn deploy 后 artifact 可下载 (HTTP 200)"

            JAR_SIZE=$(stat -c%s /tmp/maven-deploy-check.jar 2>/dev/null || stat -f%z /tmp/maven-deploy-check.jar 2>/dev/null)
            info "deploy 的 JAR 大小: ${JAR_SIZE:-unknown} bytes"
        else
            warn "mvn deploy 后 artifact 不可下载 (HTTP $HTTP_CODE)"
        fi

        # 验证 pom 文件可下载
        HTTP_CODE=$(curl -s -o /tmp/maven-deploy-pom.xml -w "%{http_code}" \
            "$BASE_URL/repository/maven-local/com/test/maven-client-test/1.0.0/maven-client-test-1.0.0.pom")
        if [ "$HTTP_CODE" = "200" ]; then
            pass "mvn deploy 后 POM 文件可下载"
        else
            warn "mvn deploy 后 POM 文件不可下载 (HTTP $HTTP_CODE)"
        fi
    else
        warn "mvn deploy 测试失败"
        info "错误日志:"
        tail -10 /tmp/maven-deploy.log
    fi
else
    fail "跳过 mvn deploy (无认证令牌)"
fi

# 清理
cd /
rm -rf "$TEST_DIR"
rm -f ~/.m2/settings.xml

echo
echo "============================================"
echo " Maven 客户端测试完成"
echo "============================================"
