#!/bin/bash

# setup-kind.sh - Create and configure local Kubernetes cluster using kind
# This script sets up a kind cluster for the workshop with proper configuration

set -e  # Exit on any error

# Configuration
CLUSTER_NAME="workshop-cluster"
KIND_CONFIG_FILE="/tmp/kind-config.yaml"

# Colors for output
RED='\033[0;31m'
GREEN='\033[0;32m'
YELLOW='\033[1;33m'
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

# Check if kind is installed
check_kind_installed() {
    if ! command -v kind &> /dev/null; then
        log_error "kind is not installed. Please install kind first."
        log_info "Installation instructions: https://kind.sigs.k8s.io/docs/user/quick-start/#installation"
        exit 1
    fi
    log_info "kind is installed: $(kind version)"
}

# Check if Docker is running
check_docker_running() {
    if ! docker info &> /dev/null; then
        log_error "Docker is not running. Please start Docker first."
        exit 1
    fi
    log_info "Docker is running"
}

# Create kind configuration file
create_kind_config() {
    log_info "Creating kind configuration file..."
    cat > "$KIND_CONFIG_FILE" << EOF
kind: Cluster
apiVersion: kind.x-k8s.io/v1alpha4
nodes:
- role: control-plane
  kubeadmConfigPatches:
  - |
    kind: InitConfiguration
    nodeRegistration:
      kubeletExtraArgs:
        node-labels: "ingress-ready=true"
  extraPortMappings:
  - containerPort: 80
    hostPort: 80
    protocol: TCP
  - containerPort: 443
    hostPort: 443
    protocol: TCP
  - containerPort: 30000
    hostPort: 30000
    protocol: TCP
- role: worker
EOF
    log_info "Kind configuration created at $KIND_CONFIG_FILE"
}

# Check if cluster already exists
check_existing_cluster() {
    if kind get clusters | grep -q "^${CLUSTER_NAME}$"; then
        log_warn "Cluster '$CLUSTER_NAME' already exists"
        read -p "Do you want to delete and recreate it? (y/N): " -n 1 -r
        echo
        if [[ $REPLY =~ ^[Yy]$ ]]; then
            log_info "Deleting existing cluster..."
            kind delete cluster --name "$CLUSTER_NAME"
        else
            log_info "Using existing cluster"
            return 0
        fi
    fi
}

# Create the kind cluster
create_cluster() {
    log_info "Creating kind cluster '$CLUSTER_NAME'..."
    if kind create cluster --name "$CLUSTER_NAME" --config "$KIND_CONFIG_FILE"; then
        log_info "Cluster created successfully"
    else
        log_error "Failed to create cluster"
        exit 1
    fi
}

# Verify cluster is ready
verify_cluster() {
    log_info "Verifying cluster is ready..."
    
    # Wait for cluster to be ready
    local max_attempts=30
    local attempt=1
    
    while [ $attempt -le $max_attempts ]; do
        if kubectl cluster-info --context "kind-${CLUSTER_NAME}" &> /dev/null; then
            log_info "Cluster is ready!"
            break
        fi
        
        log_info "Waiting for cluster to be ready... (attempt $attempt/$max_attempts)"
        sleep 5
        ((attempt++))
    done
    
    if [ $attempt -gt $max_attempts ]; then
        log_error "Cluster failed to become ready within expected time"
        exit 1
    fi
    
    # Display cluster info
    log_info "Cluster information:"
    kubectl cluster-info --context "kind-${CLUSTER_NAME}"
    
    # Display nodes
    log_info "Cluster nodes:"
    kubectl get nodes --context "kind-${CLUSTER_NAME}"
}

# Set kubectl context
set_kubectl_context() {
    log_info "Setting kubectl context to kind-${CLUSTER_NAME}..."
    kubectl config use-context "kind-${CLUSTER_NAME}"
    log_info "Current context: $(kubectl config current-context)"
}

# Install ingress controller (useful for workshop)
install_ingress() {
    log_info "Installing NGINX Ingress Controller..."
    kubectl apply -f https://raw.githubusercontent.com/kubernetes/ingress-nginx/main/deploy/static/provider/kind/deploy.yaml
    
    log_info "Waiting for ingress controller to be ready..."
    kubectl wait --namespace ingress-nginx \
        --for=condition=ready pod \
        --selector=app.kubernetes.io/component=controller \
        --timeout=90s
    
    log_info "Ingress controller installed successfully"
}

# Cleanup function
cleanup() {
    if [ -f "$KIND_CONFIG_FILE" ]; then
        rm -f "$KIND_CONFIG_FILE"
        log_info "Cleaned up temporary configuration file"
    fi
}

# Main execution
main() {
    log_info "Starting kind cluster setup for workshop..."
    
    # Set up cleanup trap
    trap cleanup EXIT
    
    # Pre-flight checks
    check_kind_installed
    check_docker_running
    
    # Create configuration
    create_kind_config
    
    # Handle existing cluster
    check_existing_cluster
    
    # Create cluster if needed
    if ! kind get clusters | grep -q "^${CLUSTER_NAME}$"; then
        create_cluster
    fi
    
    # Verify and configure
    verify_cluster
    set_kubectl_context
    install_ingress
    
    log_info "Kind cluster setup completed successfully!"
    log_info "Cluster name: $CLUSTER_NAME"
    log_info "Context: kind-${CLUSTER_NAME}"
    log_info ""
    log_info "To delete this cluster later, run:"
    log_info "  kind delete cluster --name $CLUSTER_NAME"
}

# Run main function if script is executed directly
if [[ "${BASH_SOURCE[0]}" == "${0}" ]]; then
    main "$@"
fi