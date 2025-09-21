# Stage 4: Backup Controller Implementation

## Overview

In this stage, we'll implement a comprehensive backup controller that can backup entire Kubernetes clusters or specific namespaces to Minio object storage. The implementation supports flexible backup sources, multiple namespaces, cluster-scoped resources, and intelligent resource filtering.

### Key Capabilities
- **Multi-Namespace Backup**: Backup specific namespaces, multiple namespaces, or entire clusters
- **Cluster Resource Support**: Include cluster-scoped resources like ClusterRoles, PersistentVolumes
- **Flexible Resource Selection**: Choose specific resource types and use label selectors
- **Intelligent Filtering**: Automatically exclude system resources and namespaces
- **Organized Storage**: Hierarchical backup structure for easy navigation and restore

## Learning Objectives

- Set up Minio as a Docker container accessible to kind clusters
- Configure network connectivity between kind clusters and Docker containers
- Implement Minio client integration in the backup controller
- Store Kubernetes resources as JSON objects in Minio buckets
- Test cross-cluster backup functionality

## Prerequisites

- Completed Stage 3 (Operator SDK and CRD Creation)
- Docker installed and running
- kind cluster running from previous stages

## Minio Docker Setup

### 1. Start Minio Container

First, let's start a Minio container that will be accessible from our kind clusters:

```bash
# Run the setup script to start Minio
./scripts/start-minio-docker.sh
```

This script creates a Minio container with:
- **Access Key**: `minioadmin`
- **Secret Key**: `minioadmin`
- **Console Port**: `9001` (Web UI)
- **API Port**: `9000` (S3-compatible API)
- **Network**: Connected to Docker's default bridge network

### 2. Verify Minio is Running

Check that Minio is accessible:

```bash
# Check container status
docker ps | grep minio

# Test API connectivity
curl -I http://localhost:9000/minio/health/live
```

### 3. Access Minio Console

Open your browser to `http://localhost:9001` and login with:
- **Username**: `minioadmin`
- **Password**: `minioadmin`

Create a bucket called `k8s-backups` for storing our backup data.

## Network Configuration

### Understanding Docker-Kind Networking

When running kind clusters, they create their own Docker networks. To allow communication between kind clusters and standalone Docker containers, we need to understand the networking setup:

```bash
# List Docker networks
docker network ls

# Inspect kind network
docker network inspect kind

# Get Minio container IP
docker inspect minio-server | grep IPAddress
```

### Configure Kind Cluster Access

The kind clusters need to access Minio using the container's IP address rather than localhost. We'll configure this in our backup resources.

## Backup Controller Implementation

### 1. Add Minio Client Dependency

First, let's add the required dependencies to our project:

```bash
cd cluster-backup-operator
# Add Minio Go client for object storage
go get github.com/minio/minio-go/v7

# Add cron library for proper schedule parsing
go get github.com/robfig/cron/v3
```

### 2. Enhance the API Types

Before implementing the controller logic, we need to enhance our API types to support the new backup capabilities.

#### Update BackupSource Structure

Edit `api/v1/backup_types.go` to add support for multiple namespaces and cluster resources:

```go
// BackupSource defines the source of the backup
type BackupSource struct {
	// Namespaces to backup. If empty, backs up all namespaces
	// Use ["*"] to explicitly backup all namespaces
	// Use ["namespace1", "namespace2"] to backup specific namespaces
	Namespaces []string `json:"namespaces,omitempty"`

	// Namespace to backup (deprecated, use Namespaces instead)
	// +optional
	Namespace string `json:"namespace,omitempty"`

	// LabelSelector for resources to backup
	LabelSelector *metav1.LabelSelector `json:"labelSelector,omitempty"`

	// ResourceTypes specifies which resource types to backup
	// If empty, defaults to common resource types (deployments, services, configmaps, secrets)
	ResourceTypes []string `json:"resourceTypes,omitempty"`

	// IncludeClusterResources indicates whether to include cluster-scoped resources
	// like ClusterRoles, ClusterRoleBindings, PersistentVolumes, etc.
	IncludeClusterResources bool `json:"includeClusterResources,omitempty"`

	// ExcludeNamespaces specifies namespaces to exclude from backup
	// Useful when backing up all namespaces but want to skip system namespaces
	ExcludeNamespaces []string `json:"excludeNamespaces,omitempty"`
}
```

#### Update StorageLocation for Plain Credentials

```go
// StorageLocation defines backup storage configuration
type StorageLocation struct {
	// Provider (e.g., "minio", "s3", "gcs")
	Provider string `json:"provider"`

	// Bucket name
	Bucket string `json:"bucket"`

	// Endpoint URL (for Minio)
	Endpoint string `json:"endpoint,omitempty"`

	// AccessKey for Minio authentication (for workshop simplicity)
	AccessKey string `json:"accessKey,omitempty"`

	// SecretKey for Minio authentication (for workshop simplicity)
	SecretKey string `json:"secretKey,omitempty"`
}
```

#### Enhance BackupStatus

