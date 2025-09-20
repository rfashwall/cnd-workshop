# Cloud Native Denmark 2025 Workshop

## Building Infrastructure Tools with Kubernetes Operators and Go

Welcome to the hands-on workshop on building Kubernetes Operators with Go! This 2-hour intensive session will guide you through creating production-ready infrastructure management tools using the Operator pattern.

### 🎯 Workshop Overview

In this workshop, you'll learn to build powerful infrastructure management tools using Kubernetes Operators written in Go. We'll create a complete backup and restore system for Kubernetes resources, demonstrating real-world Operator development patterns and best practices.

**Duration**: 2 hours  
**Prerequisites**: Basic knowledge of Kubernetes and Go  
**Format**: Hands-on coding with GitHub Codespaces

### 🚀 Quick Start with GitHub Codespaces

1. **Fork this repository** to your GitHub account
2. **Create a Codespace**:
   - Click the green "Code" button
   - Select "Codespaces" tab
   - Click "Create codespace on main"
3. **Wait for setup** (2-3 minutes for automatic tool installation)
4. **Verify installation** by running: `make verify-setup`

### 📚 Workshop Stages

Follow these stages in order. Each stage has a corresponding checkpoint branch for recovery:

| Stage | Topic | Duration | Checkpoint Branch | Wiki Page |
|-------|-------|----------|-------------------|-----------|
| 0 | [Environment Setup](wiki/00-setup.md) | 10 min | `main` | [Setup Guide](wiki/00-setup.md) |
| 1 | [Kubernetes Introduction](wiki/01-intro-k8s.md) | 15 min | `checkpoint-01` | [K8s Basics](wiki/01-intro-k8s.md) |
| 2 | [Controller Patterns](wiki/02-controllers.md) | 15 min | `checkpoint-02` | [Controllers](wiki/02-controllers.md) |
| 3 | [Operator SDK Basics](wiki/03-operator-sdk.md) | 15 min | `checkpoint-03` | [Operator SDK](wiki/03-operator-sdk.md) |
| 4 | [Backup Controller](wiki/04-backup-controller.md) | 20 min | `checkpoint-04` | [Backup Implementation](wiki/04-backup-controller.md) |
| 5 | [Minio Integration](wiki/05-minio-integration.md) | 15 min | `checkpoint-05` | [Minio Setup](wiki/05-minio-integration.md) |
| 6 | [Restore Controller](wiki/06-restore-controller.md) | 15 min | `checkpoint-06` | [Restore Implementation](wiki/06-restore-controller.md) |
| 7 | [Testing Strategies](wiki/07-testing.md) | 10 min | `checkpoint-07` | [Testing Guide](wiki/07-testing.md) |
| 8 | [Advanced Features](wiki/08-advanced-features.md) | 5 min | `checkpoint-08` | [Secrets & ConfigMaps](wiki/08-advanced-features.md) |

### 🛠️ Tech Stack

- **Go 1.21+** - Programming language
- **Kubernetes** - Container orchestration platform
- **Operator SDK** - Framework for building Operators
- **kind** - Local Kubernetes clusters
- **Minio** - S3-compatible object storage
- **Helm** - Kubernetes package manager
- **GitHub Codespaces** - Cloud development environment

### 🔧 Manual Setup (Alternative to Codespaces)

If you prefer local development or Codespaces isn't available:

```bash
# Clone the repository
git clone https://github.com/your-username/workshop-k8s-operators.git
cd workshop-k8s-operators

# Run setup scripts
./scripts/install-tools.sh
./scripts/setup-kind.sh
./scripts/start-minio-docker.sh

# Verify installation
make verify-setup
```

### 🆘 Troubleshooting

#### Common Codespaces Issues

**Problem**: Codespace fails to start or times out
- **Solution**: Try creating a new Codespace or use a different machine type
- **Alternative**: Use local development setup

**Problem**: Tools not installed after Codespace creation
- **Solution**: Run `./scripts/install-tools.sh` manually
- **Check**: Verify `.devcontainer/post-create.sh` completed successfully

**Problem**: Cannot access Minio from kind cluster
- **Solution**: Restart Minio container: `./scripts/start-minio-docker.sh`
- **Check**: Verify Docker is running and ports are available

#### Git and Branch Issues

**Problem**: Cannot checkout checkpoint branch
```bash
# Clean your working directory
git stash
git checkout checkpoint-XX
```

**Problem**: Merge conflicts when switching branches
```bash
# Reset to clean state
git reset --hard HEAD
git clean -fd
git checkout checkpoint-XX
```

#### Kubernetes Cluster Issues

**Problem**: kind cluster not responding
```bash
# Restart the cluster
kind delete cluster --name workshop
./scripts/setup-kind.sh
```

**Problem**: kubectl commands fail
```bash
# Verify cluster status
kubectl cluster-info
kind get clusters

# If cluster missing, recreate
./scripts/setup-kind.sh
```

#### Operator Development Issues

**Problem**: Operator fails to deploy
- **Check**: CRD definitions are valid: `kubectl get crd`
- **Verify**: RBAC permissions are correct
- **Debug**: Check operator logs: `kubectl logs -n operator-system deployment/controller-manager`

**Problem**: Controller not reconciling resources
- **Check**: Controller manager is running
- **Verify**: Custom resources are created correctly
- **Debug**: Add logging to reconcile function

### 🎯 Learning Objectives

By the end of this workshop, you will:

- ✅ Understand Kubernetes Operator patterns and architecture
- ✅ Build custom controllers using Operator SDK
- ✅ Implement backup and restore functionality for Kubernetes resources
- ✅ Integrate external storage systems (Minio) with Operators
- ✅ Write comprehensive tests for Operator code
- ✅ Handle secrets and ConfigMaps in production Operators
- ✅ Deploy and manage Operators in Kubernetes clusters

### 📖 Additional Resources

- [Kubernetes Operator Documentation](https://kubernetes.io/docs/concepts/extend-kubernetes/operator/)
- [Operator SDK Documentation](https://sdk.operatorframework.io/)
- [Go Client for Kubernetes](https://github.com/kubernetes/client-go)
- [Kubebuilder Book](https://book.kubebuilder.io/)

### 🤝 Getting Help

During the workshop:
1. **Raise your hand** for immediate assistance
2. **Check troubleshooting section** above for common issues
3. **Use checkpoint branches** to catch up if you fall behind

### 📝 Workshop Feedback

After completing the workshop, please provide feedback to help us improve future sessions.

---

**Ready to start?** Begin with [Stage 0: Environment Setup](wiki/00-setup.md)

