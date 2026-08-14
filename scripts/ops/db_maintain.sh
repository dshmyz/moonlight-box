#!/bin/bash
# 数据库维护脚本
# 用法: ./ops/db_maintain.sh [--vacuum|--integrity|--stats]

set -euo pipefail

SCRIPT_DIR="$(cd "$(dirname "$0")" && pwd)"
PROJECT_DIR="$(dirname "$(dirname "$SCRIPT_DIR")")"

# 从配置文件读取数据库路径
DB_PATH="${DB_PATH:-$PROJECT_DIR/data/registry.db}"

if [ ! -f "$DB_PATH" ]; then
    echo "Error: Database not found: $DB_PATH"
    echo "Set DB_PATH env var to override."
    exit 1
fi

usage() {
    cat <<EOF
Usage: $(basename "$0") [options]

Options:
  --vacuum     压缩数据库（回收空间，需要停服或短暂停写）
  --integrity  检查数据库完整性
  --stats      显示数据库统计信息
  --all        执行所有维护操作
  -h, --help   显示帮助
EOF
}

vacuum() {
    echo "=== VACUUM: $DB_PATH ==="
    local size_before
    size_before=$(du -h "$DB_PATH" | cut -f1)
    echo "执行前大小: $size_before"
    sqlite3 "$DB_PATH" "VACUUM;"
    local size_after
    size_after=$(du -h "$DB_PATH" | cut -f1)
    echo "执行后大小: $size_after"
    echo "VACUUM 完成。"
}

integrity() {
    echo "=== Integrity Check: $DB_PATH ==="
    local result
    result=$(sqlite3 "$DB_PATH" "PRAGMA integrity_check;" 2>&1)
    if [ "$result" = "ok" ]; then
        echo "数据库完整性检查通过 ✓"
    else
        echo "数据库完整性检查失败:"
        echo "$result"
        return 1
    fi
}

stats() {
    echo "=== Database Stats: $DB_PATH ==="
    echo ""

    echo "--- 文件信息 ---"
    ls -lh "$DB_PATH" | awk '{print "大小: "$5, "修改时间: "$6" "$7" "$8}'
    echo ""

    echo "--- 表行数 ---"
    sqlite3 "$DB_PATH" "SELECT name, (SELECT COUNT(*) FROM sqlite_master WHERE type='table' AND t.name = name) AS row_count FROM (SELECT name FROM sqlite_master WHERE type='table') t ORDER BY name;" 2>/dev/null | while IFS='|' read -r table count; do
        printf "  %-30s %s\n" "$table" "$count"
    done
    echo ""

    echo "--- 下载日志统计 ---"
    sqlite3 "$DB_PATH" "
        SELECT
            '总行数' AS metric, COUNT(*) AS value FROM download_logs
        UNION ALL
        SELECT '成功', COUNT(*) FROM download_logs WHERE status='success'
        UNION ALL
        SELECT '失败', COUNT(*) FROM download_logs WHERE status='failed'
        UNION ALL
        SELECT '缓存命中', COUNT(*) FROM download_logs WHERE status='cached'
        UNION ALL
        SELECT '保留天数', CAST((julianday('now') - julianday(MIN(created_at))) AS INTEGER) FROM download_logs
    " 2>/dev/null | while IFS='|' read -r metric value; do
        printf "  %-16s %s\n" "$metric" "$value"
    done
    echo ""

    echo "--- 聚合表统计 ---"
    sqlite3 "$DB_PATH" "
        SELECT '总行数', COUNT(*) FROM download_daily_stats
        UNION ALL
        SELECT '天数跨度', CAST((julianday('now') - julianday(MIN(date))) AS INTEGER) FROM download_daily_stats
    " 2>/dev/null | while IFS='|' read -r metric value; do
        printf "  %-16s %s\n" "$metric" "$value"
    done
    echo ""

    echo "--- 索引 ---"
    sqlite3 "$DB_PATH" "SELECT name FROM sqlite_master WHERE type='index' AND name NOT LIKE 'sqlite_%' ORDER BY name;" 2>/dev/null | head -20
    local idx_count
    idx_count=$(sqlite3 "$DB_PATH" "SELECT COUNT(*) FROM sqlite_master WHERE type='index' AND name NOT LIKE 'sqlite_%';" 2>/dev/null)
    echo "  共 $idx_count 个索引"
}

# 解析参数
ACTION=""
while [[ $# -gt 0 ]]; do
    case $1 in
        --vacuum) ACTION="vacuum"; shift ;;
        --integrity) ACTION="integrity"; shift ;;
        --stats) ACTION="stats"; shift ;;
        --all) ACTION="all"; shift ;;
        -h|--help) usage; exit 0 ;;
        *) echo "Unknown option: $1"; usage; exit 1 ;;
    esac
done

if [ -z "$ACTION" ]; then
    usage
    exit 1
fi

case "$ACTION" in
    vacuum) vacuum ;;
    integrity) integrity ;;
    stats) stats ;;
    all)
        integrity
        echo ""
        stats
        echo ""
        echo "注意: VACUUM 需要停服后执行，请单独运行 --vacuum"
        ;;
esac
