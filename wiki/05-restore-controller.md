# Stage 5: Restore Controller Implementation

## Overview

In this stage, we'll implement a restore controller that can retrieve backed-up Kubernetes resources from Minio and apply them to a different cluster. This enables cross-cluster disaster recovery and migration scenarios.

The restore controller will complement our backup controller by providing the ability to:
- Restore resources from Minio backups to any Kubernetes cluster
- Handle cross-cluster restoration scenarios with namespace mapping
- Validate and transform resources during restoration
- Provide detailed status and error reporting
- Support dry-run and validation-only modes for safe testing
- Handle resource conflicts with configurable resolution strategies

## Learning Objectives

By the end of this stage, you will:
- Understand how to use operator-sdk to create new APIs and controllers
- Learn how to implement a restore controller for Kubernetes operators
- Master retrieving and parsing backed-up resources from object storage
- Implement cross-cluster resource restoration logic with namespace mapping
- Handle resource conflicts and validation during restoration
- Create comprehensive error handling for restore operations
- Understand phase-based reconciliation patterns in Kubernetes controllers

## Restore Controller Architecture

### Custom Resource Definition

The Restore CRD defines the desired state for a restore operation:

```yaml
apiVersion: backup.cnd.dk/v1
kind: Restore
metadata:
  name: my-restore
  namespace: default
spec:
  source:
    storageLocation:
      provider: "minio"
      bucket: "cluster-backups"
      endpoint: "http://host.docker.internal:9000"
      accessKey: "minioadmin"
      secretKey: "minioadmin123"
    backupPath: "backups/cluster-backup/2025-01-21T10-30-00"
  target:
    namespaces: ["production"]  # Target namespaces for restoration
    resourceTypes: ["deployments", "services", "configmaps"]
    conflictResolution: "skip"  # skip, overwrite, or fail
  options:
    dryRun: false
    validateOnly: false
```

### Restore Phases

The restore controller manages several phases:

1. **New**: Restore resource created
2. **Validating**: Validating backup source and target cluster
3. **Downloading**: Retrieving backup data from Minio
4. **Restoring**: Applying resources to target cluster
5. **Completed**: Restore operation finished successfully
6. **Failed**: Restore operation failed

## Implementation Steps

### Step 1: Generate Restore API and Controller with Operator SDK

First, we'll use operator-sdk to generate the Restore API and controller scaffolding. This ensures we follow Kubernetes best practices and get proper code generation.

1. **Navigate to the operator directory**:
```bash
cd cluster-backup-operator
```

2. **Generate the Restore API and Controller**:
```bash
operator-sdk create api --group backup --version v1 --kind Restore --resource --controller
```

This command will:
- Create `api/v1/restore_types.go` with the Restore CRD definition
- Generate `internal/controller/restore_controller.go` with the controller scaffolding
- Update the main.go file to register the new controller
- Create sample YAML files and RBAC configurations
- Generate the CRD manifests

3. **If files already exist, use the force flag**:
```bash
operator-sdk create api --group backup --version v1 --kind Restore --resource --controller --force
```

### Step 2: Define Restore Types

Now let's examine and implement the restore types that define our Custom Resource:

Replace the generated scaffolding in `api/v1/restore_types.go` with our comprehensive types:

```go
// RestoreSpec defines the desired state of Restore.
type RestoreSpec struct {
    // Source defines where to restore from
    Source RestoreSource `json:"source"`

    // Target defines what and where to restore
    Target RestoreTarget `json:"target"`

    // Options for restore behavior
    Options RestoreOptions `json:"options,omitempty"`
}

// RestoreSource defines the backup source location
type RestoreSource struct {
    // StorageLocation where the backup is stored
    StorageLocation StorageLocation `json:"storageLocation"`

    // BackupPath is the specific backup to restore from
    BackupPath string `json:"backupPath"`
}

// RestoreTarget defines restoration targets and behavior
type RestoreTarget struct {
    // Namespaces to restore to. If empty, restores to original namespaces
    Namespaces []string `json:"namespaces,omitempty"`

    // ResourceTypes to restore. If empty, restores all resource types from backup
    ResourceTypes []string `json:"resourceTypes,omitempty"`

    // ConflictResolution strategy when resources already exist: skip, overwrite, fail
    ConflictResolution string `json:"conflictResolution,omitempty"`

    // LabelSelector for filtering resources to restore
    LabelSelector *metav1.LabelSelector `json:"labelSelector,omitempty"`

    // NamespaceMapping maps source namespaces to target namespaces
    // Format: {"source-ns": "target-ns"}
    NamespaceMapping map[string]string `json:"namespaceMapping,omitempty"`
}

// RestoreOptions defines additional restore options
type RestoreOptions struct {
    // DryRun performs validation without actually restoring resources
    DryRun bool `json:"dryRun,omitempty"`

    // ValidateOnly validates the backup without restoring
    ValidateOnly bool `json:"validateOnly,omitempty"`

    // CreateNamespaces automatically creates target namespaces if they don't exist
    CreateNamespaces bool `json:"createNamespaces,omitempty"`

    // SkipClusterResources skips restoration of cluster-scoped resources
    SkipClusterResources bool `json:"skipClusterResources,omitempty"`
}

// RestoreStatus defines the observed state of Restore.
type RestoreStatus struct {
    // Phase represents the current phase of the restore
    Phase RestorePhase `json:"phase,omitempty"`

    // Message provides additional information about the current state
    Message string `json:"message,omitempty"`

    // StartTime is when the restore operation started
    StartTime *metav1.Time `json:"startTime,omitempty"`

    // CompletionTime is when the restore operation completed
    CompletionTime *metav1.Time `json:"completionTime,omitempty"`

    // ResourceCounts tracks how many resources of each type were restored
    ResourceCounts map[string]int32 `json:"resourceCounts,omitempty"`

    // RestoredResources lists the resources that were successfully restored
    RestoredResources []RestoredResource `json:"restoredResources,omitempty"`

    // FailedResources lists the resources that failed to restore
    FailedResources []FailedResource `json:"failedResources,omitempty"`

    // SkippedResources lists the resources that were skipped due to conflicts
    SkippedResources []SkippedResource `json:"skippedResources,omitempty"`

    // BackupInfo contains information about the source backup
    BackupInfo *BackupInfo `json:"backupInfo,omitempty"`
}

// RestorePhase represents the current phase of a restore operation
type RestorePhase string

const (
    RestorePhaseNew         RestorePhase = "New"
    RestorePhaseValidating  RestorePhase = "Validating"
    RestorePhaseDownloading RestorePhase = "Downloading"
    RestorePhaseRestoring   RestorePhase = "Restoring"
    RestorePhaseCompleted   RestorePhase = "Completed"
    RestorePhaseFailed      RestorePhase = "Failed"
)
```

Add kubebuilder annotations for better kubectl output:

```go
//+kubebuilder:object:root=true
//+kubebuilder:subresource:status
//+kubebuilder:printcolumn:name="Phase",type=string,JSONPath=`.status.phase`
//+kubebuilder:printcolumn:name="Backup Path",type=string,JSONPath=`.spec.source.backupPath`
//+kubebuilder:printcolumn:name="Restored",type=integer,JSONPath=`.status.resourceCounts.total`
//+kubebuilder:printcolumn:name="Age",type=date,JSONPath=`.metadata.creationTimestamp`

// Restore is the Schema for the restores API
type Restore struct {
    metav1.TypeMeta   `json:",inline"`
    metav1.ObjectMeta `json:"metadata,omitempty"`

    Spec   RestoreSpec   `json:"spec,omitempty"`
    Status RestoreStatus `json:"status,omitempty"`
}
```

### Step 3: Implement Restore Controller Logic

The restore controller implements a phase-based reconciliation pattern. Replace the generated controller code in `internal/controller/restore_controller.go`:

#### 3.1 Add Required Imports

