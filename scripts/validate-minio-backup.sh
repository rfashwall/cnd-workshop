#!/bin/bash

# validate-minio-backup.sh - Validation script for Minio backup functionality
# This script validates the complete backup workflow for the workshop

set -e

# Colors for output
RED='\033[0;31m'
GREEN='\033[0;32m'
YELLOW='\033[1;33m'
BLUE='\033[0;34m'
NC='\033[0m' # No Color

# Configuration
MINIO_CONTAINER="workshop-minio"
TEST_NAMESPACE="test-backup"
BACKUP_NAME="test-namespace-backup"
MINIO_ENDPOINT="http://localhost:9000"
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

# Check prerequisites
check_prerequisites() {
    log_step "Checking prerequisites..."
    
    # Check if kubectl is available
    if ! command -v kubectl &> /dev/null; then
        log_error "kubectl is not installed or not in PATH"
        exit 1
    fi
    
    # Check if kind cluster is running
    if ! kubectl cluster-info &> /dev/null; then
        log_error "No Kubernetes cluster is accessible"
        exit 1
    fi
    
    # Check if Minio container is running
    if ! docker ps --format '{{.Names}}' | grep -q "^${MINIO_CONTAINER}$"; then
        log_error "Minio container '${MINIO_CONTAINER}' is not running"
        log_info "Run './scripts/start-minio-docker.sh' to start Minio"
        exit 1
    fi
    
    # Check if backup operator is deployed
    if ! kubectl get deployment cluster-backup-operator-controller-manager -n cluster-backup-operator-system &> /dev/null; then
        log_error "Backup operator is not deployed"
        log_info "Run 'make deploy' in the cluster-backup-operator directory"
        exit 1
    fi
    
    log_info "All prerequisites are met"
}

# Create test resources
create_test_resources() {
    log_step "Creating test resources..."
    
    # Create test namespace
    kubectl create namespace "$TEST_NAMESPACE" --dry-run=client -o yaml | kubectl apply -f -
    log_info "Created namespace: $TEST_NAMESPACE"
    
    # Create test deployment
    kubectl create deployment nginx --image=nginx -n "$TEST_NAMESPACE" --dry-run=client -o yaml | kubectl apply -f -
    log_info "Created deployment: nginx"
    
    # Create test configmap with backup label
    kubectl create configmap test-config --from-literal=key=value -n "$TEST_NAMESPACE" --dry-run=client -o yaml | \
    kubectl label --local -f - backup=enabled -o yaml | kubectl apply -f -
    log_info "Created configmap: test-config"
    
    # Create test secret with backup label
    kubectl create secret generic test-secret --from-literal=password=secret123 -n "$TEST_NAMESPACE" --dry-run=client -o yaml | \
    kubectl label --local -f - backup=enabled -o yaml | kubectl apply -f -
    log_info "Created secret: test-secret"
    
    # Create test service
    kubectl expose deployment nginx --port=80 -n "$TEST_NAMESPACE" --dry-run=client -o yaml | \
    kubectl label --local -f - backup=enabled -o yaml | kubectl apply -f -
    log_info "Created service: nginx"
    
    # Add backup labels to deployment
    kubectl label deployment nginx backup=enabled -n "$TEST_NAMESPACE"
    log_info "Added backup labels to resources"
    
    # Wait for deployment to be ready
    log_info "Waiting for deployment to be ready..."
    kubectl wait --for=condition=available --timeout=60s deployment/nginx -n "$TEST_NAMESPACE"
    
    log_info "Test resources created successfully"
}

# Get Minio container IP
get_minio_ip() {
    local minio_ip
    minio_ip=$(docker inspect "$MINIO_CONTAINER" | grep '"IPAddress"' | head -1 | cut -d '"' -f 4)
    if [ -z "$minio_ip" ]; then
        log_error "Could not determine Minio container IP"
        exit 1
    fi
    echo "$minio_ip"
}

# Note: Credentials are now embedded in backup resource for workshop simplicity

