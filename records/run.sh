#!/bin/bash
# records 运行脚本：./run.sh start|stop|restart|status

cd "$(dirname "$0")"

APP_NAME="records"
PID_FILE="records.pid"
LOG_FILE="logs/records.log"
PORT=8000

# 检查是否已在运行（通过端口）
check_running() {
    if command -v lsof &>/dev/null; then
        lsof -i :$PORT &>/dev/null && return 0
    elif command -v fuser &>/dev/null; then
        fuser $PORT/tcp &>/dev/null && return 0
    fi
    return 1
}

# 停止进程
do_stop() {
    stop_by_pid() {
        local pid=$1
        if [ -n "$pid" ] && kill -0 "$pid" 2>/dev/null; then
            echo "[$APP_NAME] 正在停止 (PID: $pid)..."
            kill -TERM "$pid" 2>/dev/null
            for i in {1..10}; do
                sleep 1
                kill -0 "$pid" 2>/dev/null || break
            done
            if kill -0 "$pid" 2>/dev/null; then
                kill -9 "$pid" 2>/dev/null
            fi
            return 0
        fi
        return 1
    }

    stop_by_port() {
        if command -v lsof &>/dev/null; then
            local pids=$(lsof -ti :$PORT 2>/dev/null)
            if [ -n "$pids" ]; then
                echo "[$APP_NAME] 通过端口 $PORT 停止进程..."
                echo "$pids" | xargs kill -TERM 2>/dev/null
                sleep 2
                echo "$pids" | xargs kill -9 2>/dev/null
                return 0
            fi
        elif command -v fuser &>/dev/null; then
            if fuser $PORT/tcp &>/dev/null; then
                echo "[$APP_NAME] 通过端口 $PORT 停止进程..."
                fuser -k $PORT/tcp 2>/dev/null
                return 0
            fi
        fi
        return 1
    }

    if [ -f "$PID_FILE" ]; then
        pid=$(cat "$PID_FILE")
        if stop_by_pid "$pid"; then
            rm -f "$PID_FILE"
            echo "[$APP_NAME] 已停止"
            return 0
        fi
    fi

    if stop_by_port; then
        rm -f "$PID_FILE"
        echo "[$APP_NAME] 已停止"
        return 0
    fi

    echo "[$APP_NAME] 未在运行"
}

# 检查状态
do_status() {
    if check_running; then
        local pid=""
        if [ -f "$PID_FILE" ]; then
            pid=$(cat "$PID_FILE")
            kill -0 "$pid" 2>/dev/null || pid=""
        fi
        [ -z "$pid" ] && command -v lsof &>/dev/null && pid=$(lsof -ti :$PORT 2>/dev/null | head -1)
        [ -n "$pid" ] && echo "[$APP_NAME] 运行中 (PID: $pid, 端口: $PORT)" || echo "[$APP_NAME] 运行中 (端口: $PORT)"
    else
        echo "[$APP_NAME] 未运行"
    fi
}

# 启动进程
do_start() {
    if check_running; then
        echo "[$APP_NAME] 已在运行 (端口 $PORT)"
        return 1
    fi

    mkdir -p logs

    echo "[$APP_NAME] 正在启动..."
    nohup go run . >> "$LOG_FILE" 2>&1 &
    echo $! > "$PID_FILE"

    sleep 2
    if check_running; then
        echo "[$APP_NAME] 启动成功 (PID: $(cat $PID_FILE), 端口: $PORT)"
    else
        echo "[$APP_NAME] 启动可能失败，请检查 $LOG_FILE"
        rm -f "$PID_FILE"
        return 1
    fi
}

# 主逻辑
case "${1:-}" in
    start)
        do_start
        ;;
    stop)
        do_stop
        ;;
    restart)
        do_stop
        sleep 2
        do_start
        ;;
    status)
        do_status
        ;;
    *)
        echo "用法: $0 {start|stop|restart|status}"
        exit 1
        ;;
esac
