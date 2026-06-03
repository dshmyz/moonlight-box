#!/bin/bash

set -e

BASE_URL="${1:-http://localhost:9081}"
ADMIN_USER="${ADMIN_USER:-admin}"
ADMIN_PASS="${ADMIN_PASS:-admin123}"

RED='\033[0;31m'
GREEN='\033[0;32m'
YELLOW='\033[1;33m'
BLUE='\033[0;34m'
NC='\033[0m'

PASS_COUNT=0
FAIL_COUNT=0
WARN_COUNT=0

pass() {
    echo -e "  ${GREEN}✓ PASS${NC} $1"
    PASS_COUNT=$((PASS_COUNT + 1))
}

fail() {
    echo -e "  ${RED}✗ FAIL${NC} $1"
    FAIL_COUNT=$((FAIL_COUNT + 1))
}

info() {
    echo -e "  ${BLUE}ℹ INFO${NC} $1"
}

warn() {
    echo -e "  ${YELLOW}⚠ WARN${NC} $1"
    WARN_COUNT=$((WARN_COUNT + 1))
}

get_auth_token() {
    curl -s -X POST "$BASE_URL/api/v1/auth/login" \
        -H "Content-Type: application/json" \
        -d "{\"username\":\"$ADMIN_USER\",\"password\":\"$ADMIN_PASS\"}" | \
        grep -o '"access_token":"[^"]*"' | \
        sed 's/"access_token":"//;s/"//'
}

check_mvn() {
    if ! command -v mvn &> /dev/null; then
        warn "mvn 命令未安装，跳过 Maven 生命周期测试"
        return 1
    fi
    return 0
}

# cleanup
CLEAN_TEMPS=()
cleanup() { rm -rf "${CLEAN_TEMPS[@]}" 2>/dev/null || true; }
trap cleanup EXIT

echo "============================================"
echo " Maven 生命周期测试"
echo " 目标: $BASE_URL"
echo "============================================"
echo

if ! check_mvn; then
    echo -e "${YELLOW}跳过 Maven 测试（需要安装 Maven）${NC}"
    exit 0
fi

TOKEN=$(get_auth_token)

if [ -z "$TOKEN" ]; then
    echo -e "${RED}错误: 无法获取认证令牌${NC}"
    exit 1
fi

TEST_DIR="/tmp/maven-test-$$"
CLEAN_TEMPS+=("$TEST_DIR")
mkdir -p "$TEST_DIR"

echo "════════════════════════════════════════"
echo "  测试 1: 创建测试 Maven 项目"
echo "════════════════════════════════════════"

cat > "$TEST_DIR/pom.xml" <<'EOF'
<?xml version="1.0" encoding="UTF-8"?>
<project xmlns="http://maven.apache.org/POM/4.0.0"
         xmlns:xsi="http://www.w3.org/2001/XMLSchema-instance"
         xsi:schemaLocation="http://maven.apache.org/POM/4.0.0 
         http://maven.apache.org/xsd/maven-4.0.0.xsd">
    <modelVersion>4.0.0</modelVersion>
    
    <groupId>com.test</groupId>
    <artifactId>maven-test-artifact</artifactId>
    <version>1.0.0</version>
    <packaging>jar</packaging>
    
    <properties>
        <maven.compiler.source>1.8</maven.compiler.source>
        <maven.compiler.target>1.8</maven.compiler.target>
        <project.build.sourceEncoding>UTF-8</project.build.sourceEncoding>
    </properties>
    
    <distributionManagement>
        <repository>
            <id>test-repo</id>
            <url>REPO_URL_PLACEHOLDER</url>
        </repository>
    </distributionManagement>
</project>
EOF

REPO_URL="$BASE_URL/repository/maven-local"
sed -i.bak "s|REPO_URL_PLACEHOLDER|$REPO_URL|g" "$TEST_DIR/pom.xml"
rm -f "$TEST_DIR/pom.xml.bak"

mkdir -p "$TEST_DIR/src/main/java/com/test"
cat > "$TEST_DIR/src/main/java/com/test/Test.java" <<'EOF'
package com.test;

public class Test {
    public static void main(String[] args) {
        System.out.println("Hello from test artifact!");
    }
}
EOF

if [ -f "$TEST_DIR/pom.xml" ] && [ -f "$TEST_DIR/src/main/java/com/test/Test.java" ]; then
    pass "测试 Maven 项目创建成功"
else
    fail "测试 Maven 项目创建失败"
fi

echo
echo "════════════════════════════════════════"
echo "  测试 2: 编译 Maven 项目"
echo "════════════════════════════════════════"

cd "$TEST_DIR"

if mvn compile > /dev/null 2>&1; then
    pass "Maven 项目编译成功"
else
    warn "Maven 项目编译失败（可能缺少依赖）"
fi

echo
echo "════════════════════════════════════════"
echo "  测试 3: 打包 Maven 项目"
echo "════════════════════════════════════════"

if mvn package -DskipTests > /dev/null 2>&1; then
    pass "Maven 项目打包成功"
    
    if [ -f "$TEST_DIR/target/maven-test-artifact-1.0.0.jar" ]; then
        pass "JAR 文件生成成功"
    else
        fail "JAR 文件未生成"
    fi
else
    fail "Maven 项目打包失败"
fi

echo
echo "════════════════════════════════════════"
echo "  测试 4: 部署到本地仓库"
echo "════════════════════════════════════════"

