#!/bin/bash
# moonlight-box 启动脚本
# 用法: ./ops/start.sh [--dev|--prod|--daemon] [--config path]

set -euo pipefail

SCRIPT_DIR="$(cd "$(dirname "$0")" && pwd)"
PROJECT_DIR="$(dirname "$(dirname "$SCRIPT_DIR")")"
APP_NAME="moonlight-box"
BIN_DIR="$PROJECT_DIR/bin"
PID_FILE="$PROJECT_DIR/.moonlight-box.pid"
LOG_DIR="$PROJECT_DIR/logs"
DEFAULT_CONFIG="$PROJECT_DIR/configs/config.yaml"

MODE="prod"
CONFIG=""
EXTRA_ARGS=()

usage() {
    cat <<EOF
Usage: $(basename "$0") [options]

Options:
  --dev         开发模式 (air 热重载)
  --prod        生产模式 (默认)
  --daemon      后台运行
  --config FILE 指定配置文件路径
  --stop        停止后台进程
  --restart     重启后台进程
  --status      查看进程状态
  -h, --help    显示帮助
EOF
}

stop() {
    if [ -f "$PID_FILE" ]; then
        PID=$(cat "$PID_FILE")
        if kill -0 "$PID" 2>/dev/null; then
            echo "Stopping $APP_NAME (PID $PID)..."
            kill "$PID"
            # 等待进程退出，最多 10 秒
            for i in $(seq 1 10); do
                if ! kill -0 "$PID" 2>/dev/null; then
                    rm -f "$PID_FILE"
                    echo "Stopped."
                    return 0
                fi
                sleep 1
            done
            echo "Force killing..."
            kill -9 "$PID" 2>/dev/null || true
            rm -f "$PID_FILE"
            echo "Force stopped."
        else
            echo "Process $PID not running, cleaning PID file."
            rm -f "$PID_FILE"
        fi
    else
        echo "No PID file found."
    fi
}

status() {
    if [ -f "$PID_FILE" ]; then
        PID=$(cat "$PID_FILE")
        if kill -0 "$PID" 2>/dev/null; then
            echo "$APP_NAME is running (PID $PID)"
            return 0
        else
            echo "$APP_NAME is not running (stale PID file)"
            return 1
        fi
    else
        echo "$APP_NAME is not running"
        return 1
    fi
}

# 解析参数
while [[ $# -gt 0 ]]; do
    case $1 in
        --dev) MODE="dev"; shift ;;
        --prod) MODE="prod"; shift ;;
        --daemon) MODE="daemon"; shift ;;
        --config) CONFIG="$2"; shift 2 ;;
        --stop) stop; exit $? ;;
        --restart) stop; sleep 1; shift; MODE="daemon"; break ;;
        --status) status; exit $? ;;
        -h|--help) usage; exit 0 ;;
        *) EXTRA_ARGS+=("$1"); shift ;;
    esac
done

CONFIG="${CONFIG:-$DEFAULT_CONFIG}"

# 前置检查
if [ ! -f "$CONFIG" ]; then
    echo "Error: Config file not found: $CONFIG"
    exit 1
fi

if [ "$MODE" = "dev" ]; then
    echo "Starting in dev mode with air..."
    cd "$PROJECT_DIR"
    exec air -c .air.toml 2>/dev/null || air
fi

# 确保 bin 目录和二进制存在
mkdir -p "$BIN_DIR" "$LOG_DIR"
if [ ! -f "$BIN_DIR/$APP_NAME" ]; then
    echo "Building $APP_NAME..."
    cd "$PROJECT_DIR"
    make build
fi

# 停止已有进程
stop 2>/dev/null || true

if [ "$MODE" = "daemon" ]; then
    echo "Starting $APP_NAME in daemon mode..."
    # bash 3.2 (macOS 自带) 在 set -u 下空数组展开会报 unbound variable,
    # 用 ${EXTRA_ARGS[@]+...} 惯用法兼容
    nohup "$BIN_DIR/$APP_NAME" -config "$CONFIG" serve "${EXTRA_ARGS[@]+"${EXTRA_ARGS[@]}"}" \
        > "$LOG_DIR/$APP_NAME.log" 2>&1 &
    echo $! > "$PID_FILE"
    echo "Started (PID $(cat "$PID_FILE")), log: $LOG_DIR/$APP_NAME.log"
else
    echo "Starting $APP_NAME..."
    exec "$BIN_DIR/$APP_NAME" -config "$CONFIG" serve "${EXTRA_ARGS[@]}"
fi
