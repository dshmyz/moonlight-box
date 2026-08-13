#!/bin/bash
# 日志清理脚本
# 用法: ./ops/log_cleanup.sh [--days N] [--dry-run]

set -euo pipefail

SCRIPT_DIR="$(cd "$(dirname "$0")" && pwd)"
PROJECT_DIR="$(dirname "$SCRIPT_DIR")"
LOG_DIR="$PROJECT_DIR/logs"
RETENTION_DAYS=30
DRY_RUN=false

usage() {
    cat <<EOF
Usage: $(basename "$0") [options]

清理应用日志文件（lumberjack 轮转日志），不影响数据库日志。

Options:
  --days N     保留天数（默认 30）
  --dry-run    仅列出将删除的文件，不实际删除
  -h, --help   显示帮助
EOF
}

while [[ $# -gt 0 ]]; do
    case $1 in
        --days) RETENTION_DAYS="$2"; shift 2 ;;
        --dry-run) DRY_RUN=true; shift ;;
        -h|--help) usage; exit 0 ;;
        *) echo "Unknown option: $1"; usage; exit 1 ;;
    esac
done

echo "=== Log Cleanup: $LOG_DIR ==="
echo "保留最近 $RETENTION_DAYS 天的日志"
echo ""

if [ ! -d "$LOG_DIR" ]; then
    echo "日志目录不存在: $LOG_DIR"
    exit 0
fi

# 统计
total_files=$(find "$LOG_DIR" -name "*.log" -type f 2>/dev/null | wc -l | tr -d ' ')
total_size=$(du -sh "$LOG_DIR" 2>/dev/null | cut -f1)
echo "当前日志文件: $total_files 个, 总大小: $total_size"

# 查找过期文件
old_files=$(find "$LOG_DIR" -name "*.log" -type f -mtime +"$RETENTION_DAYS" 2>/dev/null)
old_count=$(echo "$old_files" | grep -c . 2>/dev/null || echo 0)

if [ "$old_count" -eq 0 ]; then
    echo ""
    echo "没有超过 $RETENTION_DAYS 天的日志文件。"
    exit 0
fi

old_size=$(echo "$old_files" | xargs du -ch 2>/dev/null | tail -1 | cut -f1)
echo ""
echo "将删除: $old_count 个文件, 约 $old_size"

if [ "$DRY_RUN" = true ]; then
    echo ""
    echo "[Dry Run] 以下文件将被删除:"
    echo "$old_files" | while read -r f; do
        echo "  $f"
    done
    echo ""
    echo "未执行实际删除。"
    exit 0
fi

echo ""
echo "清理中..."
echo "$old_files" | xargs rm -f

# 清理空目录
find "$LOG_DIR" -type d -empty -delete 2>/dev/null || true

echo "清理完成。"
echo ""
echo "清理后:"
new_total=$(find "$LOG_DIR" -name "*.log" -type f 2>/dev/null | wc -l | tr -d ' ')
new_size=$(du -sh "$LOG_DIR" 2>/dev/null | cut -f1)
echo "  日志文件: $new_total 个, 总大小: $new_size"
