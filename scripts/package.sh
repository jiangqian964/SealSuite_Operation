#!/bin/bash

# ==============================================================
# SealSuite Operation 打包脚本
# 功能：打包项目用于部署到 Linux 服务器
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
VERSION="1.0.0"
OUTPUT_DIR="./dist"

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

# 创建输出目录
create_output_dir() {
    mkdir -p "${OUTPUT_DIR}"
}

# 打包完整项目（包含源代码）
package_full() {
    print_info "正在打包完整项目..."
    
    local timestamp=$(date +%Y%m%d_%H%M%S)
    local package_name="${PROJECT_NAME}_full_${VERSION}_${timestamp}.tar.gz"
    local package_path="${OUTPUT_DIR}/${package_name}"
    
    # 创建临时目录
    local temp_dir=$(mktemp -d -t sealsuite-XXXXXX)
    trap 'rm -rf "${temp_dir}"' EXIT
    
    # 复制文件
    cp -r . "${temp_dir}/${PROJECT_NAME}/"
    
    # 清理不必要的文件
    rm -rf "${temp_dir}/${PROJECT_NAME}/.git" 2>/dev/null || true
    rm -rf "${temp_dir}/${PROJECT_NAME}/.DS_Store" 2>/dev/null || true
    rm -rf "${temp_dir}/${PROJECT_NAME}/dist" 2>/dev/null || true
    rm -rf "${temp_dir}/${PROJECT_NAME}/bin" 2>/dev/null || true
    rm -rf "${temp_dir}/${PROJECT_NAME}/logs" 2>/dev/null || true
    
    # 创建压缩包
    cd "${temp_dir}" && tar -czf "${package_path}" "${PROJECT_NAME}/" && cd - >/dev/null
    
    local package_size=$(du -h "${package_path}" | cut -f1)
    
    print_success "完整项目打包完成"
    print_info "包名: ${package_name}"
    print_info "路径: ${package_path}"
    print_info "大小: ${package_size}"
    echo ""
    print_info "部署命令（在本地执行）:"
    print_info "  scp ${package_path} user@your-server:/tmp/"
    echo ""
    print_info "解压命令（在服务器执行）:"
    print_info "  cd /opt"
    print_info "  sudo tar -xzf /tmp/${package_name}"
    print_info "  cd ${PROJECT_NAME}"
    print_info "  sudo ./scripts/build.sh"
}

# 打包仅编译产物（推荐生产环境）
package_binary() {
    print_info "正在打包编译产物..."
    
    # 先编译
    if [ ! -f "bin/${PROJECT_NAME}" ]; then
        print_warning "未找到编译产物，正在编译..."
        ./scripts/build.sh
    fi
    
    local timestamp=$(date +%Y%m%d_%H%M%S)
    local package_name="${PROJECT_NAME}_binary_${VERSION}_${timestamp}.tar.gz"
    local package_path="${OUTPUT_DIR}/${package_name}"
    
    # 创建临时目录
    local temp_dir=$(mktemp -d -t sealsuite-XXXXXX)
    trap 'rm -rf "${temp_dir}"' EXIT
    
    # 复制必要文件
    mkdir -p "${temp_dir}/${PROJECT_NAME}/bin"
    mkdir -p "${temp_dir}/${PROJECT_NAME}/scripts"
    
    cp "bin/${PROJECT_NAME}" "${temp_dir}/${PROJECT_NAME}/bin/"
    cp "config.yaml" "${temp_dir}/${PROJECT_NAME}/"
    cp "scripts/start.sh" "${temp_dir}/${PROJECT_NAME}/scripts/"
    cp "scripts/stop.sh" "${temp_dir}/${PROJECT_NAME}/scripts/"
    cp "scripts/status.sh" "${temp_dir}/${PROJECT_NAME}/scripts/"
    cp "scripts/systemd/sealsuite-operation.service" "${temp_dir}/${PROJECT_NAME}/scripts/systemd/" 2>/dev/null || true
    
    # 创建压缩包
    cd "${temp_dir}" && tar -czf "${package_path}" "${PROJECT_NAME}/" && cd - >/dev/null
    
    local package_size=$(du -h "${package_path}" | cut -f1)
    
    print_success "编译产物打包完成"
    print_info "包名: ${package_name}"
    print_info "路径: ${package_path}"
    print_info "大小: ${package_size}"
    echo ""
    print_info "部署命令（在本地执行）:"
    print_info "  scp ${package_path} user@your-server:/tmp/"
    echo ""
    print_info "解压命令（在服务器执行）:"
    print_info "  cd /opt"
    print_info "  sudo tar -xzf /tmp/${package_name}"
    print_info "  cd ${PROJECT_NAME}"
    print_info "  chmod +x scripts/*.sh bin/${PROJECT_NAME}"
}

# 显示帮助信息
show_help() {
    echo "用法: $0 [选项]"
    echo ""
    echo "选项:"
    echo "  -f, --full          打包完整项目（含源代码）"
    echo "  -b, --binary        仅打包编译产物（推荐生产环境）"
    echo "  -a, --all           打包两种方式"
    echo "  -h, --help          显示帮助信息"
    echo ""
    echo "示例:"
    echo "  $0 -f              打包完整项目"
    echo "  $0 -b              仅打包编译产物"
    echo "  $0 -a              打包两种方式"
    echo ""
}

# 主函数
main() {
    local mode=""
    
    # 解析参数
    while [[ $# -gt 0 ]]; do
        case $1 in
            -f|--full)
                mode="full"
                shift
                ;;
            -b|--binary)
                mode="binary"
                shift
                ;;
            -a|--all)
                mode="all"
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
    
    # 默认打包完整项目
    if [ -z "${mode}" ]; then
        mode="full"
    fi
    
    echo "=========================================="
    echo "   SealSuite Operation 打包工具"
    echo "=========================================="
    echo ""
    
    create_output_dir
    
    if [ "${mode}" = "full" ] || [ "${mode}" = "all" ]; then
        package_full
        echo ""
    fi
    
    if [ "${mode}" = "binary" ] || [ "${mode}" = "all" ]; then
        package_binary
        echo ""
    fi
    
    print_success "打包完成！包文件在 ${OUTPUT_DIR}/ 目录中"
}

# 运行主函数
main "$@"
