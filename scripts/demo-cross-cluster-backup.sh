#!/bin/bash

# demo-cross-cluster-backup.sh - Demonstrate cross-cluster backup functionality
# This script shows how backups from one cluster can be accessed by another

set -e

# Colors for output
RED='\033[0;31m'
GREEN='\033[0;32m'
YELLOW='\033[1;33m'
BLUE='\033[0;34m'
NC='\033[0m' # No Color

# Configuration
CLUSTER1="kind-kind"
CLUSTER2="kind-backup-test"
MINIO_CONTAINER="workshop-minio"
TEST_NAMESPACE="demo-backup"
BUCKET_NAME="k8s-backups"

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

# Check if cluster exists
cluster_exists() {
    kind get clusters | grep -q "^$1$"
}

# Create second cluster if it doesn't exist
setup_second_cluster() {
    log_step "Setting up second cluster for cross-cluster demo..."
    
    if ! cluster_exists "backup-test"; then
        log_info "Creating second kind cluster..."
        kind create cluster --name backup-test
    else
        log_info "Second cluster already exists"
    fi
    
    # Switch to second cluster and deploy operator
    kubectl config use-context kind-backup-test
    
    # Check if operator is deployed
    if ! kubectl get namespace cluster-backup-operator-system &> /dev/null; then
        log_info "Deploying backup operator to second cluster..."
        cd cluster-backup-operator
        make deploy IMG=controller:latest
        cd ..
        
        # Wait for operator to be ready
        kubectl wait --for=condition=available --timeout=120s deployment/cluster-backup-operator-controller-manager -n cluster-backup-operator-system
    else
        log_info "Backup operator already deployed in second cluster"
    fi
}

# Create demo resources in first cluster
create_demo_resources_cluster1() {
    log_step "Creating demo resources in first cluster..."
    
    kubectl config use-context "$CLUSTER1"
    
    # Create demo namespace
    kubectl create namespace "$TEST_NAMESPACE" --dry-run=client -o yaml | kubectl apply -f -
    
    # Create demo application
    cat <<EOF | kubectl apply -f -
apiVersion: apps/v1
kind: Deployment
metadata:
  name: demo-app
  namespace: $TEST_NAMESPACE
  labels:
    app: demo-app
spec:
  replicas: 2
  selector:
    matchLabels:
      app: demo-app
  template:
    metadata:
      labels:
        app: demo-app
    spec:
      containers:
      - name: nginx
        image: nginx:1.21
        ports:
        - containerPort: 80
---
apiVersion: v1
kind: Service
metadata:
  name: demo-app-service
  namespace: $TEST_NAMESPACE
spec:
  selector:
    app: demo-app
  ports:
  - port: 80
    targetPort: 80
---
apiVersion: v1
kind: ConfigMap
metadata:
  name: demo-config
  namespace: $TEST_NAMESPACE
data:
  app.properties: |
    app.name=Demo Application
    app.version=1.0.0
    environment=production
  nginx.conf: |
    server {
        listen 80;
        location / {
            return 200 'Hello from Cluster 1!';
        }
    }
---
apiVersion: v1
kind: Secret
metadata:
  name: demo-secret
  namespace: $TEST_NAMESPACE
type: Opaque
data:
  database-url: cG9zdGdyZXNxbDovL2RiLmV4YW1wbGUuY29tOjU0MzIvZGVtb2RiCg==
  api-key: YWJjZGVmZ2hpams=
EOF
    
    log_info "Demo resources created in first cluster"
}

# Create backup in first cluster
create_backup_cluster1() {
    log_step "Creating backup in first cluster..."
    
    kubectl config use-context "$CLUSTER1"
    
    # Get Minio IP
    local minio_ip
    minio_ip=$(docker inspect "$MINIO_CONTAINER" | grep '"IPAddress"' | head -1 | cut -d '"' -f 4)
    
    # Note: Using plain credentials in backup spec for workshop simplicity
    
    # Create backup resource
    cat <<EOF | kubectl apply -f -
apiVersion: backup.cnd.dk/v1
kind: Backup
metadata:
  name: cross-cluster-demo-backup
  namespace: default
spec:
  source:
    namespace: "$TEST_NAMESPACE"
  schedule: "*/1 * * * *"  # Every minute for demo
  storageLocation:
    provider: "minio"
    bucket: "$BUCKET_NAME"
    endpoint: "http://${minio_ip}:9000"
    accessKey: "minioadmin"
    secretKey: "minioadmin123"
EOF
    
    log_info "Backup resource created in first cluster"
    
    # Wait for backup to complete
    log_info "Waiting for backup to complete..."
    local max_wait=180
    local wait_time=0
    
    while [ $wait_time -lt $max_wait ]; do
        local phase
        phase=$(kubectl get backup cross-cluster-demo-backup -o jsonpath='{.status.phase}' 2>/dev/null || echo "")
        
        case "$phase" in
            "Completed")
                log_info "✅ Backup completed successfully!"
                return 0
                ;;
            "Failed")
                log_error "❌ Backup failed!"
                kubectl describe backup cross-cluster-demo-backup
                return 1
                ;;
            *)
                log_info "Backup phase: $phase (waiting...)"
                ;;
        esac
        
        sleep 10
        wait_time=$((wait_time + 10))
    done
    
    log_error "Backup did not complete within expected time"
    return 1
}

