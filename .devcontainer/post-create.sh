#!/bin/bash

# Post-create script for Kubernetes Operators Workshop
set -e

echo "=== Starting post-create script ==="
echo "Current directory: $(pwd)"
echo "Current user: $(whoami)"
echo "PATH: $PATH"
echo "Setting up workshop environment..."

# Function to check if command exists
command_exists() {
    command -v "$1" >/dev/null 2>&1
}

# Install operator-sdk
if ! command_exists operator-sdk; then
    echo "Installing operator-sdk..."
    export ARCH=$(case $(uname -m) in x86_64) echo -n amd64 ;; aarch64) echo -n arm64 ;; *) echo -n $(uname -m) ;; esac)
    export OS=$(uname | awk '{print tolower($0)}')
    export OPERATOR_SDK_DL_URL=https://github.com/operator-framework/operator-sdk/releases/download/v1.41.1
    echo "Downloading operator-sdk for ${OS}_${ARCH}..."
    curl -LO ${OPERATOR_SDK_DL_URL}/operator-sdk_${OS}_${ARCH} || { echo "Failed to download operator-sdk"; exit 1; }
    chmod +x operator-sdk_${OS}_${ARCH}
    sudo mv operator-sdk_${OS}_${ARCH} /usr/local/bin/operator-sdk
    echo "operator-sdk installed successfully"
else
    echo "operator-sdk already installed"
fi

# Install kind (Kubernetes in Docker)
if ! command_exists kind; then
    echo "Installing kind..."
    if [ $(uname -m) = x86_64 ]; then
        curl -Lo ./kind https://kind.sigs.k8s.io/dl/v0.20.0/kind-linux-amd64 || { echo "Failed to download kind"; exit 1; }
    elif [ $(uname -m) = aarch64 ]; then
        curl -Lo ./kind https://kind.sigs.k8s.io/dl/v0.20.0/kind-linux-arm64 || { echo "Failed to download kind"; exit 1; }
    else
        echo "Unsupported architecture for kind: $(uname -m)"
        exit 1
    fi
    chmod +x ./kind
    sudo mv ./kind /usr/local/bin/kind
    echo "kind installed successfully"
else
    echo "kind already installed"
fi

# Install kustomize
if ! command_exists kustomize; then
    echo "Installing kustomize..."
    curl -s "https://raw.githubusercontent.com/kubernetes-sigs/kustomize/master/hack/install_kustomize.sh" | bash || { echo "Failed to install kustomize"; exit 1; }
    sudo mv kustomize /usr/local/bin/
    echo "kustomize installed successfully"
else
    echo "kustomize already installed"
fi

# Verify installations
echo "=== Verifying tool installations ==="
echo "Go version:"
go version || echo "Go not found"
echo "kubectl version:"
kubectl version --client || echo "kubectl not found"
echo "Helm version:"
helm version || echo "Helm not found"
echo "Operator SDK version:"
operator-sdk version || echo "operator-sdk not found"
echo "Kind version:"
kind version || echo "kind not found"
echo "Kustomize version:"
kustomize version || echo "kustomize not found"

echo "=== Workshop environment setup complete! ==="