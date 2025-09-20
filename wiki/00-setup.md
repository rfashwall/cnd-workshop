# Workshop Setup Guide

Welcome to the Cloud Native Denmark 2025 workshop on building infrastructure tools with Kubernetes Operators and Go! This guide will help you set up your development environment.

## Prerequisites

Before starting the workshop, ensure you have access to:
- A GitHub account

## Development Environment Options

### Option 1: GitHub Codespaces (Recommended)

GitHub Codespaces provides a pre-configured development environment that includes all necessary tools.

1. **Open in Codespaces**
   - Navigate to the workshop repository on GitHub
   - Click the green "Code" button
   - Select "Codespaces" tab
   - Click "Create codespace on main"

2. **Wait for Environment Setup**
   - The devcontainer will automatically install all required tools
   - This process takes 2-3 minutes
   - You'll see a VS Code interface in your browser when ready

3. **Verify Installation**
   ```bash
   # Check Go installation
   go version
   
   # Check kubectl
   kubectl version --client
   
   # Check operator-sdk
   operator-sdk version
   
   # Check kind
   kind version
   
   # Check helm
   helm version
   
   # Check docker
   docker version
   ```

### Option 2: Local Development

If you prefer to work locally, install the following tools:

#### Required Tools

1. **Go (1.21 or later)**
   ```bash
   # Download from https://golang.org/dl/
   # Verify installation
   go version
   ```

2. **kubectl**
   ```bash
   # macOS
   brew install kubectl
   
   # Linux
   curl -LO "https://dl.k8s.io/release/$(curl -L -s https://dl.k8s.io/release/stable.txt)/bin/linux/amd64/kubectl"
   chmod +x kubectl
   sudo mv kubectl /usr/local/bin/
   
   # Verify
   kubectl version --client
   ```

3. **kind (Kubernetes in Docker)**
   ```bash
   # macOS
   brew install kind
   
   # Linux
   curl -Lo ./kind https://kind.sigs.k8s.io/dl/v0.20.0/kind-linux-amd64
   chmod +x ./kind
   sudo mv ./kind /usr/local/bin/kind
   
   # Verify
   kind version
   ```

4. **Operator SDK**
   ```bash
   # Download latest release
   export ARCH=$(case $(uname -m) in x86_64) echo -n amd64 ;; aarch64) echo -n arm64 ;; *) echo -n $(uname -m) ;; esac)
   export OS=$(uname | awk '{print tolower($0)}')
   export OPERATOR_SDK_DL_URL=https://github.com/operator-framework/operator-sdk/releases/download/v1.32.0
   curl -LO ${OPERATOR_SDK_DL_URL}/operator-sdk_${OS}_${ARCH}
   chmod +x operator-sdk_${OS}_${ARCH}
   sudo mv operator-sdk_${OS}_${ARCH} /usr/local/bin/operator-sdk
   
   # Verify
   operator-sdk version
   ```

5. **Helm**
   ```bash
   # macOS
   brew install helm
   
   # Linux
   curl https://raw.githubusercontent.com/helm/helm/main/scripts/get-helm-3 | bash
   
   # Verify
   helm version
   ```

6. **Docker**
   - Install Docker Desktop from https://www.docker.com/products/docker-desktop
   - Ensure Docker daemon is running

## Workshop Environment Setup

### 1. Create Kubernetes Cluster

We'll use kind to create a local Kubernetes cluster:

```bash
# Run the setup script
./scripts/setup-kind.sh

# Verify cluster is running
kubectl cluster-info
kubectl get nodes
```

### 2. Start Minio for Backup Storage

Minio will serve as our backup storage system:

```bash
# Start Minio container
./scripts/start-minio-docker.sh

# Verify Minio is accessible
curl http://localhost:9000/minio/health/live
```

### 3. Install Additional Tools

Run the tool installation script:

```bash
# Install remaining tools
./scripts/install-tools.sh
```

## Troubleshooting

### Common Issues

**Issue: Codespaces fails to start**
- Solution: Try refreshing the page or creating a new codespace
- Alternative: Use local development setup

**Issue: kind cluster creation fails**
- Check Docker is running: `docker ps`
- Ensure no port conflicts on 8080, 9000
- Try: `kind delete cluster --name workshop` then recreate

**Issue: Cannot access Minio**
- Verify Docker container is running: `docker ps | grep minio`
- Check port 9000 is not in use: `lsof -i :9000`
- Restart container: `docker restart minio`

**Issue: kubectl commands fail**
- Verify cluster context: `kubectl config current-context`
- Check cluster status: `kubectl cluster-info`
- Recreate cluster if needed

**Issue: Go modules not working**
- Ensure GOPATH is set correctly
- Run: `go mod tidy` in project directory
- Check Go version: `go version`

### Getting Help

If you encounter issues during the workshop:
1. Check this troubleshooting section
2. Ask the instructor for assistance
3. Use checkpoint branches to recover from a known good state

## Next Steps

Once your environment is set up, proceed to:
- [01 - Kubernetes Introduction](01-intro-k8s.md)

---

**Navigation:**
- **Next:** [Kubernetes Introduction →](01-intro-k8s.md)
- **Home:** [Workshop Overview](../README.md)