```go
// BackupStatus defines the observed state of Backup.
type BackupStatus struct {
	// Phase represents the current phase of the backup
	Phase BackupPhase `json:"phase,omitempty"`

	// Message provides additional information about the current state
	Message string `json:"message,omitempty"`

	// LastBackupTime is the timestamp of the last successful backup
	LastBackupTime *metav1.Time `json:"lastBackupTime,omitempty"`

	// NextBackupTime is the timestamp of the next scheduled backup
	NextBackupTime *metav1.Time `json:"nextBackupTime,omitempty"`

	// BackupCount is the total number of backups performed
	BackupCount int32 `json:"backupCount,omitempty"`

	// ResourceCounts tracks how many resources of each type were backed up
	ResourceCounts map[string]int32 `json:"resourceCounts,omitempty"`

	// BackupPath is the path in storage where the backup is stored
	BackupPath string `json:"backupPath,omitempty"`
}
```

### 3. Implement the Backup Controller Logic

Now let's implement the comprehensive backup controller. The key changes are in `internal/controller/backup_controller.go`.

#### Add Required Imports

```go
import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"github.com/robfig/cron/v3"
	"github.com/minio/minio-go/v7"
	"github.com/minio/minio-go/v7/pkg/credentials"
	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	rbacv1 "k8s.io/api/rbac/v1"
	storagev1 "k8s.io/api/storage/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	logf "sigs.k8s.io/controller-runtime/pkg/log"

	backupv1 "github.com/rfashwall/cnd-workshop/api/v1"
)
```

#### Update RBAC Permissions

Add comprehensive RBAC permissions for all resource types:

```go
// +kubebuilder:rbac:groups=backup.cnd.dk,resources=backups,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=backup.cnd.dk,resources=backups/status,verbs=get;update;patch
// +kubebuilder:rbac:groups=backup.cnd.dk,resources=backups/finalizers,verbs=update
// +kubebuilder:rbac:groups=apps,resources=deployments,verbs=get;list;watch
// +kubebuilder:rbac:groups="",resources=configmaps,verbs=get;list;watch
// +kubebuilder:rbac:groups="",resources=secrets,verbs=get;list;watch
// +kubebuilder:rbac:groups="",resources=services,verbs=get;list;watch
// +kubebuilder:rbac:groups="",resources=persistentvolumeclaims,verbs=get;list;watch
// +kubebuilder:rbac:groups="",resources=namespaces,verbs=get;list;watch
// +kubebuilder:rbac:groups="",resources=persistentvolumes,verbs=get;list;watch
// +kubebuilder:rbac:groups=networking.k8s.io,resources=ingresses,verbs=get;list;watch
// +kubebuilder:rbac:groups=rbac.authorization.k8s.io,resources=clusterroles,verbs=get;list;watch
// +kubebuilder:rbac:groups=rbac.authorization.k8s.io,resources=clusterrolebindings,verbs=get;list;watch
// +kubebuilder:rbac:groups=storage.k8s.io,resources=storageclasses,verbs=get;list;watch
```

#### Core Backup Logic Implementation

The main backup logic in the `performBackup` method:

```go
func (r *BackupReconciler) performBackup(ctx context.Context, backup *backupv1.Backup) error {
	log := logf.FromContext(ctx)

	// Update status to running
	backup.Status.Phase = backupv1.BackupPhaseRunning
	backup.Status.Message = "Backup in progress"
	if err := r.Status().Update(ctx, backup); err != nil {
		return fmt.Errorf("failed to update status to running: %w", err)
	}

	// Initialize Minio client
	minioClient, err := r.initMinioClient(ctx, backup)
	if err != nil {
		return fmt.Errorf("failed to initialize Minio client: %w", err)
	}

	// Ensure bucket exists
	bucketName := backup.Spec.StorageLocation.Bucket
	exists, err := minioClient.BucketExists(ctx, bucketName)
	if err != nil {
		return fmt.Errorf("failed to check bucket existence: %w", err)
	}
	if !exists {
		if err := minioClient.MakeBucket(ctx, bucketName, minio.MakeBucketOptions{}); err != nil {
			return fmt.Errorf("failed to create bucket: %w", err)
		}
		log.Info("Created bucket", "bucket", bucketName)
	}

	// Create backup timestamp and path
	timestamp := time.Now().Format("2006-01-02T15-04-05")
	backupPath := fmt.Sprintf("backups/cluster-backup/%s", timestamp)

	// Initialize resource counts
	resourceCounts := make(map[string]int32)

	// Determine which namespaces to backup
	namespacesToBackup, err := r.getNamespacesToBackup(ctx, backup.Spec.Source)
	if err != nil {
		return fmt.Errorf("failed to determine namespaces to backup: %w", err)
	}

	// Determine which resource types to backup
	resourceTypes := r.getResourceTypesToBackup(backup.Spec.Source)

	log.Info("Starting backup operation",
		"namespaces", namespacesToBackup,
		"resourceTypes", resourceTypes,
		"backupPath", backupPath,
		"includeClusterResources", backup.Spec.Source.IncludeClusterResources)

	// Backup namespace-scoped resources
	for _, namespace := range namespacesToBackup {
		for _, resourceType := range resourceTypes {
			count, err := r.backupNamespacedResourceType(ctx, minioClient, bucketName, backupPath, namespace, backup.Spec.Source, resourceType)
			if err != nil {
				return fmt.Errorf("failed to backup %s in namespace %s: %w", resourceType, namespace, err)
			}
			key := fmt.Sprintf("%s/%s", namespace, resourceType)
			resourceCounts[key] = count
			if count > 0 {
				log.Info("Backed up namespaced resources", "namespace", namespace, "type", resourceType, "count", count)
			}
		}
	}

	// Backup cluster-scoped resources if requested
	if backup.Spec.Source.IncludeClusterResources {
		clusterResourceTypes := r.getClusterResourceTypes()
		for _, resourceType := range clusterResourceTypes {
			count, err := r.backupClusterResourceType(ctx, minioClient, bucketName, backupPath, backup.Spec.Source, resourceType)
			if err != nil {
				return fmt.Errorf("failed to backup cluster resource %s: %w", resourceType, err)
			}
			key := fmt.Sprintf("cluster/%s", resourceType)
			resourceCounts[key] = count
			if count > 0 {
				log.Info("Backed up cluster resources", "type", resourceType, "count", count)
			}
		}
	}

	// Update backup status with results
	backup.Status.ResourceCounts = resourceCounts
	backup.Status.BackupPath = backupPath

	log.Info("Backup operation completed successfully",
		"namespaces", namespacesToBackup,
		"bucket", bucketName,
		"backupPath", backupPath,
		"resourceCounts", resourceCounts)

	return nil
}
```

