#!/bin/bash

# install-tools.sh - Automated installation of workshop tools
# Installs operator-sdk, kubectl, and helm for the Kubernetes operator workshop

set -e  # Exit on any error

# Configuration
OPERATOR_SDK_VERSION="v1.41.1"
KUBECTL_VERSION="v1.29.0"
HELM_VERSION="v3.14.0"
INSTALL_DIR="$HOME/.local/bin"

# Colors for output
RED='\033[0;31m'
GREEN='\033[0;32m'
YELLOW='\033[1;33m'
BLUE='\033[0;34m'
NC='\033[0m' # No Color

# Logging functions
log_info() {
    echo -e "${GREEN}[INFO]${NC} $1"
}

log_warn() {
    echo -e "${YELLOW}[WARN]${NC} $1"
}

log_error() {
    echo -e "${RED}[ERROR]${NC} $1"
}

log_step() {
    echo -e "${BLUE}[STEP]${NC} $1"
}

# Detect OS and architecture
detect_platform() {
    OS=$(uname -s | tr '[:upper:]' '[:lower:]')
    ARCH=$(uname -m)
    
    case $ARCH in
        x86_64)
            ARCH="amd64"
            ;;
        aarch64|arm64)
            ARCH="arm64"
            ;;
        *)
            log_error "Unsupported architecture: $ARCH"
            exit 1
            ;;
    esac
    
    log_info "Detected platform: $OS/$ARCH"
}

# Create installation directory
setup_install_dir() {
    if [ ! -d "$INSTALL_DIR" ]; then
        log_info "Creating installation directory: $INSTALL_DIR"
        mkdir -p "$INSTALL_DIR"
    fi
    
    # Add to PATH if not already there
    if [[ ":$PATH:" != *":$INSTALL_DIR:"* ]]; then
        log_info "Adding $INSTALL_DIR to PATH"
        echo "export PATH=\"$INSTALL_DIR:\$PATH\"" >> "$HOME/.bashrc"
        echo "export PATH=\"$INSTALL_DIR:\$PATH\"" >> "$HOME/.zshrc" 2>/dev/null || true
        export PATH="$INSTALL_DIR:$PATH"
    fi
}

# Check if tool is already installed
check_tool_installed() {
    local tool=$1
    local version_cmd=$2
    local expected_version=$3
    
    if command -v "$tool" &> /dev/null; then
        local current_version
        current_version=$($version_cmd 2>/dev/null || echo "unknown")
        log_info "$tool is already installed: $current_version"
        
        if [[ "$current_version" == *"$expected_version"* ]]; then
            return 0  # Tool is installed with correct version
        else
            log_warn "$tool version mismatch. Expected: $expected_version, Found: $current_version"
            return 1  # Tool needs update
        fi
    else
        log_info "$tool is not installed"
        return 1  # Tool not installed
    fi
}

# Download and install kubectl
install_kubectl() {
    log_step "Installing kubectl $KUBECTL_VERSION..."
    
    if check_tool_installed "kubectl" "kubectl version --client --short" "$KUBECTL_VERSION"; then
        log_info "kubectl is up to date, skipping installation"
        return 0
    fi
    
    local download_url="https://dl.k8s.io/release/${KUBECTL_VERSION}/bin/${OS}/${ARCH}/kubectl"
    local temp_file="/tmp/kubectl"
    
    log_info "Downloading kubectl from $download_url"
    if curl -L "$download_url" -o "$temp_file"; then
        chmod +x "$temp_file"
        mv "$temp_file" "$INSTALL_DIR/kubectl"
        log_info "kubectl installed successfully"
    else
        log_error "Failed to download kubectl"
        return 1
    fi
}

# Download and install helm
install_helm() {
    log_step "Installing Helm $HELM_VERSION..."
    
    if check_tool_installed "helm" "helm version --short" "$HELM_VERSION"; then
        log_info "Helm is up to date, skipping installation"
        return 0
    fi
    
    local download_url="https://get.helm.sh/helm-${HELM_VERSION}-${OS}-${ARCH}.tar.gz"
    local temp_dir="/tmp/helm-install"
    local temp_file="$temp_dir/helm.tar.gz"
    
    log_info "Downloading Helm from $download_url"
    mkdir -p "$temp_dir"
    
    if curl -L "$download_url" -o "$temp_file"; then
        cd "$temp_dir"
        tar -zxf "helm.tar.gz"
        chmod +x "${OS}-${ARCH}/helm"
        mv "${OS}-${ARCH}/helm" "$INSTALL_DIR/helm"
        rm -rf "$temp_dir"
        log_info "Helm installed successfully"
    else
        log_error "Failed to download Helm"
        rm -rf "$temp_dir"
        return 1
    fi
}