```go
import (
    "bytes"
    "context"
    "encoding/json"
    "fmt"
    "strings"
    "time"

    "github.com/minio/minio-go/v7"
    "github.com/minio/minio-go/v7/pkg/credentials"
    backupv1 "github.com/rfashwall/cnd-workshop/api/v1"
    corev1 "k8s.io/api/core/v1"
    "k8s.io/apimachinery/pkg/api/errors"
    metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
    "k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
    "k8s.io/apimachinery/pkg/runtime"
    ctrl "sigs.k8s.io/controller-runtime"
    "sigs.k8s.io/controller-runtime/pkg/client"
    logf "sigs.k8s.io/controller-runtime/pkg/log"
)
```

#### 3.2 Add RBAC Permissions

Update the RBAC annotations to include permissions for all resources we might restore:

```go
//+kubebuilder:rbac:groups=backup.cnd.dk,resources=restores,verbs=get;list;watch;create;update;patch;delete
//+kubebuilder:rbac:groups=backup.cnd.dk,resources=restores/status,verbs=get;update;patch
//+kubebuilder:rbac:groups=backup.cnd.dk,resources=restores/finalizers,verbs=update
//+kubebuilder:rbac:groups=apps,resources=deployments,verbs=get;list;watch;create;update;patch;delete
//+kubebuilder:rbac:groups="",resources=configmaps,verbs=get;list;watch;create;update;patch;delete
//+kubebuilder:rbac:groups="",resources=secrets,verbs=get;list;watch;create;update;patch;delete
//+kubebuilder:rbac:groups="",resources=services,verbs=get;list;watch;create;update;patch;delete
//+kubebuilder:rbac:groups="",resources=persistentvolumeclaims,verbs=get;list;watch;create;update;patch;delete
//+kubebuilder:rbac:groups="",resources=namespaces,verbs=get;list;watch;create;update;patch;delete
```

#### 3.3 Implement Phase-Based Reconciliation

```go
func (r *RestoreReconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
    log := logf.FromContext(ctx)

    // Fetch the Restore instance
    restore := &backupv1.Restore{}
    err := r.Get(ctx, req.NamespacedName, restore)
    if err != nil {
        if client.IgnoreNotFound(err) == nil {
            log.Info("Restore resource not found. Ignoring since object must be deleted")
            return ctrl.Result{}, nil
        }
        log.Error(err, "Failed to get Restore")
        return ctrl.Result{}, err
    }

    // Initialize status if not set
    if restore.Status.Phase == "" {
        restore.Status.Phase = backupv1.RestorePhaseNew
        restore.Status.Message = "Restore resource created"
        restore.Status.StartTime = &metav1.Time{Time: time.Now()}
        if err := r.Status().Update(ctx, restore); err != nil {
            log.Error(err, "Failed to update Restore status")
            return ctrl.Result{}, err
        }
        return ctrl.Result{Requeue: true}, nil
    }

    // Handle different phases
    switch restore.Status.Phase {
    case backupv1.RestorePhaseNew:
        return r.handleValidatingPhase(ctx, restore)
    case backupv1.RestorePhaseValidating:
        return r.handleDownloadingPhase(ctx, restore)
    case backupv1.RestorePhaseDownloading:
        return r.handleRestoringPhase(ctx, restore)
    case backupv1.RestorePhaseCompleted, backupv1.RestorePhaseFailed:
        // Nothing to do for completed or failed restores
        return ctrl.Result{}, nil
    }

    return ctrl.Result{}, nil
}
```

#### 3.4 Key Implementation Functions

The controller implements several key functions:

- **`handleValidatingPhase()`**: Validates restore configuration and backup source accessibility
- **`handleDownloadingPhase()`**: Analyzes backup contents and prepares for restoration
- **`handleRestoringPhase()`**: Performs the actual resource restoration
- **`validateRestoreConfig()`**: Validates the restore specification
- **`validateBackupSource()`**: Verifies backup exists and is accessible in Minio
- **`analyzeBackup()`**: Scans backup to understand what resources are available
- **`performRestore()`**: Downloads and applies resources to the target cluster
- **`restoreResource()`**: Handles restoration of individual resources with conflict resolution
- **`cleanResourceForRestore()`**: Removes cluster-specific fields from resources
- **`mapNamespace()`**: Maps source namespaces to target namespaces
- **`ensureNamespaceExists()`**: Creates target namespaces if needed

### Step 4: Generate Manifests and Build