#### Namespace Discovery Logic

```go
// getNamespacesToBackup determines which namespaces to backup based on the source configuration
func (r *BackupReconciler) getNamespacesToBackup(ctx context.Context, source backupv1.BackupSource) ([]string, error) {
	// Handle backward compatibility with single namespace field
	if source.Namespace != "" && len(source.Namespaces) == 0 {
		return []string{source.Namespace}, nil
	}

	// If namespaces are explicitly specified
	if len(source.Namespaces) > 0 {
		// Check for wildcard (all namespaces)
		for _, ns := range source.Namespaces {
			if ns == "*" {
				return r.getAllNamespaces(ctx, source.ExcludeNamespaces)
			}
		}
		return source.Namespaces, nil
	}

	// Default: backup all namespaces except system ones
	defaultExcludes := []string{"kube-system", "kube-public", "kube-node-lease"}
	excludes := append(defaultExcludes, source.ExcludeNamespaces...)
	return r.getAllNamespaces(ctx, excludes)
}

// getAllNamespaces gets all namespaces in the cluster, excluding specified ones
func (r *BackupReconciler) getAllNamespaces(ctx context.Context, excludeNamespaces []string) ([]string, error) {
	namespaceList := &corev1.NamespaceList{}
	if err := r.List(ctx, namespaceList); err != nil {
		return nil, fmt.Errorf("failed to list namespaces: %w", err)
	}

	var namespaces []string
	for _, ns := range namespaceList.Items {
		// Skip excluded namespaces
		excluded := false
		for _, exclude := range excludeNamespaces {
			if ns.Name == exclude {
				excluded = true
				break
			}
		}
		if !excluded {
			namespaces = append(namespaces, ns.Name)
		}
	}

	return namespaces, nil
}
```

#### Resource Type Handlers

```go
// getResourceTypesToBackup determines which resource types to backup based on the source configuration
func (r *BackupReconciler) getResourceTypesToBackup(source backupv1.BackupSource) []string {
	// If resource types are explicitly specified, use those
	if len(source.ResourceTypes) > 0 {
		return source.ResourceTypes
	}

	// Default resource types for cluster backup
	return []string{"deployments", "services", "configmaps", "secrets", "persistentvolumeclaims", "ingresses"}
}

// getClusterResourceTypes returns the list of cluster-scoped resource types to backup
func (r *BackupReconciler) getClusterResourceTypes() []string {
	return []string{"clusterroles", "clusterrolebindings", "persistentvolumes", "storageclasses"}
}
```

#### Minio Client Initialization

```go
// initMinioClient creates and configures a Minio client
func (r *BackupReconciler) initMinioClient(ctx context.Context, backup *backupv1.Backup) (*minio.Client, error) {
	storage := backup.Spec.StorageLocation

	// Get credentials from backup spec (simplified for workshop)
	accessKey := storage.AccessKey
	secretKey := storage.SecretKey

	// Use default credentials if not specified
	if accessKey == "" {
		accessKey = "minioadmin"
	}
	if secretKey == "" {
		secretKey = "minioadmin123"
	}

	// Parse endpoint URL
	endpoint := storage.Endpoint
	if endpoint == "" {
		return nil, fmt.Errorf("storage endpoint is required")
	}

	// Remove http:// or https:// prefix for minio client
	endpoint = strings.TrimPrefix(endpoint, "http://")
	endpoint = strings.TrimPrefix(endpoint, "https://")

	// Create Minio client
	minioClient, err := minio.New(endpoint, &minio.Options{
		Creds:  credentials.NewStaticV4(accessKey, secretKey, ""),
		Secure: false, // Use HTTP for workshop setup
	})
	if err != nil {
		return nil, fmt.Errorf("failed to create Minio client: %w", err)
	}

	return minioClient, nil
}
```

#### Resource Backup Implementation

Example implementation for deployments (similar pattern for other resources):

