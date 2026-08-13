#!/bin/bash
# 存储清理脚本：清理孤立的 blob 文件
# 用法: ./ops/storage_cleanup.sh [--dry-run]

set -euo pipefail

SCRIPT_DIR="$(cd "$(dirname "$0")" && pwd)"
PROJECT_DIR="$(dirname "$SCRIPT_DIR")"
DB_PATH="${DB_PATH:-$PROJECT_DIR/data/registry.db}"
STORAGE_DIR="${STORAGE_DIR:-$PROJECT_DIR/data}"
DRY_RUN=false

usage() {
    cat <<EOF
Usage: $(basename "$0") [options]

扫描存储目录，清理数据库中不存在的孤立文件。
需要安装 sqlite3 CLI。

Options:
  --dry-run     仅列出孤立文件，不删除
  --db PATH     数据库路径（默认: data/registry.db）
  --dir PATH    存储目录（默认: data）
  -h, --help    显示帮助
EOF
}

while [[ $# -gt 0 ]]; do
    case $1 in
        --dry-run) DRY_RUN=true; shift ;;
        --db) DB_PATH="$2"; shift 2 ;;
        --dir) STORAGE_DIR="$2"; shift 2 ;;
        -h|--help) usage; exit 0 ;;
        *) echo "Unknown option: $1"; usage; exit 1 ;;
    esac
done

if [ ! -f "$DB_PATH" ]; then
    echo "Error: Database not found: $DB_PATH"
    exit 1
fi

if ! command -v sqlite3 &>/dev/null; then
    echo "Error: sqlite3 CLI not found. Install with: brew install sqlite3"
    exit 1
fi

echo "=== Storage Cleanup ==="
echo "数据库: $DB_PATH"
echo "存储目录: $STORAGE_DIR"
echo ""

# 获取数据库中引用的所有存储路径
echo "查询数据库中的存储路径..."
DB_PATHS=$(sqlite3 "$DB_PATH" "
    SELECT path FROM artifacts WHERE path != '' AND path IS NOT NULL
    UNION
    SELECT remote_path FROM artifacts WHERE remote_path != '' AND remote_path IS NOT NULL
    UNION
    SELECT file_path FROM backups WHERE file_path != '' AND file_path IS NOT NULL
" 2>/dev/null | sort -u)

DB_COUNT=$(echo "$DB_PATHS" | grep -c . 2>/dev/null || echo 0)
echo "数据库中引用: $DB_COUNT 个路径"

# 扫描存储目录中的实际文件
echo "扫描存储目录..."
ORPHAN_COUNT=0
ORPHAN_SIZE=0

if [ -d "$STORAGE_DIR" ]; then
    while IFS= read -r file; do
        # 获取相对路径
        rel_path="${file#$STORAGE_DIR/}"
        # 检查是否在数据库中被引用
        if ! echo "$DB_PATHS" | grep -qF "$rel_path"; then
            ORPHAN_COUNT=$((ORPHAN_COUNT + 1))
            size=$(stat -f%z "$file" 2>/dev/null || stat -c%s "$file" 2>/dev/null || echo 0)
            ORPHAN_SIZE=$((ORPHAN_SIZE + size))
            if [ "$DRY_RUN" = true ]; then
                echo "  孤立: $rel_path ($(numfmt --to=iec "$size" 2>/dev/null || echo "${size}B"))"
            fi
        fi
    done < <(find "$STORAGE_DIR" -type f \( -name "*.blob" -o -name "*.pack" -o -name "*.tar.*" -o -name "*.zip" \) 2>/dev/null)
fi

echo ""
echo "=== 结果 ==="
if [ "$ORPHAN_COUNT" -eq 0 ]; then
    echo "未发现孤立文件 ✓"
else
    echo "孤立文件: $ORPHAN_COUNT 个"
    echo "可回收空间: $(numfmt --to=iec "$ORPHAN_SIZE" 2>/dev/null || echo "${ORPHAN_SIZE}B")"
    if [ "$DRY_RUN" = false ]; then
        echo ""
        echo "请确认后手动删除，或使用 --dry-run 先预览"
    else
        echo ""
        echo "未执行删除。运行不带 --dry-run 执行实际清理。"
    fi
fi