# Create backup resource
create_backup_resource() {
    log_step "Creating backup resource..."
    
    local minio_ip
    minio_ip=$(get_minio_ip)
    log_info "Using Minio IP: $minio_ip"
    
    cat <<EOF | kubectl apply -f -
apiVersion: backup.cnd.dk/v1
kind: Backup
metadata:
  name: $BACKUP_NAME
  namespace: default
spec:
  source:
    namespace: "$TEST_NAMESPACE"
    resourceTypes: ["deployments", "services", "configmaps", "secrets"]
    labelSelector:
      matchLabels:
        backup: "enabled"
  schedule: "*/1 * * * *"  # Every minute for testing
  retention: "1d"
  storageLocation:
    provider: "minio"
    bucket: "$BUCKET_NAME"
    endpoint: "http://${minio_ip}:9000"
    accessKey: "minioadmin"
    secretKey: "minioadmin123"
EOF
    
    log_info "Created backup resource: $BACKUP_NAME"
}

# Monitor backup execution
monitor_backup() {
    log_step "Monitoring backup execution..."
    
    local max_wait=300  # 5 minutes
    local wait_time=0
    local check_interval=10
    
    while [ $wait_time -lt $max_wait ]; do
        local phase
        phase=$(kubectl get backup "$BACKUP_NAME" -o jsonpath='{.status.phase}' 2>/dev/null || echo "")
        
        if [ -z "$phase" ]; then
            log_info "Backup resource not found or status not set yet..."
        else
            log_info "Backup phase: $phase"
            
            case "$phase" in
                "Completed")
                    log_info "Backup completed successfully!"
                    return 0
                    ;;
                "Failed")
                    log_error "Backup failed!"
                    kubectl describe backup "$BACKUP_NAME"
                    return 1
                    ;;
                "Running")
                    log_info "Backup is running..."
                    ;;
                *)
                    log_info "Backup phase: $phase"
                    ;;
            esac
        fi
        
        sleep $check_interval
        wait_time=$((wait_time + check_interval))
    done
    
    log_error "Backup did not complete within $max_wait seconds"
    return 1
}

# Verify backup in Minio
verify_backup_in_minio() {
    log_step "Verifying backup in Minio..."
    
    # Use Minio client to list objects
    local mc_cmd="docker run --rm --network kind minio/mc:latest"
    
    # Configure mc
    $mc_cmd alias set workshop "http://${MINIO_CONTAINER}:9000" "minioadmin" "minioadmin123" &> /dev/null
    
    # List objects in backup bucket
    log_info "Listing objects in bucket '$BUCKET_NAME':"
    if $mc_cmd ls "workshop/$BUCKET_NAME" --recursive; then
        log_info "Backup files found in Minio!"
    else
        log_error "No backup files found in Minio"
        return 1
    fi
    
    # Check for specific resource types
    local resource_types=("deployments" "configmaps" "secrets" "services")
    local backup_path="workshop/$BUCKET_NAME/backups/$TEST_NAMESPACE"
    
    for resource_type in "${resource_types[@]}"; do
        if $mc_cmd ls "$backup_path" --recursive | grep -q "$resource_type"; then
            log_info "✓ Found backup for $resource_type"
        else
            log_warn "⚠ No backup found for $resource_type"
        fi
    done
    
    return 0
}

# Download and verify backup content
verify_backup_content() {
    log_step "Verifying backup content..."
    
    local mc_cmd="docker run --rm --network kind minio/mc:latest"
    local temp_dir="/tmp/backup-validation-$$"
    
    # Create temporary directory
    mkdir -p "$temp_dir"
    
    # Download backup files
    log_info "Downloading backup files for verification..."
    $mc_cmd mirror "workshop/$BUCKET_NAME/backups/$TEST_NAMESPACE" "$temp_dir" &> /dev/null
    
    # Check if files were downloaded
    if [ ! -d "$temp_dir" ] || [ -z "$(ls -A "$temp_dir" 2>/dev/null)" ]; then
        log_error "No backup files were downloaded"
        rm -rf "$temp_dir"
        return 1
    fi
    
    # Verify JSON structure of downloaded files
    local json_files
    json_files=$(find "$temp_dir" -name "*.json" 2>/dev/null)
    
    if [ -z "$json_files" ]; then
        log_error "No JSON backup files found"
        rm -rf "$temp_dir"
        return 1
    fi
    
    local valid_files=0
    local total_files=0
    
    while IFS= read -r file; do
        total_files=$((total_files + 1))
        if jq . "$file" &> /dev/null; then
            valid_files=$((valid_files + 1))
            log_info "✓ Valid JSON: $(basename "$file")"
        else
            log_warn "⚠ Invalid JSON: $(basename "$file")"
        fi
    done <<< "$json_files"
    
    log_info "Verified $valid_files/$total_files JSON files"
    
    # Cleanup
    rm -rf "$temp_dir"
    
    if [ "$valid_files" -eq "$total_files" ] && [ "$total_files" -gt 0 ]; then
        log_info "All backup files are valid JSON"
        return 0
    else
        log_error "Some backup files are invalid"
        return 1
    fi
}

