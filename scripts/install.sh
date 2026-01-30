
#!/usr/bin/env bash
# scripts/install.sh
# Phantom Client 安装脚本

set -e

GITHUB_REPO="anthropics/phantom-client"
INSTALL_DIR="/usr/local/bin"
BINARY_NAME="phantom-client"

RED='\033[0;31m'
GREEN='\033[0;32m'
YELLOW='\033[0;33m'
NC='\033[0m'

log_info()  { echo -e "${GREEN}[✓]${NC} $1"; }
log_warn()  { echo -e "${YELLOW}[!]${NC} $1"; }
log_error() { echo -e "${RED}[✗]${NC} $1"; }

detect_arch() {
    case $(uname -m) in
        x86_64|amd64)  ARCH="amd64" ;;
        aarch64|arm64) ARCH="arm64" ;;
        *) log_error "不支持的架构: $(uname -m)"; exit 1 ;;
    esac
}

detect_os() {
    case $(uname -s) in
        Linux)  OS="linux" ;;
        Darwin) OS="darwin" ;;
        *) log_error "不支持的系统: $(uname -s)"; exit 1 ;;
    esac
}

get_latest_version() {
    curl -s "https://api.github.com/repos/${GITHUB_REPO}/releases/latest" | \
        grep '"tag_name"' | sed -E 's/.*"v?([^"]+)".*/\1/'
}

install() {
    detect_os
    detect_arch
    
    log_info "获取最新版本..."
    VERSION=$(get_latest_version)
    if [[ -z "$VERSION" ]]; then
        log_error "无法获取版本信息"
        exit 1
    fi
    log_info "版本: v${VERSION}"
    
    URL="https://github.com/${GITHUB_REPO}/releases/download/v${VERSION}/${BINARY_NAME}-${OS}-${ARCH}.tar.gz"
    
    log_info "下载中..."
    curl -fsSL -o /tmp/phantom-client.tar.gz "$URL"
    
    log_info "安装中..."
    tar -xzf /tmp/phantom-client.tar.gz -C /tmp
    
    if [[ -w "$INSTALL_DIR" ]]; then
        mv /tmp/${BINARY_NAME} "$INSTALL_DIR/"
    else
        sudo mv /tmp/${BINARY_NAME} "$INSTALL_DIR/"
    fi
    
    chmod +x "${INSTALL_DIR}/${BINARY_NAME}"
    rm /tmp/phantom-client.tar.gz
    
    log_info "安装完成!"
    echo ""
    echo "使用方法:"
    echo "  ${BINARY_NAME} -s <server:port> -psk <base64_psk>"
    echo ""
}

case "${1:-install}" in
    install) install ;;
    *) echo "用法: $0 install" ;;
esac
