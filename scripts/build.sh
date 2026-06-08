#!/bin/bash

# ==============================================================
# SealSuite Operation 编译构建脚本
# 功能：编译 Go 项目为可执行文件，支持多平台
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
MAIN_PATH="./cmd/main.go"
OUTPUT_DIR="./bin"

# 默认目标平台（当前系统）
TARGET_OS=$(uname | tr '[:upper:]' '[:lower:]')
TARGET_ARCH=$(uname -m)
if [ "${TARGET_ARCH}" = "x86_64" ]; then
    TARGET_ARCH="amd64"
elif [ "${TARGET_ARCH}" = "arm64" ]; then
    TARGET_ARCH="arm64"
fi

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

# 检查 Go 是否安装
check_go_installed() {
    if ! command -v go &> /dev/null; then
        print_error "Go 未安装，请先安装 Go 1.21 或更高版本"
        print_info "下载地址: https://golang.org/dl/"
        exit 1
    fi
    
    GO_VERSION=$(go version | awk '{print $3}')
    print_info "检测到 Go 版本: ${GO_VERSION}"
}

# 检查项目目录
check_project_dir() {
    if [ ! -f "go.mod" ]; then
        print_error "未找到 go.mod 文件，请在项目根目录运行此脚本"
        exit 1
    fi
}

# 下载依赖
download_dependencies() {
    print_info "正在下载依赖包..."
    go mod download
    print_success "依赖下载完成"
}

# 编译项目
build_project() {
    local os=$1
    local arch=$2
    local output_name="${PROJECT_NAME}"
    
    if [ "${os}" = "windows" ]; then
        output_name="${PROJECT_NAME}.exe"
    fi
    
    local output_path="${OUTPUT_DIR}/${os}_${arch}/${output_name}"
    
    print_info "开始编译 ${os}/${arch} 版本..."
    
    mkdir -p "${OUTPUT_DIR}/${os}_${arch}"
    
    CGO_ENABLED=0 GOOS="${os}" GOARCH="${arch}" go build \
        -ldflags "-s -w" \
        -o "${output_path}" \
        "${MAIN_PATH}"
    
    if [ $? -eq 0 ]; then
        print_success "编译成功！"
        print_info "可执行文件位置: ${output_path}"
        
        if command -v file &> /dev/null; then
            FILE_INFO=$(file "${output_path}")
            print_info "文件信息: ${FILE_INFO}"
        fi
        
        FILE_SIZE=$(du -h "${output_path}" | cut -f1)
        print_info "文件大小: ${FILE_SIZE}"
        
        # 如果是当前平台，创建符号链接
        if [ "${os}" = "${TARGET_OS}" ] && [ "${arch}" = "${TARGET_ARCH}" ]; then
            mkdir -p "${OUTPUT_DIR}"
            rm -f "${OUTPUT_DIR}/${PROJECT_NAME}" "${OUTPUT_DIR}/${PROJECT_NAME}.exe"
            ln -s "${os}_${arch}/${output_name}" "${OUTPUT_DIR}/${PROJECT_NAME}"
            print_info "已创建当前平台的符号链接: ${OUTPUT_DIR}/${PROJECT_NAME}"
        fi
    else
        print_error "编译失败！"
        exit 1
    fi
}

# 显示帮助信息
show_help() {
    echo "用法: $0 [选项]"
    echo ""
    echo "选项:"
    echo "  -l, --linux           编译 Linux amd64 版本（默认）"
    echo "  -m, --macos           编译 macOS arm64 版本"
    echo "  -x, --macos-x86       编译 macOS x86_64 版本"
    echo "  -w, --windows         编译 Windows amd64 版本"
    echo "  -a, --all             编译所有平台版本"
    echo "  -c, --current         编译当前平台版本"
    echo "  -h, --help            显示帮助信息"
    echo ""
    echo "示例:"
    echo "  $0                    编译 Linux amd64 版本（默认）"
    echo "  $0 -m                 编译 macOS arm64 版本"
    echo "  $0 -c                 编译当前平台版本"
    echo "  $0 -a                 编译所有平台版本"
    echo ""
    echo "当前平台: ${TARGET_OS}/${TARGET_ARCH}"
}

# 主函数
main() {
    local build_all=false
    local build_current=false
    local targets=()
    
    # 解析参数
    while [[ $# -gt 0 ]]; do
        case $1 in
            -l|--linux)
                targets+=("linux/amd64")
                shift
                ;;
            -m|--macos)
                targets+=("darwin/arm64")
                shift
                ;;
            -x|--macos-x86)
                targets+=("darwin/amd64")
                shift
                ;;
            -w|--windows)
                targets+=("windows/amd64")
                shift
                ;;
            -a|--all)
                build_all=true
                shift
                ;;
            -c|--current)
                build_current=true
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
    
    # 如果没有指定目标，默认编译 Linux amd64
    if [ ${#targets[@]} -eq 0 ] && [ "${build_all}" = false ] && [ "${build_current}" = false ]; then
        targets=("linux/amd64")
    fi
    
    # 如果指定了 all，编译所有平台
    if [ "${build_all}" = true ]; then
        targets=("linux/amd64" "darwin/arm64" "darwin/amd64" "windows/amd64")
    fi
    
    # 如果指定了 current，编译当前平台
    if [ "${build_current}" = true ]; then
        targets=("${TARGET_OS}/${TARGET_ARCH}")
    fi
    
    echo "=========================================="
    echo "   SealSuite Operation 编译构建脚本"
    echo "=========================================="
    echo ""
    
    check_go_installed
    check_project_dir
    download_dependencies
    
    # 编译所有目标平台
    for target in "${targets[@]}"; do
        os=$(echo "${target}" | cut -d'/' -f1)
        arch=$(echo "${target}" | cut -d'/' -f2)
        build_project "${os}" "${arch}"
        echo ""
    done
    
    print_success "构建流程完成！"
    echo ""
    print_info "运行方式:"
    print_info "  ./scripts/start.sh"
    echo ""
}

# 运行主函数
main "$@"
