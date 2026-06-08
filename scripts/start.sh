#!/bin/bash

# ==============================================================
# SealSuite Operation 启动脚本
# 功能：启动服务，支持前台和后台运行模式
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
BIN_PATH="./bin/${PROJECT_NAME}"
LOG_DIR="./logs"
PID_FILE="${LOG_DIR}/${PROJECT_NAME}.pid"
LOG_FILE="${LOG_DIR}/app.log"

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

# 检查项目目录
check_project_dir() {
    if [ ! -f "go.mod" ]; then
        print_error "未找到 go.mod 文件，请在项目根目录运行此脚本"
        exit 1
    fi
}

# 检查二进制文件
check_binary() {
    if [ ! -f "${BIN_PATH}" ]; then
        print_warning "未找到可执行文件，尝试编译..."
        ./scripts/build.sh
    fi
}

# 检查配置文件
check_config() {
    if [ ! -f "config.yaml" ]; then
        print_error "未找到配置文件 config.yaml"
        exit 1
    fi
}

# 创建日志目录
create_log_dir() {
    mkdir -p "${LOG_DIR}"
}

# 检查进程是否已在运行
check_process_running() {
    if [ -f "${PID_FILE}" ]; then
        PID=$(cat "${PID_FILE}")
        if kill -0 "${PID}" 2>/dev/null; then
            print_warning "服务已在运行 (PID: ${PID})"
            print_info "如需重启，请先运行: ./scripts/stop.sh"
            return 1
        else
            print_warning "PID 文件存在但进程不存在，清理旧文件"
            rm -f "${PID_FILE}"
        fi
    fi
    return 0
}

# 前台运行模式
run_foreground() {
    print_info "启动服务（前台模式）..."
    print_info "按 Ctrl+C 停止服务"
    echo ""
    "${BIN_PATH}"
}

# 后台运行模式
run_background() {
    print_info "启动服务（后台模式）..."
    
    # 启动服务
    nohup "${BIN_PATH}" > "${LOG_FILE}" 2>&1 &
    PID=$!
    
    # 保存 PID
    echo "${PID}" > "${PID_FILE}"
    
    # 等待一小段时间检查是否启动成功
    sleep 2
    
    if kill -0 "${PID}" 2>/dev/null; then
        print_success "服务启动成功！"
        print_info "PID: ${PID}"
        print_info "日志文件: ${LOG_FILE}"
        print_info "PID 文件: ${PID_FILE}"
        echo ""
        print_info "查看日志: tail -f ${LOG_FILE}"
        print_info "停止服务: ./scripts/stop.sh"
    else
        print_error "服务启动失败，请检查日志: ${LOG_FILE}"
        tail -50 "${LOG_FILE}" 2>/dev/null || true
        rm -f "${PID_FILE}"
        exit 1
    fi
}

# 显示帮助信息
show_help() {
    echo "用法: $0 [选项]"
    echo ""
    echo "选项:"
    echo "  -f, --foreground    前台运行模式（默认）"
    echo "  -d, --daemon        后台运行模式"
    echo "  -h, --help          显示帮助信息"
    echo ""
    echo "示例:"
    echo "  $0                    前台运行"
    echo "  $0 -f                 前台运行"
    echo "  $0 -d                 后台运行"
    echo ""
}

# 主函数
main() {
    local mode="foreground"
    
    # 解析参数
    while [[ $# -gt 0 ]]; do
        case $1 in
            -f|--foreground)
                mode="foreground"
                shift
                ;;
            -d|--daemon)
                mode="daemon"
                shift
                ;;
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
    echo "   SealSuite Operation 启动脚本"
    echo "=========================================="
    echo ""
    
    check_project_dir
    check_binary
    check_config
    create_log_dir
    check_process_running || exit 0
    
    if [ "${mode}" = "daemon" ]; then
        run_background
    else
        run_foreground
    fi
}

# 运行主函数
main "$@"