```go
// backupDeployments backs up all deployments in the specified namespace
func (r *BackupReconciler) backupDeployments(ctx context.Context, minioClient *minio.Client, bucket, backupPath, namespace string, source backupv1.BackupSource) (int32, error) {
	deployments := &appsv1.DeploymentList{}

	// Build list options with namespace and label selector
	listOpts := []client.ListOption{client.InNamespace(namespace)}
	if source.LabelSelector != nil {
		selector, err := metav1.LabelSelectorAsSelector(source.LabelSelector)
		if err != nil {
			return 0, fmt.Errorf("failed to convert label selector: %w", err)
		}
		listOpts = append(listOpts, client.MatchingLabelsSelector{Selector: selector})
	}

	if err := r.List(ctx, deployments, listOpts...); err != nil {
		return 0, fmt.Errorf("failed to list deployments: %w", err)
	}

	count := int32(0)
	for _, deployment := range deployments.Items {
		objectName := fmt.Sprintf("%s/namespaces/%s/deployments/%s.json", backupPath, namespace, deployment.Name)
		if err := r.uploadResource(ctx, minioClient, bucket, objectName, deployment); err != nil {
			return 0, fmt.Errorf("failed to backup deployment %s: %w", deployment.Name, err)
		}
		count++
	}

	return count, nil
}
```

#### Resource Upload Helper

```go
// uploadResource serializes a Kubernetes resource to JSON and uploads it to Minio
func (r *BackupReconciler) uploadResource(ctx context.Context, minioClient *minio.Client, bucket, objectName string, resource interface{}) error {
	// Serialize resource to JSON
	jsonData, err := json.MarshalIndent(resource, "", "  ")
	if err != nil {
		return fmt.Errorf("failed to marshal resource to JSON: %w", err)
	}

	// Upload to Minio
	reader := bytes.NewReader(jsonData)
	_, err = minioClient.PutObject(ctx, bucket, objectName, reader, int64(len(jsonData)), minio.PutObjectOptions{
		ContentType: "application/json",
	})
	if err != nil {
		return fmt.Errorf("failed to upload object to Minio: %w", err)
	}

	return nil
}
```

### 4. Generate Manifests and Build

After implementing the controller logic:

```bash
# Generate updated CRDs and RBAC
make manifests

# Build the controller
go build -o bin/manager cmd/main.go

# Build Docker image
make docker-build IMG=controller:latest

# Deploy to cluster
make deploy IMG=controller:latest
```

### 5. Enhanced Backup Controller Features

The backup controller now provides comprehensive cluster backup capabilities:

#### Source-Aware Backup Processing
- **Namespace Targeting**: Backup resources from specific namespaces
- **Resource Type Selection**: Choose which resource types to backup (deployments, services, configmaps, secrets, persistentvolumeclaims)
- **Label Selector Support**: Use Kubernetes label selectors to backup only specific resources
- **Default Resource Types**: When no resource types are specified, defaults to common cluster resources

#### Intelligent Resource Handling
- **System Resource Filtering**: Automatically skips system secrets (service account tokens, etc.)
- **Resource Counting**: Tracks how many resources of each type were backed up
- **Organized Storage**: Stores resources in organized directory structure by namespace, timestamp, and resource type
- **Status Tracking**: Detailed backup status with resource counts and backup paths

#### Enhanced Backup Structure
```
k8s-backups/
├── backups/
│   └── cluster-backup/
│       └── {timestamp}/
│           ├── namespaces/
│           │   ├── {namespace1}/
│           │   │   ├── deployments/
│           │   │   │   ├── app1.json
│           │   │   │   └── app2.json
│           │   │   ├── services/
│           │   │   │   └── app-service.json
│           │   │   ├── configmaps/
│           │   │   │   └── app-config.json
│           │   │   └── secrets/
│           │   │       └── app-secret.json
│           │   └── {namespace2}/
│           │       └── ...
│           └── cluster/
│               ├── clusterroles/
│               │   └── admin-role.json
│               ├── clusterrolebindings/
│               │   └── admin-binding.json
│               └── persistentvolumes/
│                   └── pv-data.json
```

### 3. Configuration Examples

The backup controller now supports comprehensive cluster backup with flexible resource selection:

#### Full Cluster Backup
```yaml
apiVersion: backup.cnd.dk/v1
kind: Backup
metadata:
  name: full-cluster-backup
  namespace: default
spec:
  source:
    namespaces: ["*"]  # All namespaces
    includeClusterResources: true
    excludeNamespaces: ["kube-system", "kube-public"]
  schedule: "0 2 * * *"  # Daily at 2 AM
  storageLocation:
    provider: "minio"
    bucket: "k8s-backups"
    endpoint: "http://172.17.0.3:9000"
    accessKey: "minioadmin"
    secretKey: "minioadmin123"
```

#### Multi-Namespace Production Backup
```yaml
apiVersion: backup.cnd.dk/v1
kind: Backup
metadata:
  name: production-backup
  namespace: default
spec:
  source:
    namespaces: ["production", "prod-data", "prod-monitoring"]
    resourceTypes: ["deployments", "services", "configmaps", "secrets"]
    labelSelector:
      matchLabels:
        environment: "production"
        backup: "enabled"
  schedule: "0 */6 * * *"  # Every 6 hours
  storageLocation:
    provider: "minio"
    bucket: "k8s-backups"
    endpoint: "http://172.17.0.3:9000"
    accessKey: "minioadmin"
    secretKey: "minioadmin123"
```

#### Configuration-Only Backup (All Namespaces)
```yaml
apiVersion: backup.cnd.dk/v1
kind: Backup
metadata:
  name: config-backup
  namespace: default
spec:
  source:
    namespaces: ["*"]
    resourceTypes: ["configmaps", "secrets"]
    excludeNamespaces: ["kube-system", "kube-public", "monitoring"]
  schedule: "0 */2 * * *"  # Every 2 hours
  storageLocation:
    provider: "minio"
    bucket: "k8s-backups"
    endpoint: "http://172.17.0.3:9000"
    accessKey: "minioadmin"
    secretKey: "minioadmin123"
```