# Download and install operator-sdk
install_operator_sdk() {
    log_step "Installing Operator SDK $OPERATOR_SDK_VERSION..."
    
    if check_tool_installed "operator-sdk" "operator-sdk version" "$OPERATOR_SDK_VERSION"; then
        log_info "Operator SDK is up to date, skipping installation"
        return 0
    fi
    
    local download_url="https://github.com/operator-framework/operator-sdk/releases/download/${OPERATOR_SDK_VERSION}/operator-sdk_${OS}_${ARCH}"
    local temp_file="/tmp/operator-sdk"
    
    log_info "Downloading Operator SDK from $download_url"
    if curl -L "$download_url" -o "$temp_file"; then
        chmod +x "$temp_file"
        mv "$temp_file" "$INSTALL_DIR/operator-sdk"
        log_info "Operator SDK installed successfully"
    else
        log_error "Failed to download Operator SDK"
        return 1
    fi
}

# Install additional dependencies based on OS
install_dependencies() {
    log_step "Installing system dependencies..."
    
    case $OS in
        linux)
            # Check if we're in a container or have package manager access
            if command -v apt-get &> /dev/null; then
                log_info "Installing dependencies with apt-get..."
                sudo apt-get update -qq
                sudo apt-get install -y curl wget git make
            elif command -v yum &> /dev/null; then
                log_info "Installing dependencies with yum..."
                sudo yum install -y curl wget git make
            elif command -v apk &> /dev/null; then
                log_info "Installing dependencies with apk..."
                sudo apk add --no-cache curl wget git make
            else
                log_warn "No package manager found, assuming dependencies are already installed"
            fi
            ;;
        darwin)
            if command -v brew &> /dev/null; then
                log_info "Installing dependencies with Homebrew..."
                brew install curl wget git make
            else
                log_warn "Homebrew not found, assuming dependencies are already installed"
            fi
            ;;
        *)
            log_warn "Unknown OS: $OS, skipping dependency installation"
            ;;
    esac
}

# Verify all installations
verify_installations() {
    log_step "Verifying installations..."
    
    local all_good=true
    
    # Check kubectl
    if command -v kubectl &> /dev/null; then
        log_info "✓ kubectl: $(kubectl version --client --short 2>/dev/null || kubectl version --client)"
    else
        log_error "✗ kubectl not found"
        all_good=false
    fi
    
    # Check helm
    if command -v helm &> /dev/null; then
        log_info "✓ helm: $(helm version --short)"
    else
        log_error "✗ helm not found"
        all_good=false
    fi
    
    # Check operator-sdk
    if command -v operator-sdk &> /dev/null; then
        log_info "✓ operator-sdk: $(operator-sdk version)"
    else
        log_error "✗ operator-sdk not found"
        all_good=false
    fi
    
    if [ "$all_good" = true ]; then
        log_info "All tools installed successfully!"
        return 0
    else
        log_error "Some tools failed to install"
        return 1
    fi
}

# Display usage information
show_usage() {
    echo "Usage: $0 [OPTIONS]"
    echo ""
    echo "Install workshop tools: kubectl, helm, and operator-sdk"
    echo ""
    echo "Options:"
    echo "  -h, --help              Show this help message"
    echo "  --kubectl-version       Kubectl version to install (default: $KUBECTL_VERSION)"
    echo "  --helm-version          Helm version to install (default: $HELM_VERSION)"
    echo "  --operator-sdk-version  Operator SDK version to install (default: $OPERATOR_SDK_VERSION)"
    echo "  --install-dir           Installation directory (default: $INSTALL_DIR)"
    echo "  --skip-deps             Skip system dependency installation"
    echo ""
    echo "Examples:"
    echo "  $0                                    # Install all tools with default versions"
    echo "  $0 --kubectl-version v1.28.0         # Install with specific kubectl version"
    echo "  $0 --skip-deps                       # Skip system dependency installation"
}

# Parse command line arguments
parse_args() {
    local skip_deps=false
    
    while [[ $# -gt 0 ]]; do
        case $1 in
            -h|--help)
                show_usage
                exit 0
                ;;
            --kubectl-version)
                KUBECTL_VERSION="$2"
                shift 2
                ;;
            --helm-version)
                HELM_VERSION="$2"
                shift 2
                ;;
            --operator-sdk-version)
                OPERATOR_SDK_VERSION="$2"
                shift 2
                ;;
            --install-dir)
                INSTALL_DIR="$2"
                shift 2
                ;;
            --skip-deps)
                skip_deps=true
                shift
                ;;
            *)
                log_error "Unknown option: $1"
                show_usage
                exit 1
                ;;
        esac
    done
    
    return 0
}

# Main execution
main() {
    log_info "Starting workshop tools installation..."
    
    # Parse arguments
    parse_args "$@"
    
    # Detect platform
    detect_platform
    
    # Setup installation directory
    setup_install_dir
    
    # Install system dependencies (unless skipped)
    if [ "$skip_deps" != true ]; then
        install_dependencies
    fi
    
    # Install tools
    install_kubectl
    install_helm
    install_operator_sdk
    
    # Verify installations
    verify_installations
    
    log_info "Workshop tools installation completed!"
    log_info ""
    log_info "Tools installed in: $INSTALL_DIR"
    log_info "Make sure $INSTALL_DIR is in your PATH"
    log_info ""
    log_info "You may need to restart your shell or run:"
    log_info "  export PATH=\"$INSTALL_DIR:\$PATH\""
}

# Run main function if script is executed directly
if [[ "${BASH_SOURCE[0]}" == "${0}" ]]; then
    main "$@"
fi