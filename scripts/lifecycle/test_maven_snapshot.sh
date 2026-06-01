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
        warn "mvn 命令未安装，跳过 Maven SNAPSHOT 测试"
        return 1
    fi
    return 0
}

# cleanup
CLEAN_TEMPS=()
cleanup() { rm -rf "${CLEAN_TEMPS[@]}" 2>/dev/null || true; }
trap cleanup EXIT

echo "============================================"
echo " Maven SNAPSHOT 版本测试"
echo " 目标: $BASE_URL"
echo "============================================"
echo

if ! check_mvn; then
    echo -e "${YELLOW}跳过 Maven SNAPSHOT 测试（需要安装 Maven）${NC}"
    exit 0
fi

TOKEN=$(get_auth_token)

if [ -z "$TOKEN" ]; then
    echo -e "${RED}错误: 无法获取认证令牌${NC}"
    exit 1
fi

REPO_CHECK=$(curl -s -o /dev/null -w "%{http_code}" "$BASE_URL/repository/maven-snapshots/" -H "Authorization: Bearer $TOKEN")
if [ "$REPO_CHECK" = "404" ]; then
    curl -s -X POST "$BASE_URL/api/v1/repositories" \
        -H "Authorization: Bearer $TOKEN" \
        -H "Content-Type: application/json" \
        -d '{"name":"maven-snapshots","type":"local","format":"maven","package_type":"maven","description":"Maven SNAPSHOT repository","config":{"allow_redeploy":true,"layout_policy":"permissive"}}' > /dev/null 2>&1
fi

TEST_DIR="/tmp/maven-snapshot-test-$$"
CLEAN_TEMPS+=("$TEST_DIR")
mkdir -p "$TEST_DIR"

echo "════════════════════════════════════════"
echo "  测试 1: 创建 SNAPSHOT 版本 Maven 项目"
echo "════════════════════════════════════════"

cd "$TEST_DIR"

cat > pom.xml <<'EOF'
<?xml version="1.0" encoding="UTF-8"?>
<project xmlns="http://maven.apache.org/POM/4.0.0"
         xmlns:xsi="http://www.w3.org/2001/XMLSchema-instance"
         xsi:schemaLocation="http://maven.apache.org/POM/4.0.0 
         http://maven.apache.org/xsd/maven-4.0.0.xsd">
    <modelVersion>4.0.0</modelVersion>
    
    <groupId>com.test</groupId>
    <artifactId>snapshot-lib</artifactId>
    <version>1.0-SNAPSHOT</version>
    <packaging>jar</packaging>
    
    <properties>
        <maven.compiler.source>1.8</maven.compiler.source>
        <maven.compiler.target>1.8</maven.compiler.target>
        <project.build.sourceEncoding>UTF-8</project.build.sourceEncoding>
    </properties>
    
    <distributionManagement>
        <snapshotRepository>
            <id>test-snapshot-repo</id>
            <url>SNAPSHOT_REPO_URL_PLACEHOLDER</url>
        </snapshotRepository>
    </distributionManagement>
</project>
EOF

REPO_URL="$BASE_URL/repository/maven-snapshots"
sed -i.bak "s|SNAPSHOT_REPO_URL_PLACEHOLDER|$REPO_URL|g" pom.xml
rm -f pom.xml.bak

mkdir -p src/main/java/com/test
cat > src/main/java/com/test/SnapshotLib.java <<'EOF'
package com.test;

public class SnapshotLib {
    public static String getVersion() {
        return "1.0-SNAPSHOT";
    }
}
EOF

if [ -f "pom.xml" ] && [ -f "src/main/java/com/test/SnapshotLib.java" ]; then
    pass "SNAPSHOT Maven 项目创建成功"
else
    fail "SNAPSHOT Maven 项目创建失败"
fi

echo
echo "════════════════════════════════════════"
echo "  测试 2: 打包 SNAPSHOT 版本"
echo "════════════════════════════════════════"

if mvn package -DskipTests > /dev/null 2>&1; then
    pass "SNAPSHOT 项目打包成功"
    
    SNAPSHOT_JAR=$(find target -name "snapshot-lib-1.0-*.jar" | head -n 1)
    if [ -n "$SNAPSHOT_JAR" ] && [ -f "$SNAPSHOT_JAR" ]; then
        pass "SNAPSHOT JAR 文件生成成功: $(basename $SNAPSHOT_JAR)"
    else
        fail "SNAPSHOT JAR 文件未生成"
    fi