# Verify backup files in Minio
verify_backup_files() {
    log_step "Verifying backup files in Minio..."
    
    local mc_cmd="docker run --rm --network kind minio/mc:latest"
    
    # Configure mc
    $mc_cmd alias set workshop "http://${MINIO_CONTAINER}:9000" "minioadmin" "minioadmin123" &> /dev/null
    
    # List backup files
    log_info "Backup files in Minio:"
    $mc_cmd ls "workshop/$BUCKET_NAME/backups/$TEST_NAMESPACE" --recursive
    
    return 0
}

# Access backup from second cluster
access_backup_cluster2() {
    log_step "Accessing backup from second cluster..."
    
    kubectl config use-context kind-backup-test
    
    # Get Minio IP
    local minio_ip
    minio_ip=$(docker inspect "$MINIO_CONTAINER" | grep '"IPAddress"' | head -1 | cut -d '"' -f 4)
    
    # Note: Using plain credentials in backup spec for workshop simplicity
    
    # Create a backup resource that can read from the same Minio
    cat <<EOF | kubectl apply -f -
apiVersion: backup.cnd.dk/v1
kind: Backup
metadata:
  name: cross-cluster-reader
  namespace: default
spec:
  source:
    namespace: "default"  # Different namespace for demo
  schedule: "0 0 * * *"   # Daily (won't trigger immediately)
  storageLocation:
    provider: "minio"
    bucket: "$BUCKET_NAME"
    endpoint: "http://${minio_ip}:9000"
    accessKey: "minioadmin"
    secretKey: "minioadmin123"
EOF
    
    log_info "Backup reader created in second cluster"
    
    # Demonstrate that both clusters can access the same Minio
    log_info "Both clusters can now access the same Minio storage!"
    
    return 0
}

# Show cross-cluster access demonstration
demonstrate_cross_cluster_access() {
    log_step "Demonstrating cross-cluster access..."
    
    local mc_cmd="docker run --rm --network kind minio/mc:latest"
    
    # Configure mc
    $mc_cmd alias set workshop "http://${MINIO_CONTAINER}:9000" "minioadmin" "minioadmin123" &> /dev/null
    
    echo ""
    echo "🌐 Cross-Cluster Backup Demonstration"
    echo "======================================"
    echo ""
    echo "📁 Backup files created by Cluster 1:"
    $mc_cmd ls "workshop/$BUCKET_NAME/backups/$TEST_NAMESPACE" --recursive | head -10
    echo ""
    echo "🔗 Both clusters can access the same Minio storage:"
    echo "   - Cluster 1 (kind-kind): Created the backup"
    echo "   - Cluster 2 (kind-backup-test): Can access the same storage"
    echo ""
    echo "💡 This demonstrates how multiple Kubernetes clusters can:"
    echo "   - Share the same backup storage"
    echo "   - Perform cross-cluster disaster recovery"
    echo "   - Migrate workloads between clusters"
    echo ""
}

# Cleanup demo resources
cleanup_demo() {
    log_step "Cleaning up demo resources..."
    
    # Cleanup first cluster
    kubectl config use-context "$CLUSTER1"
    kubectl delete backup cross-cluster-demo-backup --ignore-not-found=true
    kubectl delete namespace "$TEST_NAMESPACE" --ignore-not-found=true
    
    # Cleanup second cluster
    kubectl config use-context kind-backup-test
    kubectl delete backup cross-cluster-reader --ignore-not-found=true
    
    log_info "Demo cleanup completed"
}

# Show usage
show_usage() {
    echo "Usage: $0 [COMMAND]"
    echo ""
    echo "Demonstrate cross-cluster backup functionality"
    echo ""
    echo "Commands:"
    echo "  demo      Run complete cross-cluster demo (default)"
    echo "  cleanup   Clean up demo resources"
    echo "  help      Show this help message"
    echo ""
    echo "Prerequisites:"
    echo "  - Minio container running (./scripts/start-minio-docker.sh)"
    echo "  - First kind cluster with backup operator deployed"
    echo ""
}

# Main execution
main() {
    local command="${1:-demo}"
    
    case "$command" in
        demo)
            log_info "Starting cross-cluster backup demonstration..."
            
            # Check prerequisites
            if ! docker ps --format '{{.Names}}' | grep -q "^${MINIO_CONTAINER}$"; then
                log_error "Minio container is not running"
                log_info "Run: ./scripts/start-minio-docker.sh"
                exit 1
            fi
            
            if ! kubectl config get-contexts | grep -q "$CLUSTER1"; then
                log_error "First cluster ($CLUSTER1) not found"
                exit 1
            fi
            
            # Run demo steps
            setup_second_cluster
            create_demo_resources_cluster1
            create_backup_cluster1
            verify_backup_files
            access_backup_cluster2
            demonstrate_cross_cluster_access
            
            echo ""
            log_info "🎉 Cross-cluster backup demonstration completed!"
            echo ""
            echo "Next steps:"
            echo "  - Explore backup files in Minio console: http://localhost:9001"
            echo "  - Try creating restore functionality (Stage 6)"
            echo "  - Run cleanup: $0 cleanup"
            ;;
        cleanup)
            cleanup_demo
            ;;
        help|--help|-h)
            show_usage
            ;;
        *)
            log_error "Unknown command: $command"
            show_usage
            exit 1
            ;;
    esac
}

# Run main function if script is executed directly
if [[ "${BASH_SOURCE[0]}" == "${0}" ]]; then
    main "$@"
fi