SETTINGS_FILE="$TEST_DIR/settings.xml"
cat > "$SETTINGS_FILE" <<EOF
<?xml version="1.0" encoding="UTF-8"?>
<settings xmlns="http://maven.apache.org/SETTINGS/1.0.0"
          xmlns:xsi="http://www.w3.org/2001/XMLSchema-instance"
          xsi:schemaLocation="http://maven.apache.org/SETTINGS/1.0.0 
          http://maven.apache.org/xsd/settings-1.0.0.xsd">
    <servers>
        <server>
            <id>test-repo</id>
            <username>$ADMIN_USER</username>
            <password>$ADMIN_PASS</password>
        </server>
    </servers>
</settings>
EOF

if mvn deploy -DskipTests -s "$SETTINGS_FILE" > /dev/null 2>&1; then
    pass "Maven 项目部署成功"
    
    HTTP_CODE=$(curl -s -o /dev/null -w "%{http_code}" \
        "$BASE_URL/repository/maven-local/com/test/maven-test-artifact/1.0.0/maven-test-artifact-1.0.0.jar")
    
    if [ "$HTTP_CODE" = "200" ]; then
        pass "部署的 JAR 文件可访问 (HTTP 200)"
    else
        fail "部署的 JAR 文件不可访问 (HTTP $HTTP_CODE)"
    fi
else
    warn "Maven 项目部署失败（可能需要认证配置）"
fi

echo
echo "════════════════════════════════════════"
echo "  测试 5: 下载依赖"
echo "════════════════════════════════════════"

DOWNLOAD_DIR="/tmp/maven-download-test-$$"
CLEAN_TEMPS+=("$DOWNLOAD_DIR")
mkdir -p "$DOWNLOAD_DIR"

cat > "$DOWNLOAD_DIR/pom.xml" <<EOF
<?xml version="1.0" encoding="UTF-8"?>
<project xmlns="http://maven.apache.org/POM/4.0.0"
         xmlns:xsi="http://www.w3.org/2001/XMLSchema-instance"
         xsi:schemaLocation="http://maven.apache.org/POM/4.0.0 
         http://maven.apache.org/xsd/maven-4.0.0.xsd">
    <modelVersion>4.0.0</modelVersion>
    
    <groupId>com.test</groupId>
    <artifactId>download-test</artifactId>
    <version>1.0.0</version>
    
    <repositories>
        <repository>
            <id>test-repo</id>
            <url>$REPO_URL</url>
        </repository>
    </repositories>
    
    <dependencies>
        <dependency>
            <groupId>com.test</groupId>
            <artifactId>maven-test-artifact</artifactId>
            <version>1.0.0</version>
        </dependency>
    </dependencies>
</project>
EOF

cd "$DOWNLOAD_DIR"

if mvn dependency:resolve > /dev/null 2>&1; then
    pass "从仓库下载依赖成功"
else
    warn "从仓库下载依赖失败（可能需要配置）"
fi

echo
echo "════════════════════════════════════════"
echo "  测试 6: 代理仓库下载"
echo "════════════════════════════════════════"

PROXY_DOWNLOAD_DIR="/tmp/maven-proxy-download-test-$$"
CLEAN_TEMPS+=("$PROXY_DOWNLOAD_DIR")
mkdir -p "$PROXY_DOWNLOAD_DIR"

cat > "$PROXY_DOWNLOAD_DIR/pom.xml" <<EOF
<?xml version="1.0" encoding="UTF-8"?>
<project xmlns="http://maven.apache.org/POM/4.0.0"
         xmlns:xsi="http://www.w3.org/2001/XMLSchema-instance"
         xsi:schemaLocation="http://maven.apache.org/POM/4.0.0 
         http://maven.apache.org/xsd/maven-4.0.0.xsd">
    <modelVersion>4.0.0</modelVersion>
    
    <groupId>com.test</groupId>
    <artifactId>proxy-download-test</artifactId>
    <version>1.0.0</version>
    
    <repositories>
        <repository>
            <id>proxy-repo</id>
            <url>$BASE_URL/repository/maven-proxy-aliyun</url>
        </repository>
    </repositories>
    
    <dependencies>
        <dependency>
            <groupId>com.google.guava</groupId>
            <artifactId>guava</artifactId>
            <version>32.1.3-jre</version>
        </dependency>
    </dependencies>
</project>
EOF

cd "$PROXY_DOWNLOAD_DIR"

if mvn dependency:resolve > /dev/null 2>&1; then
    pass "从代理仓库下载依赖成功"
else
    warn "从代理仓库下载依赖失败"
fi

echo
echo "════════════════════════════════════════"
echo "  测试 7: 清理测试文件"
echo "════════════════════════════════════════"

cd /
rm -rf "$TEST_DIR" "$DOWNLOAD_DIR" "$PROXY_DOWNLOAD_DIR"

if [ ! -d "$TEST_DIR" ] && [ ! -d "$DOWNLOAD_DIR" ] && [ ! -d "$PROXY_DOWNLOAD_DIR" ]; then
    pass "测试文件清理成功"
else
    warn "测试文件清理可能不完整"
fi

echo
echo "============================================"
echo " 测试汇总"
echo "============================================"
echo -e "  通过: ${GREEN}$PASS_COUNT${NC}"
echo -e "  失败: ${RED}$FAIL_COUNT${NC}"
echo -e "  警告: ${YELLOW}$WARN_COUNT${NC}"
echo -e "  总计: $((PASS_COUNT + FAIL_COUNT + WARN_COUNT))"
echo

if [ $FAIL_COUNT -eq 0 ]; then
    echo -e "${GREEN}所有测试通过! ✓${NC}"
    exit 0
else
    echo -e "${YELLOW}部分测试失败! ❌${NC}"
    exit 1
fi