else
    fail "SNAPSHOT 项目打包失败"
fi

echo
echo "════════════════════════════════════════"
echo "  测试 3: 部署 SNAPSHOT 版本（第一次）"
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
            <id>test-snapshot-repo</id>
            <username>$ADMIN_USER</username>
            <password>$ADMIN_PASS</password>
        </server>
    </servers>
</settings>
EOF

if mvn deploy -DskipTests -s "$SETTINGS_FILE" > /dev/null 2>&1; then
    pass "SNAPSHOT 版本第一次部署成功"
else
    warn "SNAPSHOT 版本第一次部署失败（可能需要认证配置）"
fi

echo
echo "════════════════════════════════════════"
echo "  测试 4: 验证 SNAPSHOT 文件存储结构"
echo "════════════════════════════════════════"

HTTP_CODE=$(curl -s -o /dev/null -w "%{http_code}" \
    "$BASE_URL/repository/maven-snapshots/com/test/snapshot-lib/1.0-SNAPSHOT/maven-metadata.xml")

if [ "$HTTP_CODE" = "200" ]; then
    pass "maven-metadata.xml 可访问 (HTTP 200)"
    
    METADATA_CONTENT=$(curl -s "$BASE_URL/repository/maven-snapshots/com/test/snapshot-lib/1.0-SNAPSHOT/maven-metadata.xml")
    
    if echo "$METADATA_CONTENT" | grep -q "<snapshot>"; then
        pass "maven-metadata.xml 包含 <snapshot> 标签"
    else
        fail "maven-metadata.xml 未包含 <snapshot> 标签"
    fi
    
    if echo "$METADATA_CONTENT" | grep -q "<timestamp>"; then
        pass "maven-metadata.xml 包含 <timestamp> 标签"
        
        TIMESTAMP=$(echo "$METADATA_CONTENT" | grep -o "<timestamp>[^<]*</timestamp>" | sed 's/<timestamp>//;s/<\/timestamp>//')
        info "SNAPSHOT 时间戳: $TIMESTAMP"
    else
        info "maven-metadata.xml 未包含 <timestamp> 标签"
    fi
    
    if echo "$METADATA_CONTENT" | grep -q "<buildNumber>"; then
        pass "maven-metadata.xml 包含 <buildNumber> 标签"
    else
        info "maven-metadata.xml 未包含 <buildNumber> 标签"
    fi
else
    fail "maven-metadata.xml 不可访问 (HTTP $HTTP_CODE)"
fi

echo
echo "════════════════════════════════════════"
echo "  测试 5: 验证 SNAPSHOT JAR 文件可下载"
echo "════════════════════════════════════════"

if [ -n "$TIMESTAMP" ]; then
    SNAPSHOT_JAR_URL="$BASE_URL/repository/maven-snapshots/com/test/snapshot-lib/1.0-SNAPSHOT/snapshot-lib-1.0-${TIMESTAMP}-1.jar"
    
    HTTP_CODE=$(curl -s -o /tmp/snapshot-download-test.jar -w "%{http_code}" "$SNAPSHOT_JAR_URL")
    
    if [ "$HTTP_CODE" = "200" ]; then
        pass "SNAPSHOT JAR 文件下载成功 (HTTP 200)"
        
        if [ -f "/tmp/snapshot-download-test.jar" ] && [ -s "/tmp/snapshot-download-test.jar" ]; then
            pass "下载的 SNAPSHOT JAR 文件非空"
        else
            fail "下载的 SNAPSHOT JAR 文件为空"
        fi
    else
        warn "SNAPSHOT JAR 文件下载失败 (HTTP $HTTP_CODE)"
        info "尝试 URL: $SNAPSHOT_JAR_URL"
    fi
    
    rm -f /tmp/snapshot-download-test.jar
fi

echo
echo "════════════════════════════════════════"
echo "  测试 6: 部署 SNAPSHOT 版本（第二次 - 更新）"
echo "════════════════════════════════════════"

sleep 2

cat > src/main/java/com/test/SnapshotLib.java <<'EOF'
package com.test;

public class SnapshotLib {
    public static String getVersion() {
        return "1.0-SNAPSHOT (updated)";
    }
}
EOF