#### Single Namespace (Backward Compatible)
```yaml
apiVersion: backup.cnd.dk/v1
kind: Backup
metadata:
  name: single-namespace-backup
  namespace: default
spec:
  source:
    namespace: "production"  # Legacy single namespace field
    resourceTypes: ["deployments", "services", "configmaps", "secrets"]
  schedule: "0 1 * * *"  # Daily at 1 AM
  storageLocation:
    provider: "minio"
    bucket: "k8s-backups"
    endpoint: "http://172.17.0.3:9000"
    accessKey: "minioadmin"
    secretKey: "minioadmin123"
```

### 4. Key Implementation Concepts

#### Controller Reconciliation Loop

The backup controller follows the standard Kubernetes controller pattern:

1. **Watch for Changes**: Monitors Backup custom resources
2. **Reconcile State**: Compares desired vs actual state
3. **Take Action**: Performs backup operations when needed
4. **Update Status**: Reports progress and results

```go
func (r *BackupReconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	log := logf.FromContext(ctx)

	// Fetch the Backup instance
	backup := &backupv1.Backup{}
	err := r.Get(ctx, req.NamespacedName, backup)
	if err != nil {
		// Handle not found (resource deleted)
		return ctrl.Result{}, client.IgnoreNotFound(err)
	}

	// Initialize status if not set
	if backup.Status.Phase == "" {
		backup.Status.Phase = backupv1.BackupPhaseNew
		backup.Status.Message = "Backup resource created"
		// Update status and requeue
	}

	// Check if it's time to perform backup
	if backup.Status.Phase == backupv1.BackupPhaseScheduled {
		// Perform the actual backup operation
		if err := r.performBackup(ctx, backup); err != nil {
			// Handle backup failure
		}
		// Update status to completed
	}

	return ctrl.Result{}, nil
}
```

#### Namespace Discovery Logic

The controller intelligently determines which namespaces to backup:

1. **Backward Compatibility**: Single `namespace` field still supported
2. **Explicit Lists**: `namespaces: ["prod", "staging"]` for specific namespaces  
3. **Wildcard Support**: `namespaces: ["*"]` for all namespaces
4. **Smart Exclusions**: Automatically excludes system namespaces unless specified

**Implementation Flow:**
```go
func (r *BackupReconciler) getNamespacesToBackup(ctx context.Context, source BackupSource) ([]string, error) {
	// 1. Check backward compatibility
	if source.Namespace != "" && len(source.Namespaces) == 0 {
		return []string{source.Namespace}, nil
	}
	
	// 2. Handle explicit namespace lists
	if len(source.Namespaces) > 0 {
		// Check for wildcard
		if contains(source.Namespaces, "*") {
			return r.getAllNamespaces(ctx, source.ExcludeNamespaces)
		}
		return source.Namespaces, nil
	}
	
	// 3. Default: all namespaces with smart exclusions
	defaultExcludes := []string{"kube-system", "kube-public", "kube-node-lease"}
	excludes := append(defaultExcludes, source.ExcludeNamespaces...)
	return r.getAllNamespaces(ctx, excludes)
}
```

#### Resource Type Handling

**Namespace-Scoped Resources:**
- `deployments` - Application workloads
- `services` - Service definitions and endpoints
- `configmaps` - Configuration data
- `secrets` - Sensitive data (with filtering)
- `persistentvolumeclaims` - Storage requests
- `ingresses` - HTTP routing rules

**Cluster-Scoped Resources:**
- `clusterroles` - Cluster-wide RBAC roles
- `clusterrolebindings` - Cluster-wide RBAC bindings  
- `persistentvolumes` - Actual storage volumes
- `storageclasses` - Storage provisioning templates

**Resource Selection Logic:**
```go
func (r *BackupReconciler) backupNamespacedResourceType(ctx context.Context, ..., resourceType string) (int32, error) {
	switch resourceType {
	case "deployments":
		return r.backupDeployments(ctx, ...)
	case "services":
		return r.backupServices(ctx, ...)
	// ... other resource types
	}
}
```

#### Backup Organization Structure

**Hierarchical Storage Layout:**
```
k8s-backups/
├── backups/
│   └── cluster-backup/
│       └── {timestamp}/
│           ├── namespaces/
│           │   ├── {namespace1}/
│           │   │   ├── deployments/
│           │   │   │   ├── app1.json
│           │   │   │   └── app2.json
│           │   │   ├── services/
│           │   │   │   └── app-service.json
│           │   │   └── configmaps/
│           │   │       └── app-config.json
│           │   └── {namespace2}/
│           │       └── ...
│           └── cluster/
│               ├── clusterroles/
│               │   └── admin-role.json
│               └── persistentvolumes/
│                   └── pv-data.json
```

**Benefits of This Structure:**
- **Easy Navigation**: Clear hierarchy for finding specific resources
- **Restore Friendly**: Structure matches Kubernetes organization
- **Scalable**: Handles large clusters with many namespaces
- **Extensible**: Easy to add new resource types

#### Status Tracking and Reporting