After implementing the types and controller, generate the CRDs and other manifests:

```bash
# Generate CRDs, RBAC, and other manifests
make manifests

# Generate code (deepcopy methods, etc.)
make generate

# Build and test the code
go build ./...
go test ./...
```

### Step 5: Handle Cross-Cluster Scenarios

The restore controller must handle several cross-cluster challenges:

#### 5.1 Namespace Mapping
Resources may need to be restored to different namespaces:
```go
func (r *RestoreReconciler) mapNamespace(sourceNamespace string, target backupv1.RestoreTarget) string {
    // Check namespace mapping first
    if target.NamespaceMapping != nil {
        if targetNS, exists := target.NamespaceMapping[sourceNamespace]; exists {
            return targetNS
        }
    }

    // If only one target namespace specified, map all to it
    if len(target.Namespaces) == 1 {
        return target.Namespaces[0]
    }

    // Otherwise, keep original namespace
    return sourceNamespace
}
```

#### 5.2 Resource Conflicts
Handle existing resources in the target cluster:
```go
func (r *RestoreReconciler) restoreResource(ctx context.Context, minioClient *minio.Client, bucket, objectKey, targetNamespace, conflictResolution string) (*RestoreResult, error) {
    // Download and parse resource
    // ...

    // Check if resource already exists
    existing := &unstructured.Unstructured{}
    existing.SetGroupVersionKind(resource.GetObjectKind().GroupVersionKind())
    
    getErr := r.Get(ctx, client.ObjectKey{Name: resource.GetName(), Namespace: resource.GetNamespace()}, existing)

    if getErr == nil {
        // Resource exists, handle conflict resolution
        switch conflictResolution {
        case "skip", "":
            return &RestoreResult{Action: "skipped", Reason: "resource already exists"}, nil
        case "fail":
            return nil, fmt.Errorf("resource %s/%s already exists", resource.GetKind(), resource.GetName())
        case "overwrite":
            // Update the existing resource
            resource.SetResourceVersion(existing.GetResourceVersion())
            if err := r.Update(ctx, &resource); err != nil {
                return nil, fmt.Errorf("failed to update resource: %w", err)
            }
            return &RestoreResult{Action: "updated"}, nil
        }
    } else if errors.IsNotFound(getErr) {
        // Resource doesn't exist, create it
        if err := r.Create(ctx, &resource); err != nil {
            return nil, fmt.Errorf("failed to create resource: %w", err)
        }
        return &RestoreResult{Action: "created"}, nil
    }

    return nil, fmt.Errorf("failed to check if resource exists: %w", getErr)
}
```

#### 5.3 Resource Cleanup
Ensure resources are valid for the target cluster:
```go
func (r *RestoreReconciler) cleanResourceForRestore(resource *unstructured.Unstructured, targetNamespace string) {
    // Remove metadata fields that shouldn't be restored
    unstructured.RemoveNestedField(resource.Object, "metadata", "resourceVersion")
    unstructured.RemoveNestedField(resource.Object, "metadata", "uid")
    unstructured.RemoveNestedField(resource.Object, "metadata", "generation")
    unstructured.RemoveNestedField(resource.Object, "metadata", "creationTimestamp")
    unstructured.RemoveNestedField(resource.Object, "metadata", "managedFields")

    // Remove status field
    unstructured.RemoveNestedField(resource.Object, "status")

    // Update namespace if specified
    if targetNamespace != "" && resource.GetNamespace() != "" {
        resource.SetNamespace(targetNamespace)
    }

    // Remove cluster-specific fields for certain resource types
    switch resource.GetKind() {
    case "Service":
        unstructured.RemoveNestedField(resource.Object, "spec", "clusterIP")
        unstructured.RemoveNestedField(resource.Object, "spec", "clusterIPs")
    case "PersistentVolumeClaim":
        unstructured.RemoveNestedField(resource.Object, "spec", "volumeName")
    }
}
```

### Step 6: Deploy and Test

1. **Install the CRDs**:
```bash
make install
```

2. **Run the controller locally** (for development):
```bash
make run
```

3. **Or deploy to the cluster**:
```bash
make deploy
```

