#!/bin/bash

# ==============================================================
# SealSuite Operation 停止脚本
# 功能：停止正在运行的服务
# ==============================================================

set -e

# 颜色定义
RED='\033[0;31m'
GREEN='\033[0;32m'
YELLOW='\033[1;33m'
BLUE='\033[0;34m'
NC='\033[0m' # No Color

# 项目配置
PROJECT_NAME="sealsuite-operation"
LOG_DIR="./logs"
PID_FILE="${LOG_DIR}/${PROJECT_NAME}.pid"

# 打印信息函数
print_info() {
    echo -e "${BLUE}[INFO]${NC} $1"
}

print_success() {
    echo -e "${GREEN}[SUCCESS]${NC} $1"
}

print_error() {
    echo -e "${RED}[ERROR]${NC} $1"
}

print_warning() {
    echo -e "${YELLOW}[WARNING]${NC} $1"
}

# 停止服务
stop_service() {
    if [ ! -f "${PID_FILE}" ]; then
        print_warning "未找到 PID 文件，服务可能未运行"
        return 0
    fi
    
    PID=$(cat "${PID_FILE}")
    
    # 检查进程是否存在
    if ! kill -0 "${PID}" 2>/dev/null; then
        print_warning "进程 ${PID} 不存在，清理 PID 文件"
        rm -f "${PID_FILE}"
        return 0
    fi
    
    print_info "正在停止服务 (PID: ${PID})..."
    
    # 尝试优雅停止
    kill -TERM "${PID}" 2>/dev/null || true
    
    # 等待进程停止
    local count=0
    local max_wait=30
    
    while kill -0 "${PID}" 2>/dev/null && [ ${count} -lt ${max_wait} ]; do
        sleep 1
        count=$((count + 1))
        print_info "等待进程停止... (${count}/${max_wait})"
    done
    
    # 检查进程是否还在运行
    if kill -0 "${PID}" 2>/dev/null; then
        print_warning "进程未响应，强制停止..."
        kill -KILL "${PID}" 2>/dev/null || true
        sleep 2
    fi
    
    # 最终检查
    if kill -0 "${PID}" 2>/dev/null; then
        print_error "无法停止进程 ${PID}"
        return 1
    else
        print_success "服务已停止"
        rm -f "${PID_FILE}"
        return 0
    fi
}

# 显示帮助信息
show_help() {
    echo "用法: $0 [选项]"
    echo ""
    echo "选项:"
    echo "  -h, --help          显示帮助信息"
    echo ""
    echo "示例:"
    echo "  $0                    停止服务"
    echo ""
}

# 主函数
main() {
    # 解析参数
    while [[ $# -gt 0 ]]; do
        case $1 in
            -h|--help)
                show_help
                exit 0
                ;;
            *)
                print_error "未知参数: $1"
                show_help
                exit 1
                ;;
        esac
    done
    
    echo "=========================================="
    echo "   SealSuite Operation 停止脚本"
    echo "=========================================="
    echo ""
    
    stop_service
}

# 运行主函数
main "$@"