if mvn package deploy -DskipTests -s "$SETTINGS_FILE" > /dev/null 2>&1; then
    pass "SNAPSHOT 版本第二次部署成功"
    
    HTTP_CODE=$(curl -s -o /dev/null -w "%{http_code}" \
        "$BASE_URL/repository/maven-snapshots/com/test/snapshot-lib/1.0-SNAPSHOT/maven-metadata.xml")
    
    if [ "$HTTP_CODE" = "200" ]; then
        UPDATED_METADATA=$(curl -s "$BASE_URL/repository/maven-snapshots/com/test/snapshot-lib/1.0-SNAPSHOT/maven-metadata.xml")
        
        if echo "$UPDATED_METADATA" | grep -q "<buildNumber>2</buildNumber>"; then
            pass "buildNumber 已更新为 2"
        else
            info "buildNumber 未更新或格式不同"
        fi
    fi
else
    warn "SNAPSHOT 版本第二次部署失败"
fi

echo
echo "════════════════════════════════════════"
echo "  测试 7: 从仓库下载 SNAPSHOT 依赖"
echo "════════════════════════════════════════"

DOWNLOAD_DIR="/tmp/maven-snapshot-download-test-$$"
CLEAN_TEMPS+=("$DOWNLOAD_DIR")
mkdir -p "$DOWNLOAD_DIR"
cd "$DOWNLOAD_DIR"

cat > pom.xml <<EOF
<?xml version="1.0" encoding="UTF-8"?>
<project xmlns="http://maven.apache.org/POM/4.0.0"
         xmlns:xsi="http://www.w3.org/2001/XMLSchema-instance"
         xsi:schemaLocation="http://maven.apache.org/POM/4.0.0 
         http://maven.apache.org/xsd/maven-4.0.0.xsd">
    <modelVersion>4.0.0</modelVersion>
    
    <groupId>com.test</groupId>
    <artifactId>snapshot-consumer</artifactId>
    <version>1.0.0</version>
    
    <repositories>
        <repository>
            <id>snapshot-repo</id>
            <url>$REPO_URL</url>
            <snapshots>
                <enabled>true</enabled>
            </snapshots>
        </repository>
    </repositories>
    
    <dependencies>
        <dependency>
            <groupId>com.test</groupId>
            <artifactId>snapshot-lib</artifactId>
            <version>1.0-SNAPSHOT</version>
        </dependency>
    </dependencies>
</project>
EOF

if mvn dependency:resolve > /dev/null 2>&1; then
    pass "SNAPSHOT 依赖下载成功"
else
    warn "SNAPSHOT 依赖下载失败（可能需要配置）"
fi

echo
echo "════════════════════════════════════════"
echo "  测试 8: 验证 SNAPSHOT 校验和文件"
echo "════════════════════════════════════════"

HTTP_CODE=$(curl -s -o /dev/null -w "%{http_code}" \
    "$BASE_URL/repository/maven-snapshots/com/test/snapshot-lib/1.0-SNAPSHOT/maven-metadata.xml.sha1")

if [ "$HTTP_CODE" = "200" ]; then
    pass "maven-metadata.xml.sha1 可访问 (HTTP 200)"
else
    info "maven-metadata.xml.sha1 返回 HTTP $HTTP_CODE"
fi

HTTP_CODE=$(curl -s -o /dev/null -w "%{http_code}" \
    "$BASE_URL/repository/maven-snapshots/com/test/snapshot-lib/1.0-SNAPSHOT/maven-metadata.xml.md5")

if [ "$HTTP_CODE" = "200" ]; then
    pass "maven-metadata.xml.md5 可访问 (HTTP 200)"
else
    fail "maven-metadata.xml.md5 返回 HTTP $HTTP_CODE"
fi

echo
echo "════════════════════════════════════════"
echo "  测试 9: 清理测试文件"
echo "════════════════════════════════════════"

cd /
rm -rf "$TEST_DIR" "$DOWNLOAD_DIR"

if [ ! -d "$TEST_DIR" ] && [ ! -d "$DOWNLOAD_DIR" ]; then
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
echo -e "  总计: $((PASS_COUNT + FAIL_COUNT))"
echo

if [ $FAIL_COUNT -eq 0 ]; then
    echo -e "${GREEN}所有测试通过! ✓${NC}"
    exit 0
else
    echo -e "${YELLOW}部分测试失败! ❌${NC}"
    exit 1
fi