4. **Verify the restore CRD is installed**:
```bash
kubectl get crd restores.backup.cnd.dk
kubectl describe crd restores.backup.cnd.dk
```

## Hands-on Exercise

### Exercise 1: Create a Basic Restore Resource

1. **First, check what backups are available in Minio**:
```bash
# Access Minio console at http://localhost:9001
# Or use mc CLI to list backups
mc ls minio/cluster-backups/backups/cluster-backup/
```

2. **Create a restore configuration** (`restore-example.yaml`):
```yaml
apiVersion: backup.cnd.dk/v1
kind: Restore
metadata:
  name: test-restore
  namespace: default
spec:
  source:
    storageLocation:
      provider: "minio"
      bucket: "cluster-backups"
      endpoint: "http://host.docker.internal:9000"
      accessKey: "minioadmin"
      secretKey: "minioadmin123"
    backupPath: "backups/cluster-backup/2025-01-21T10-30-00"  # Use your actual backup path
  target:
    namespaces: ["restored-namespace"]
    resourceTypes: ["deployments", "services", "configmaps"]
    conflictResolution: "skip"
  options:
    dryRun: true  # Start with dry run for safety
    createNamespaces: true
```

3. **Apply the restore resource**:
```bash
kubectl apply -f restore-example.yaml
```

4. **Monitor the restore progress**:
```bash
# Watch the restore status
kubectl get restore test-restore -w

# Get detailed status
kubectl get restore test-restore -o yaml

# Check controller logs
kubectl logs -n cluster-backup-operator-system deployment/cluster-backup-operator-controller-manager

# Describe the restore for events
kubectl describe restore test-restore
```

5. **Check the results**:
```bash
# List restored resources
kubectl get all -n restored-namespace

# Verify the restore status
kubectl get restore test-restore -o jsonpath='{.status.phase}'
kubectl get restore test-restore -o jsonpath='{.status.resourceCounts}'
```

### Exercise 2: Cross-Cluster Restore

This exercise demonstrates restoring backups from one cluster to a completely different cluster.

1. **Create a second kind cluster**:
```bash
kind create cluster --name restore-target
```

2. **Switch to the new cluster context**:
```bash
kubectl config use-context kind-restore-target
```

3. **Deploy the restore controller to the new cluster**:
```bash
# Install CRDs
make install

# Deploy the controller
make deploy

# Verify deployment
kubectl get pods -n cluster-backup-operator-system
```

4. **Create a cross-cluster restore configuration** (`cross-cluster-restore.yaml`):
```yaml
apiVersion: backup.cnd.dk/v1
kind: Restore
metadata:
  name: cross-cluster-restore
  namespace: default
spec:
  source:
    storageLocation:
      provider: "minio"
      bucket: "cluster-backups"
      endpoint: "http://host.docker.internal:9000"
      accessKey: "minioadmin"
      secretKey: "minioadmin123"
    backupPath: "backups/cluster-backup/2025-01-21T10-30-00"  # From source cluster
  target:
    namespaces: ["production", "staging"]
    resourceTypes: ["deployments", "services", "configmaps", "secrets"]
    conflictResolution: "overwrite"
    namespaceMapping:
      "default": "production"
      "test": "staging"
  options:
    dryRun: false
    createNamespaces: true
    skipClusterResources: true
```

5. **Apply and monitor the cross-cluster restore**:
```bash
kubectl apply -f cross-cluster-restore.yaml

# Monitor progress
kubectl get restore cross-cluster-restore -w

# Check restored resources
kubectl get all -n production
kubectl get all -n staging
```

### Exercise 3: Advanced Restore Scenarios

1. **Selective restore with label selector**:
```yaml
apiVersion: backup.cnd.dk/v1
kind: Restore
metadata:
  name: selective-restore
spec:
  source:
    storageLocation:
      provider: "minio"
      bucket: "cluster-backups"
      endpoint: "http://host.docker.internal:9000"
    backupPath: "backups/cluster-backup/2025-01-21T10-30-00"
  target:
    namespaces: ["filtered-apps"]
    resourceTypes: ["deployments", "services"]
    conflictResolution: "fail"
    labelSelector:
      matchLabels:
        app: "web-server"
        tier: "frontend"
  options:
    createNamespaces: true
```

