#!/bin/bash

# ==============================================================
# SealSuite Operation 状态检查脚本
# 功能：检查服务运行状态
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

# 检查服务状态
check_status() {
    echo "=========================================="
    echo "   SealSuite Operation 状态检查"
    echo "=========================================="
    echo ""
    
    local is_running=0
    
    # 检查 PID 文件
    if [ -f "${PID_FILE}" ]; then
        PID=$(cat "${PID_FILE}")
        
        if kill -0 "${PID}" 2>/dev/null; then
            print_success "服务正在运行"
            print_info "PID: ${PID}"
            
            # 获取进程信息
            if command -v ps &> /dev/null; then
                PROCESS_INFO=$(ps -p "${PID}" -o pid,ppid,etime,cmd --no-headers 2>/dev/null || true)
                if [ -n "${PROCESS_INFO}" ]; then
                    print_info "进程信息:"
                    echo "  ${PROCESS_INFO}"
                fi
            fi
            
            is_running=1
        else
            print_warning "PID 文件存在但进程不存在"
            rm -f "${PID_FILE}"
        fi
    else
        print_warning "服务未运行（无 PID 文件）"
    fi
    
    echo ""
    
    # 检查日志文件
    if [ -f "${LOG_FILE}" ]; then
        print_info "日志文件: ${LOG_FILE}"
        LOG_SIZE=$(du -h "${LOG_FILE}" | cut -f1)
        print_info "日志大小: ${LOG_SIZE}"
        
        # 显示最新几行日志
        echo ""
        print_info "最新日志（最后 10 行）:"
        echo "----------------------------------------"
        tail -10 "${LOG_FILE}" 2>/dev/null || true
        echo "----------------------------------------"
    else
        print_warning "日志文件不存在"
    fi
    
    echo ""
    
    if [ ${is_running} -eq 1 ]; then
        print_success "服务状态: 运行中"
    else
        print_warning "服务状态: 已停止"
    fi
    
    echo ""
}

# 显示帮助信息
show_help() {
    echo "用法: $0 [选项]"
    echo ""
    echo "选项:"
    echo "  -h, --help          显示帮助信息"
    echo ""
    echo "示例:"
    echo "  $0                    查看服务状态"
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
    
    check_status
}

# 运行主函数
main "$@"