# Check controller logs
check_controller_logs() {
    log_step "Checking controller logs..."
    
    local controller_pod
    controller_pod=$(kubectl get pods -n cluster-backup-operator-system -l control-plane=controller-manager -o jsonpath='{.items[0].metadata.name}' 2>/dev/null)
    
    if [ -z "$controller_pod" ]; then
        log_error "Controller pod not found"
        return 1
    fi
    
    log_info "Controller pod: $controller_pod"
    log_info "Recent controller logs:"
    kubectl logs "$controller_pod" -n cluster-backup-operator-system --tail=20
    
    return 0
}

# Cleanup test resources
cleanup_test_resources() {
    log_step "Cleaning up test resources..."
    
    # Delete backup resource
    kubectl delete backup "$BACKUP_NAME" --ignore-not-found=true
    log_info "Deleted backup resource"
    
    # Delete test namespace (this will delete all resources in it)
    kubectl delete namespace "$TEST_NAMESPACE" --ignore-not-found=true
    log_info "Deleted test namespace"
    
    # Note: No secrets to clean up (using plain credentials)
    
    log_info "Cleanup completed"
}

# Show validation summary
show_summary() {
    log_step "Validation Summary"
    echo ""
    echo "🎉 Minio backup validation completed successfully!"
    echo ""
    echo "✅ Validated components:"
    echo "   - Minio container connectivity"
    echo "   - Backup operator deployment"
    echo "   - Test resource creation"
    echo "   - Backup resource processing"
    echo "   - Backup file storage in Minio"
    echo "   - JSON backup file integrity"
    echo ""
    echo "🔗 Access Minio Console:"
    echo "   URL: http://localhost:9001"
    echo "   Username: minioadmin"
    echo "   Password: minioadmin123"
    echo ""
    echo "📁 Backup files are stored in bucket: $BUCKET_NAME"
    echo ""
}

# Show usage
show_usage() {
    echo "Usage: $0 [OPTIONS]"
    echo ""
    echo "Validate Minio backup functionality for the workshop"
    echo ""
    echo "Options:"
    echo "  -h, --help              Show this help message"
    echo "  --no-cleanup            Don't cleanup test resources after validation"
    echo "  --namespace NAME        Use custom test namespace (default: $TEST_NAMESPACE)"
    echo "  --backup-name NAME      Use custom backup name (default: $BACKUP_NAME)"
    echo ""
    echo "Examples:"
    echo "  $0                      # Run full validation with cleanup"
    echo "  $0 --no-cleanup        # Run validation but keep test resources"
    echo ""
}

# Main execution
main() {
    local cleanup=true
    
    # Parse arguments
    while [[ $# -gt 0 ]]; do
        case $1 in
            -h|--help)
                show_usage
                exit 0
                ;;
            --no-cleanup)
                cleanup=false
                shift
                ;;
            --namespace)
                TEST_NAMESPACE="$2"
                shift 2
                ;;
            --backup-name)
                BACKUP_NAME="$2"
                shift 2
                ;;
            *)
                log_error "Unknown option: $1"
                show_usage
                exit 1
                ;;
        esac
    done
    
    log_info "Starting Minio backup validation..."
    
    # Run validation steps
    check_prerequisites
    create_test_resources
    create_backup_resource
    
    if monitor_backup; then
        verify_backup_in_minio
        verify_backup_content
        check_controller_logs
        show_summary
        
        if [ "$cleanup" = true ]; then
            cleanup_test_resources
        else
            log_info "Skipping cleanup (--no-cleanup specified)"
        fi
    else
        log_error "Backup validation failed"
        check_controller_logs
        
        if [ "$cleanup" = true ]; then
            cleanup_test_resources
        fi
        
        exit 1
    fi
}

# Run main function if script is executed directly
if [[ "${BASH_SOURCE[0]}" == "${0}" ]]; then
    main "$@"
fi