2. **Validation-only restore**:
```yaml
apiVersion: backup.cnd.dk/v1
kind: Restore
metadata:
  name: validation-restore
spec:
  source:
    storageLocation:
      provider: "minio"
      bucket: "cluster-backups"
      endpoint: "http://host.docker.internal:9000"
    backupPath: "backups/cluster-backup/2025-01-21T10-30-00"
  target:
    conflictResolution: "skip"
  options:
    validateOnly: true  # Only validate, don't restore
```

## Validation Steps

After implementing the restore controller, verify the following:

### 1. CRD Installation
```bash
# Check if the restore CRD is installed
kubectl get crd restores.backup.cnd.dk

# Verify CRD structure
kubectl describe crd restores.backup.cnd.dk

# Check custom columns are working
kubectl get restore
```

### 2. Controller Deployment
```bash
# Verify controller is running
kubectl get pods -n cluster-backup-operator-system

# Check controller logs
kubectl logs -n cluster-backup-operator-system deployment/cluster-backup-operator-controller-manager

# Verify RBAC permissions
kubectl auth can-i create restores --as=system:serviceaccount:cluster-backup-operator-system:cluster-backup-operator-controller-manager
```

### 3. Backup Source Validation
```bash
# Test with invalid backup path
kubectl apply -f - <<EOF
apiVersion: backup.cnd.dk/v1
kind: Restore
metadata:
  name: invalid-backup-test
spec:
  source:
    storageLocation:
      provider: "minio"
      bucket: "cluster-backups"
      endpoint: "http://host.docker.internal:9000"
    backupPath: "non-existent-path"
  target:
    namespaces: ["test"]
  options:
    validateOnly: true
EOF

# Should show validation failure
kubectl get restore invalid-backup-test -o jsonpath='{.status.message}'
```

### 4. Resource Restoration Testing
```bash
# Create test resources to backup first
kubectl create namespace test-source
kubectl create deployment nginx --image=nginx -n test-source
kubectl create service clusterip nginx --tcp=80:80 -n test-source

# Create backup (assuming backup controller is running)
# Then test restore to different namespace

# Verify resources were restored correctly
kubectl get all -n test-target
kubectl describe deployment nginx -n test-target
```

### 5. Status Reporting Verification
```bash
# Check detailed status
kubectl get restore test-restore -o yaml | grep -A 20 status:

# Verify resource counts
kubectl get restore test-restore -o jsonpath='{.status.resourceCounts}'

# Check restored resources list
kubectl get restore test-restore -o jsonpath='{.status.restoredResources}'

# Check for any failed resources
kubectl get restore test-restore -o jsonpath='{.status.failedResources}'
```

### 6. Error Handling Testing
```bash
# Test conflict resolution
kubectl create namespace conflict-test
kubectl create deployment nginx --image=nginx -n conflict-test

# Try to restore to same namespace with different conflict strategies
# Should handle conflicts according to specified strategy
```

## Common Issues and Troubleshooting

### Issue: "Backup path not found"
**Cause**: The specified backup path doesn't exist in Minio  
**Solution**: 
```bash
# Verify backup path exists
mc ls minio/cluster-backups/backups/cluster-backup/

# Check Minio connectivity
kubectl run minio-test --image=minio/mc --rm -it -- mc ls minio/cluster-backups/
```

### Issue: "Resource already exists"
**Cause**: Target cluster already has resources with the same names  
**Solution**: 
```bash
# Use appropriate conflict resolution strategy
spec:
  target:
    conflictResolution: "overwrite"  # or "skip" or "fail"

# Or clean up existing resources
kubectl delete deployment nginx -n target-namespace
```

### Issue: "Namespace not found"
**Cause**: Target namespace doesn't exist in the target cluster  
**Solution**: 
```bash
# Enable automatic namespace creation
spec:
  options:
    createNamespaces: true

# Or create namespaces manually
kubectl create namespace target-namespace
```