**Detailed Resource Counting:**
```go
// Example status after backup
status:
  phase: "Completed"
  resourceCounts:
    "production/deployments": 5
    "production/services": 3
    "production/configmaps": 8
    "staging/deployments": 2
    "cluster/clusterroles": 12
  backupPath: "backups/cluster-backup/2025-01-21T15-30-00"
  lastBackupTime: "2025-01-21T15:30:00Z"
```

**Progress Tracking:**
- **Phase Updates**: New → Scheduled → Running → Scheduled (continuous cycle)
- **Real-time Counts**: Resources backed up per namespace and type
- **Error Details**: Specific error messages for troubleshooting
- **Timing Information**: Backup duration and scheduling
- **Next Backup Scheduling**: Automatically calculates and sets next backup time based on cron schedule

#### Error Handling Strategy

**Graceful Degradation:**
```go
// Continue backup even if individual resources fail
for _, namespace := range namespacesToBackup {
	for _, resourceType := range resourceTypes {
		count, err := r.backupNamespacedResourceType(...)
		if err != nil {
			log.Error(err, "Failed to backup resource type", 
				"namespace", namespace, "type", resourceType)
			// Continue with next resource type instead of failing entire backup
			continue
		}
		resourceCounts[fmt.Sprintf("%s/%s", namespace, resourceType)] = count
	}
}
```

**Error Categories:**
- **Permission Errors**: Clear RBAC-related error messages
- **Network Issues**: Minio connection and upload failures
- **Resource Conflicts**: Handle resources that can't be serialized
- **Partial Failures**: Continue backup when individual resources fail

#### System Resource Filtering

**Automatic Exclusions:**
```go
// Skip system secrets automatically
if secret.Type == corev1.SecretTypeServiceAccountToken ||
   strings.HasPrefix(secret.Name, "default-token-") ||
   strings.Contains(secret.Name, "token-") {
	continue // Skip this secret
}
```

**Smart Filtering Benefits:**
- **Security**: Avoids backing up sensitive system tokens
- **Cleanliness**: Excludes auto-generated resources
- **Restore Safety**: Prevents conflicts during restore operations
- **Size Optimization**: Reduces backup size by excluding noise

#### Backup Scheduling Implementation

**NextBackupTime Calculation:**
```go
func (r *BackupReconciler) calculateNextBackupTime(schedule string) (time.Time, error) {
	// Parse the cron schedule using robfig/cron library
	parser := cron.NewParser(cron.Minute | cron.Hour | cron.Dom | cron.Month | cron.Dow | cron.Descriptor)
	
	cronSchedule, err := parser.Parse(schedule)
	if err != nil {
		return time.Time{}, fmt.Errorf("failed to parse cron schedule '%s': %w", schedule, err)
	}
	
	// Calculate next run time from now
	now := time.Now()
	nextTime := cronSchedule.Next(now)
	
	return nextTime, nil
}
```

**Scheduling Logic:**
- **Initial Scheduling**: Sets NextBackupTime when backup is first created
- **Time-Based Triggers**: Only runs backup when current time >= NextBackupTime
- **Automatic Rescheduling**: Calculates next backup time after each successful backup
- **Continuous Cycle**: After backup completion, returns to "Scheduled" phase for next run
- **Efficient Requeuing**: Uses RequeueAfter to wake up exactly when next backup is due
- **Production-Ready Parsing**: Uses `github.com/robfig/cron/v3` for full cron specification support
- **Error Handling**: Invalid cron schedules are properly detected and reported

**Backup Lifecycle:**
```
New → Scheduled → Running → Scheduled → Running → Scheduled → ...
  ↑      ↓          ↓          ↓          ↓          ↓
Create  Wait    Execute   Reschedule   Wait    Execute
```

**Supported Schedule Patterns (Full Cron Syntax):**
- `0 2 * * *` - Daily at 2 AM
- `0 */6 * * *` - Every 6 hours (at 0:00, 6:00, 12:00, 18:00)
- `0 */2 * * *` - Every 2 hours (at even hours)
- `*/5 * * * *` - Every 5 minutes
- `* * * * *` - Every minute
- `0 0 * * 0` - Weekly on Sunday at midnight
- `0 0 1 * *` - Monthly on the 1st at midnight
- `0 9-17 * * 1-5` - Every hour from 9 AM to 5 PM, Monday to Friday
- `@daily`, `@hourly`, `@weekly` - Cron descriptors supported

### 🔄 **Self-Sustaining Backup System**

The controller now implements a **continuous backup cycle**:

1. **Create Backup Resource** → Status: `New`
2. **Calculate First Schedule** → Status: `Scheduled`, NextBackupTime set
3. **Wait Until Due** → Controller sleeps using `RequeueAfter`
4. **Execute Backup** → Status: `Running`
5. **Complete & Reschedule** → Status: `Scheduled`, NextBackupTime updated
6. **Repeat from step 3** → Continuous operation

**Key Benefits:**
- ✅ **No Manual Intervention**: Backups run automatically according to schedule
- ✅ **Efficient Resource Usage**: Controller only wakes up when backup is due
- ✅ **Persistent Scheduling**: Survives controller restarts and pod rescheduling
- ✅ **Visible Status**: Always shows when next backup will occur
- ✅ **Failure Recovery**: Failed backups don't break the schedule cycle

### 5. Testing and Validation

#### Build and Deploy the Controller

After implementing all the code changes:

```bash
# 1. Generate updated CRDs and RBAC manifests
make manifests

# 2. Build the controller binary
go build -o bin/manager cmd/main.go

# 3. Run unit tests
go test ./internal/controller/... -v

# 4. Build Docker image
make docker-build IMG=controller:latest

# 5. Load image into kind cluster
kind load docker-image controller:latest

# 6. Deploy to cluster
make deploy IMG=controller:latest
```

#### Verify Controller Deployment

```bash
# Check if controller is running
kubectl get pods -n cluster-backup-operator-system

# View controller logs
kubectl logs -f deployment/cluster-backup-operator-controller-manager -n cluster-backup-operator-system

# Verify CRDs are installed
kubectl get crd backups.backup.cnd.dk
```

#### Test Different Backup Scenarios

**1. Single Namespace Backup:**
```bash
# Apply single namespace backup
kubectl apply -f config/samples/backup_v1_minio_backup.yaml

# Monitor backup progress
kubectl get backup minio-backup-sample -w

# Check backup status
kubectl describe backup minio-backup-sample
```

**2. Multi-Namespace Backup:**
```bash
# Apply comprehensive examples
kubectl apply -f config/samples/backup_v1_comprehensive_examples_v2.yaml

# Monitor all backups
kubectl get backups -A

# Check specific backup details
kubectl get backup production-backup -o yaml
```

**3. Full Cluster Backup:**
```bash
# Create full cluster backup
kubectl apply -f - <<EOF
apiVersion: backup.cnd.dk/v1
kind: Backup
metadata:
  name: full-cluster-test
  namespace: default
spec:
  source:
    namespaces: ["*"]
    includeClusterResources: true
    excludeNamespaces: ["kube-system", "kube-public"]
  schedule: "*/5 * * * *"
  storageLocation:
    provider: "minio"
    bucket: "k8s-backups"
    endpoint: "http://172.17.0.3:9000"
    accessKey: "minioadmin"
    secretKey: "minioadmin123"
EOF

# Watch backup execution
kubectl get backup full-cluster-test -w
```

#### Validate Backup Results

**Check Backup Status:**
```bash
# View detailed backup status
kubectl get backup <backup-name> -o jsonpath='{.status}' | jq

# Example output:
{
  "phase": "Completed",
  "message": "Backup completed successfully",
  "lastBackupTime": "2025-01-21T15:30:00Z",
  "backupCount": 1,
  "resourceCounts": {
    "default/deployments": 2,
    "default/services": 1,
    "default/configmaps": 3,
    "production/deployments": 5,
    "cluster/clusterroles": 15
  },
  "backupPath": "backups/cluster-backup/2025-01-21T15-30-00"
}
```

**Verify Files in Minio:**
```bash
# Use Minio client to list backup files
docker run --rm --network kind minio/mc:latest \
  alias set workshop http://workshop-minio:9000 minioadmin minioadmin123

docker run --rm --network kind minio/mc:latest \
  ls workshop/k8s-backups/backups/cluster-backup/ --recursive
```

#### Troubleshooting Common Issues

**1. Controller Not Starting:**
```bash
# Check controller logs
kubectl logs deployment/cluster-backup-operator-controller-manager -n cluster-backup-operator-system

# Common issues:
# - Image pull errors: Check image name and availability
# - RBAC errors: Verify service account permissions
# - CRD errors: Ensure CRDs are properly installed
```

**2. Backup Stuck in Running:**
```bash
# Check detailed backup status
kubectl describe backup <backup-name>

# View controller logs for specific backup
kubectl logs -f deployment/cluster-backup-operator-controller-manager -n cluster-backup-operator-system | grep <backup-name>

# Common causes:
# - Minio connection issues
# - Large number of resources
# - Permission errors for specific resource types
```

**3. Minio Connection Errors:**
```bash
# Test Minio connectivity from within cluster
kubectl run test-minio --image=curlimages/curl --rm -it -- sh
# Inside pod: curl -I http://172.17.0.3:9000/minio/health/live

# Check Minio container status
docker ps | grep minio
docker logs workshop-minio
```

### 6. Minio Credentials

For workshop simplicity, credentials are specified directly in the Backup resource. In production, you would use Kubernetes secrets or other secure credential management systems.

## Testing Procedures

### 1. Deploy Test Resources

Create some test resources to backup:

```bash
# Create test namespace
kubectl create namespace test-backup

# Create test resources
kubectl create deployment nginx --image=nginx -n test-backup
kubectl create configmap test-config --from-literal=key=value -n test-backup
kubectl create secret generic test-secret --from-literal=password=secret -n test-backup
```

### 2. Create Backup Resource

Apply a backup configuration:

```yaml
apiVersion: backup.cnd.dk/v1
kind: Backup
metadata:
  name: test-namespace-backup
  namespace: default
spec:
  source:
    namespace: "test-backup"
  schedule: "*/5 * * * *"  # Every 5 minutes for testing
  storageLocation:
    provider: "minio"
    bucket: "k8s-backups"
    endpoint: "http://172.17.0.3:9000"
    credentialsSecret: "minio-credentials"
```

### 3. Monitor Backup Execution

Watch the backup controller logs:

```bash
# View controller logs
kubectl logs -f deployment/cluster-backup-operator-controller-manager -n cluster-backup-operator-system

# Check backup status
kubectl get backup test-namespace-backup -o yaml
```

### 4. Verify Backup in Minio

Check that backup files are created in Minio:

1. Open Minio console at `http://localhost:9001`
2. Navigate to the `k8s-backups` bucket
3. Look for backup files organized by date and namespace

Expected structure:
```
k8s-backups/
├── backups/
│   └── test-backup/
│       └── 2025-01-XX/
│           ├── deployments/
│           │   └── nginx.json
│           ├── configmaps/
│           │   └── test-config.json
│           └── secrets/
│               └── test-secret.json
```

## Cross-Cluster Testing

### 1. Create Second Kind Cluster

```bash
# Create second cluster
kind create cluster --name backup-test-2

# Switch context
kubectl config use-context kind-backup-test-2
```

### 2. Install Operator in Second Cluster

```bash
# Deploy CRDs and operator
make deploy IMG=controller:latest
```

### 3. Test Backup Access

Create a backup resource in the second cluster that reads from the same Minio instance to verify cross-cluster functionality.

## Troubleshooting

### Common Issues

**1. Connection Refused to Minio**
- Check Minio container is running: `docker ps | grep minio`
- Verify network connectivity: `docker network inspect bridge`
- Ensure correct IP address in backup configuration

**2. Authentication Errors**
- Verify credentials secret exists: `kubectl get secret minio-credentials`
- Check secret contains correct keys: `kubectl describe secret minio-credentials`

**3. Backup Not Triggering**
- Check controller logs for errors
- Verify backup resource status: `kubectl describe backup <name>`
- Ensure schedule format is correct (cron syntax)

**4. Resources Not Found**
- Verify source namespace exists
- Check RBAC permissions for controller
- Ensure resources exist in the specified namespace

### Validation Commands

```bash
# Test Minio connectivity from within kind cluster
kubectl run test-pod --image=curlimages/curl --rm -it -- sh
# Inside pod: curl -I http://172.17.0.3:9000/minio/health/live

# Check backup controller permissions
kubectl auth can-i get deployments --as=system:serviceaccount:cluster-backup-operator-system:cluster-backup-operator-controller-manager

# Verify backup resource creation
kubectl get backups -A
kubectl describe backup <backup-name>
```

## What We've Accomplished

In this stage, we have successfully:

✅ **Created comprehensive Minio integration documentation**
- Detailed setup instructions for Docker Minio
- Network configuration for kind clusters
- Configuration examples and troubleshooting guides

✅ **Implemented complete backup functionality**
- Added Minio Go client dependency
- Implemented comprehensive backup logic with source-aware processing
- Added support for multiple Kubernetes resource types (Deployments, ConfigMaps, Secrets, Services, PVCs)
- Implemented label selector support for selective backups
- Added configurable resource type selection
- Proper error handling and detailed status tracking with resource counts

✅ **Enhanced RBAC permissions**
- Added necessary permissions for accessing Kubernetes resources
- Updated cluster role with proper resource access

✅ **Created comprehensive testing tools**
- Automated validation script (`validate-minio-backup.sh`)
- Cross-cluster demonstration script (`demo-cross-cluster-backup.sh`)
- Sample backup configurations for different scenarios

✅ **Verified functionality**
- Code compiles successfully with Minio integration
- Unit tests pass for basic functionality
- All configuration files are properly structured

## Next Steps

In Stage 5, we'll implement a restore controller that can read backup data from Minio and restore resources to different clusters, completing the cross-cluster backup and restore workflow.

## Automated Testing

### Quick Validation Script

We've provided an automated validation script to test the complete backup workflow:

```bash
# Run the validation script
./scripts/validate-minio-backup.sh
```

This script will:
1. Check all prerequisites (kubectl, kind cluster, Minio container, operator)
2. Create test resources (deployment, configmap, secret, service)
3. Create backup credentials and backup resource
4. Monitor backup execution
5. Verify backup files in Minio
6. Validate JSON structure of backup files
7. Clean up test resources

### Manual Testing Steps

If you prefer to test manually or need to troubleshoot:

```bash
# 1. Create test namespace and resources
kubectl create namespace test-backup
kubectl create deployment nginx --image=nginx -n test-backup
kubectl create configmap test-config --from-literal=key=value -n test-backup
kubectl create secret generic test-secret --from-literal=password=secret123 -n test-backup
kubectl expose deployment nginx --port=80 -n test-backup

# 2. Get Minio container IP
MINIO_IP=$(docker inspect workshop-minio | grep '"IPAddress"' | head -1 | cut -d '"' -f 4)
echo "Minio IP: $MINIO_IP"

# 3. Apply backup configuration (credentials are embedded for workshop simplicity)
cat <<EOF | kubectl apply -f -
apiVersion: backup.cnd.dk/v1
kind: Backup
metadata:
  name: test-backup
  namespace: default
spec:
  source:
    namespace: "test-backup"
  schedule: "*/1 * * * *"
  storageLocation:
    provider: "minio"
    bucket: "k8s-backups"
    endpoint: "http://${MINIO_IP}:9000"
    accessKey: "minioadmin"
    secretKey: "minioadmin123"
EOF

# 5. Monitor backup
kubectl get backup test-backup -w

# 6. Check backup files in Minio console
# Open http://localhost:9001 in browser
```

## Key Concepts Learned

- Docker container networking with Kubernetes
- Object storage integration patterns
- Kubernetes resource serialization
- Cross-cluster storage access
- Backup scheduling and automation
- Error handling in distributed systems
- RBAC permissions for resource access
- Kubernetes client-go usage patterns