### Issue: "Invalid resource format"
**Cause**: Backup contains corrupted or invalid resource data  
**Solution**: 
```bash
# Validate backup integrity
mc cat minio/cluster-backups/backups/cluster-backup/2025-01-21T10-30-00/namespaces/default/deployments/nginx.json

# Check JSON format
kubectl apply --dry-run=client -f downloaded-resource.json
```

### Issue: "Controller not reconciling"
**Cause**: Controller may not be watching the restore resources  
**Solution**: 
```bash
# Check controller logs
kubectl logs -n cluster-backup-operator-system deployment/cluster-backup-operator-controller-manager -f

# Verify CRD registration
kubectl get crd restores.backup.cnd.dk -o yaml | grep -A 5 "served: true"

# Restart controller
kubectl rollout restart deployment/cluster-backup-operator-controller-manager -n cluster-backup-operator-system
```

### Issue: "Permission denied errors"
**Cause**: Insufficient RBAC permissions for the controller  
**Solution**: 
```bash
# Check RBAC permissions
kubectl describe clusterrole cluster-backup-operator-manager-role

# Verify service account
kubectl get serviceaccount -n cluster-backup-operator-system

# Re-apply RBAC
make deploy
```

### Issue: "Minio connection timeout"
**Cause**: Network connectivity issues to Minio  
**Solution**: 
```bash
# Test Minio connectivity from cluster
kubectl run minio-test --image=minio/mc --rm -it -- mc config host add minio http://host.docker.internal:9000 minioadmin minioadmin123

# Check if Minio is accessible
curl -I http://host.docker.internal:9000/minio/health/live

# Verify endpoint configuration
spec:
  source:
    storageLocation:
      endpoint: "http://host.docker.internal:9000"  # Correct for kind clusters
```

## Next Steps

In the next stage, we'll implement comprehensive testing strategies for both backup and restore controllers, including unit tests, integration tests, and end-to-end validation scenarios.

## Implementation Summary

The restore controller implementation includes:

1. **Operator SDK Usage**: Proper use of `operator-sdk create api` to generate scaffolding
2. **Comprehensive Types**: Detailed RestoreSpec, RestoreStatus, and supporting types
3. **Phase-Based Reconciliation**: Structured approach with validation, downloading, and restoring phases
4. **Cross-Cluster Support**: Namespace mapping and resource transformation for different clusters
5. **Conflict Resolution**: Configurable strategies for handling existing resources
6. **Error Handling**: Comprehensive validation and detailed error reporting
7. **Status Tracking**: Detailed progress reporting with resource counts and operation results

## Key Takeaways

- **Operator SDK**: Use `operator-sdk create api` to generate proper scaffolding for new APIs and controllers
- **Phase-Based Design**: Implement complex operations using phase-based reconciliation for better control and debugging
- **Cross-Cluster Restoration**: Requires careful handling of namespaces, resource conflicts, and cluster-specific fields
- **Validation First**: Always validate configuration and backup source before attempting restoration
- **Conflict Resolution**: Provide configurable strategies (skip, overwrite, fail) for handling existing resources
- **Resource Cleanup**: Remove cluster-specific metadata and fields when restoring to different clusters
- **Dry-Run Support**: Essential for testing restore operations safely before actual execution
- **Comprehensive Status**: Detailed status reporting helps users understand operation progress and troubleshoot issues
- **Error Handling**: Proper error handling with meaningful messages improves user experience
- **RBAC Permissions**: Ensure controller has appropriate permissions for all resources it might restore

## Code Organization

```
cluster-backup-operator/
├── api/v1/
│   ├── backup_types.go          # Backup CRD types
│   └── restore_types.go         # Restore CRD types (new)
├── internal/controller/
│   ├── backup_controller.go     # Backup controller
│   └── restore_controller.go    # Restore controller (new)
├── config/
│   ├── crd/bases/
│   │   ├── backup.cnd.dk_backups.yaml
│   │   └── backup.cnd.dk_restores.yaml  # Generated restore CRD
│   └── samples/
│       ├── backup_v1_backup.yaml
│       └── backup_v1_restore.yaml       # Restore examples
└── cmd/
    └── main.go                  # Updated to include restore controller
```