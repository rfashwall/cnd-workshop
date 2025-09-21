/*
Copyright 2025.

Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at

    http://www.apache.org/licenses/LICENSE-2.0

Unless required by applicable law or agreed to in writing, software
distributed under the License is distributed on an "AS IS" BASIS,
WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
See the License for the specific language governing permissions and
limitations under the License.
*/

package controller

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

// RestoreReconciler reconciles a Restore object
type RestoreReconciler struct {
	client.Client
	Scheme *runtime.Scheme
}

//+kubebuilder:rbac:groups=backup.cnd.dk,resources=restores,verbs=get;list;watch;create;update;patch;delete
//+kubebuilder:rbac:groups=backup.cnd.dk,resources=restores/status,verbs=get;update;patch
//+kubebuilder:rbac:groups=backup.cnd.dk,resources=restores/finalizers,verbs=update
//+kubebuilder:rbac:groups=apps,resources=deployments,verbs=get;list;watch;create;update;patch;delete
//+kubebuilder:rbac:groups="",resources=configmaps,verbs=get;list;watch;create;update;patch;delete
//+kubebuilder:rbac:groups="",resources=secrets,verbs=get;list;watch;create;update;patch;delete
//+kubebuilder:rbac:groups="",resources=services,verbs=get;list;watch;create;update;patch;delete
//+kubebuilder:rbac:groups="",resources=persistentvolumeclaims,verbs=get;list;watch;create;update;patch;delete
//+kubebuilder:rbac:groups="",resources=namespaces,verbs=get;list;watch;create;update;patch;delete
//+kubebuilder:rbac:groups="",resources=persistentvolumes,verbs=get;list;watch;create;update;patch;delete
//+kubebuilder:rbac:groups=networking.k8s.io,resources=ingresses,verbs=get;list;watch;create;update;patch;delete
//+kubebuilder:rbac:groups=rbac.authorization.k8s.io,resources=clusterroles,verbs=get;list;watch;create;update;patch;delete
//+kubebuilder:rbac:groups=rbac.authorization.k8s.io,resources=clusterrolebindings,verbs=get;list;watch;create;update;patch;delete
//+kubebuilder:rbac:groups=storage.k8s.io,resources=storageclasses,verbs=get;list;watch;create;update;patch;delete

// Reconcile is part of the main kubernetes reconciliation loop which aims to
// move the current state of the cluster closer to the desired state.
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

	log.Info("Reconciling Restore", "restore", restore.Name, "namespace", restore.Namespace)

	// Initialize status if not set
	if restore.Status.Phase == "" {
		restore.Status.Phase = backupv1.RestorePhaseNew
		restore.Status.Message = "Restore resource created"
		restore.Status.StartTime = &metav1.Time{Time: time.Now()}
		if err := r.Status().Update(ctx, restore); err != nil {
			log.Error(err, "Failed to update Restore status")
			return ctrl.Result{}, err
		}
		log.Info("Initialized restore status", "phase", restore.Status.Phase)
		return ctrl.Result{Requeue: true}, nil
	}

	// Log the current restore configuration
	log.Info("Restore configuration",
		"source.backupPath", restore.Spec.Source.BackupPath,
		"target.namespaces", restore.Spec.Target.Namespaces,
		"target.resourceTypes", restore.Spec.Target.ResourceTypes,
		"current.phase", restore.Status.Phase)

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

	log.Info("Restore reconciliation completed", "restore", restore.Name)
	return ctrl.Result{}, nil
}

// handleValidatingPhase validates the restore configuration and backup source
func (r *RestoreReconciler) handleValidatingPhase(ctx context.Context, restore *backupv1.Restore) (ctrl.Result, error) {
	log := logf.FromContext(ctx)

	restore.Status.Phase = backupv1.RestorePhaseValidating
	restore.Status.Message = "Validating backup source and restore configuration"

	if err := r.Status().Update(ctx, restore); err != nil {
		log.Error(err, "Failed to update status to validating")
		return ctrl.Result{}, err
	}

	// Validate restore configuration
	if err := r.validateRestoreConfig(restore); err != nil {
		restore.Status.Phase = backupv1.RestorePhaseFailed
		restore.Status.Message = fmt.Sprintf("Validation failed: %v", err)
		restore.Status.CompletionTime = &metav1.Time{Time: time.Now()}
		if updateErr := r.Status().Update(ctx, restore); updateErr != nil {
			log.Error(updateErr, "Failed to update status to failed")
		}
		return ctrl.Result{}, err
	}

	// Validate backup source exists
	if err := r.validateBackupSource(ctx, restore); err != nil {
		restore.Status.Phase = backupv1.RestorePhaseFailed
		restore.Status.Message = fmt.Sprintf("Backup source validation failed: %v", err)
		restore.Status.CompletionTime = &metav1.Time{Time: time.Now()}
		if updateErr := r.Status().Update(ctx, restore); updateErr != nil {
			log.Error(updateErr, "Failed to update status to failed")
		}
		return ctrl.Result{}, err
	}

	log.Info("Validation completed successfully")
	return ctrl.Result{Requeue: true}, nil
}

// handleDownloadingPhase downloads and analyzes the backup data
func (r *RestoreReconciler) handleDownloadingPhase(ctx context.Context, restore *backupv1.Restore) (ctrl.Result, error) {
	log := logf.FromContext(ctx)

	restore.Status.Phase = backupv1.RestorePhaseDownloading
	restore.Status.Message = "Downloading and analyzing backup data"

	if err := r.Status().Update(ctx, restore); err != nil {
		log.Error(err, "Failed to update status to downloading")
		return ctrl.Result{}, err
	}

	// Analyze backup contents
	backupInfo, err := r.analyzeBackup(ctx, restore)
	if err != nil {
		restore.Status.Phase = backupv1.RestorePhaseFailed
		restore.Status.Message = fmt.Sprintf("Failed to analyze backup: %v", err)
		restore.Status.CompletionTime = &metav1.Time{Time: time.Now()}
		if updateErr := r.Status().Update(ctx, restore); updateErr != nil {
			log.Error(updateErr, "Failed to update status to failed")
		}
		return ctrl.Result{}, err
	}

	restore.Status.BackupInfo = backupInfo
	log.Info("Backup analysis completed", "totalResources", backupInfo.TotalResources, "namespaces", backupInfo.Namespaces)
	return ctrl.Result{Requeue: true}, nil
}

// handleRestoringPhase performs the actual restoration
func (r *RestoreReconciler) handleRestoringPhase(ctx context.Context, restore *backupv1.Restore) (ctrl.Result, error) {
	log := logf.FromContext(ctx)

	restore.Status.Phase = backupv1.RestorePhaseRestoring
	restore.Status.Message = "Restoring resources to target cluster"

	if err := r.Status().Update(ctx, restore); err != nil {
		log.Error(err, "Failed to update status to restoring")
		return ctrl.Result{}, err
	}

	// Perform the actual restore
	if err := r.performRestore(ctx, restore); err != nil {
		restore.Status.Phase = backupv1.RestorePhaseFailed
		restore.Status.Message = fmt.Sprintf("Restore failed: %v", err)
		restore.Status.CompletionTime = &metav1.Time{Time: time.Now()}
		if updateErr := r.Status().Update(ctx, restore); updateErr != nil {
			log.Error(updateErr, "Failed to update status to failed")
		}
		return ctrl.Result{}, err
	}

	restore.Status.Phase = backupv1.RestorePhaseCompleted
	restore.Status.Message = "Restore completed successfully"
	restore.Status.CompletionTime = &metav1.Time{Time: time.Now()}

	if err := r.Status().Update(ctx, restore); err != nil {
		log.Error(err, "Failed to update status to completed")
		return ctrl.Result{}, err
	}

	log.Info("Restore completed successfully")
	return ctrl.Result{}, nil
}

// validateRestoreConfig validates the restore configuration
func (r *RestoreReconciler) validateRestoreConfig(restore *backupv1.Restore) error {
	if restore.Spec.Source.BackupPath == "" {
		return fmt.Errorf("backup path is required")
	}

	if restore.Spec.Source.StorageLocation.Provider == "" {
		return fmt.Errorf("storage provider is required")
	}

	if restore.Spec.Source.StorageLocation.Bucket == "" {
		return fmt.Errorf("storage bucket is required")
	}

	if restore.Spec.Source.StorageLocation.Endpoint == "" {
		return fmt.Errorf("storage endpoint is required")
	}

	// Validate conflict resolution strategy
	conflictResolution := restore.Spec.Target.ConflictResolution
	if conflictResolution != "" && conflictResolution != "skip" && conflictResolution != "overwrite" && conflictResolution != "fail" {
		return fmt.Errorf("invalid conflict resolution strategy: %s (must be skip, overwrite, or fail)", conflictResolution)
	}

	return nil
}

// validateBackupSource validates that the backup source exists and is accessible
func (r *RestoreReconciler) validateBackupSource(ctx context.Context, restore *backupv1.Restore) error {
	minioClient, err := r.initMinioClient(ctx, restore)
	if err != nil {
		return fmt.Errorf("failed to initialize Minio client: %w", err)
	}

	// Check if bucket exists
	bucketName := restore.Spec.Source.StorageLocation.Bucket
	exists, err := minioClient.BucketExists(ctx, bucketName)
	if err != nil {
		return fmt.Errorf("failed to check bucket existence: %w", err)
	}
	if !exists {
		return fmt.Errorf("bucket %s does not exist", bucketName)
	}

	// Check if backup path exists by listing objects
	backupPath := restore.Spec.Source.BackupPath
	objectCh := minioClient.ListObjects(ctx, bucketName, minio.ListObjectsOptions{
		Prefix:    backupPath,
		Recursive: true,
	})

	hasObjects := false
	for object := range objectCh {
		if object.Err != nil {
			return fmt.Errorf("failed to list backup objects: %w", object.Err)
		}
		hasObjects = true
		break // We just need to know if any objects exist
	}

	if !hasObjects {
		return fmt.Errorf("no backup found at path %s", backupPath)
	}

	return nil
}

// analyzeBackup analyzes the backup contents and returns backup information
func (r *RestoreReconciler) analyzeBackup(ctx context.Context, restore *backupv1.Restore) (*backupv1.BackupInfo, error) {
	minioClient, err := r.initMinioClient(ctx, restore)
	if err != nil {
		return nil, fmt.Errorf("failed to initialize Minio client: %w", err)
	}

	bucketName := restore.Spec.Source.StorageLocation.Bucket
	backupPath := restore.Spec.Source.BackupPath

	// List all objects in the backup path
	objectCh := minioClient.ListObjects(ctx, bucketName, minio.ListObjectsOptions{
		Prefix:    backupPath,
		Recursive: true,
	})

	backupInfo := &backupv1.BackupInfo{
		BackupPath:     backupPath,
		TotalResources: 0,
		ResourceTypes:  []string{},
		Namespaces:     []string{},
	}

	resourceTypeSet := make(map[string]bool)
	namespaceSet := make(map[string]bool)

	for object := range objectCh {
		if object.Err != nil {
			return nil, fmt.Errorf("failed to list backup objects: %w", object.Err)
		}

		// Parse object path to extract namespace and resource type
		// Expected format: backups/cluster-backup/timestamp/namespaces/namespace/resourcetype/resource.json
		pathParts := strings.Split(object.Key, "/")
		if len(pathParts) >= 6 && pathParts[len(pathParts)-3] != "" && pathParts[len(pathParts)-2] != "" {
			namespace := pathParts[len(pathParts)-3]
			resourceType := pathParts[len(pathParts)-2]

			if !namespaceSet[namespace] {
				backupInfo.Namespaces = append(backupInfo.Namespaces, namespace)
				namespaceSet[namespace] = true
			}

			if !resourceTypeSet[resourceType] {
				backupInfo.ResourceTypes = append(backupInfo.ResourceTypes, resourceType)
				resourceTypeSet[resourceType] = true
			}

			backupInfo.TotalResources++
		}
	}

	return backupInfo, nil
}

// performRestore performs the actual restoration of resources
func (r *RestoreReconciler) performRestore(ctx context.Context, restore *backupv1.Restore) error {
	log := logf.FromContext(ctx)

	minioClient, err := r.initMinioClient(ctx, restore)
	if err != nil {
		return fmt.Errorf("failed to initialize Minio client: %w", err)
	}

	bucketName := restore.Spec.Source.StorageLocation.Bucket
	backupPath := restore.Spec.Source.BackupPath

	// Initialize counters
	resourceCounts := make(map[string]int32)
	var restoredResources []backupv1.RestoredResource
	var failedResources []backupv1.FailedResource
	var skippedResources []backupv1.SkippedResource

	// Get target namespaces and resource types
	targetNamespaces := r.getTargetNamespaces(restore)
	targetResourceTypes := r.getTargetResourceTypes(restore)

	// Create target namespaces if needed
	if restore.Spec.Options.CreateNamespaces {
		for _, ns := range targetNamespaces {
			if err := r.ensureNamespaceExists(ctx, ns); err != nil {
				log.Error(err, "Failed to create namespace", "namespace", ns)
				failedResources = append(failedResources, backupv1.FailedResource{
					APIVersion: "v1",
					Kind:       "Namespace",
					Name:       ns,
					Error:      err.Error(),
				})
			}
		}
	}

	// List and restore resources
	objectCh := minioClient.ListObjects(ctx, bucketName, minio.ListObjectsOptions{
		Prefix:    backupPath,
		Recursive: true,
	})

	for object := range objectCh {
		if object.Err != nil {
			return fmt.Errorf("failed to list backup objects: %w", object.Err)
		}

		// Skip non-JSON files
		if !strings.HasSuffix(object.Key, ".json") {
			continue
		}

		// Parse object path to extract namespace and resource type
		pathParts := strings.Split(object.Key, "/")
		if len(pathParts) < 6 {
			continue
		}

		sourceNamespace := pathParts[len(pathParts)-3]
		resourceType := pathParts[len(pathParts)-2]
		resourceName := strings.TrimSuffix(pathParts[len(pathParts)-1], ".json")

		// Check if we should restore this resource type
		if len(targetResourceTypes) > 0 {
			found := false
			for _, rt := range targetResourceTypes {
				if rt == resourceType {
					found = true
					break
				}
			}
			if !found {
				continue
			}
		}

		// Determine target namespace
		targetNamespace := r.mapNamespace(sourceNamespace, restore.Spec.Target)

		// Check if we should restore to this namespace
		if len(targetNamespaces) > 0 {
			found := false
			for _, ns := range targetNamespaces {
				if ns == targetNamespace {
					found = true
					break
				}
			}
			if !found {
				continue
			}
		}

		// Download and restore the resource
		if restore.Spec.Options.DryRun || restore.Spec.Options.ValidateOnly {
			// For dry run, just validate the resource
			log.Info("Dry run: would restore resource", "type", resourceType, "name", resourceName, "namespace", targetNamespace)
			restoredResources = append(restoredResources, backupv1.RestoredResource{
				Kind:      resourceType,
				Name:      resourceName,
				Namespace: targetNamespace,
				Action:    "dry-run",
			})
		} else {
			// Actually restore the resource
			result, err := r.restoreResource(ctx, minioClient, bucketName, object.Key, targetNamespace, restore.Spec.Target.ConflictResolution)
			if err != nil {
				log.Error(err, "Failed to restore resource", "type", resourceType, "name", resourceName, "namespace", targetNamespace)
				failedResources = append(failedResources, backupv1.FailedResource{
					Kind:      resourceType,
					Name:      resourceName,
					Namespace: targetNamespace,
					Error:     err.Error(),
				})
			} else if result.Action == "skipped" {
				skippedResources = append(skippedResources, backupv1.SkippedResource{
					Kind:      resourceType,
					Name:      resourceName,
					Namespace: targetNamespace,
					Reason:    result.Reason,
				})
			} else {
				restoredResources = append(restoredResources, backupv1.RestoredResource{
					APIVersion: result.APIVersion,
					Kind:       result.Kind,
					Name:       result.Name,
					Namespace:  result.Namespace,
					Action:     result.Action,
				})
			}
		}

		// Update resource counts
		key := fmt.Sprintf("%s/%s", targetNamespace, resourceType)
		resourceCounts[key]++
	}

	// Update restore status with results
	restore.Status.ResourceCounts = resourceCounts
	restore.Status.RestoredResources = restoredResources
	restore.Status.FailedResources = failedResources
	restore.Status.SkippedResources = skippedResources

	log.Info("Restore operation completed",
		"restored", len(restoredResources),
		"failed", len(failedResources),
		"skipped", len(skippedResources))

	return nil
}

// RestoreResult represents the result of restoring a single resource
type RestoreResult struct {
	APIVersion string
	Kind       string
	Name       string
	Namespace  string
	Action     string // created, updated, skipped
	Reason     string // reason for skipping
}

// restoreResource restores a single resource from backup
func (r *RestoreReconciler) restoreResource(ctx context.Context, minioClient *minio.Client, bucket, objectKey, targetNamespace, conflictResolution string) (*RestoreResult, error) {
	// Download the resource JSON
	object, err := minioClient.GetObject(ctx, bucket, objectKey, minio.GetObjectOptions{})
	if err != nil {
		return nil, fmt.Errorf("failed to download resource: %w", err)
	}
	defer object.Close()

	// Read the JSON data
	buf := new(bytes.Buffer)
	_, err = buf.ReadFrom(object)
	if err != nil {
		return nil, fmt.Errorf("failed to read resource data: %w", err)
	}

	// Parse the resource
	var resource unstructured.Unstructured
	if err := json.Unmarshal(buf.Bytes(), &resource); err != nil {
		return nil, fmt.Errorf("failed to parse resource JSON: %w", err)
	}

	// Clean up the resource for restoration
	r.cleanResourceForRestore(&resource, targetNamespace)

	result := &RestoreResult{
		APIVersion: resource.GetAPIVersion(),
		Kind:       resource.GetKind(),
		Name:       resource.GetName(),
		Namespace:  resource.GetNamespace(),
	}

	// Check if resource already exists
	existing := &unstructured.Unstructured{}
	existing.SetGroupVersionKind(resource.GetObjectKind().GroupVersionKind())

	var getErr error
	if resource.GetNamespace() != "" {
		getErr = r.Get(ctx, client.ObjectKey{Name: resource.GetName(), Namespace: resource.GetNamespace()}, existing)
	} else {
		getErr = r.Get(ctx, client.ObjectKey{Name: resource.GetName()}, existing)
	}

	if getErr == nil {
		// Resource exists, handle conflict resolution
		switch conflictResolution {
		case "skip", "":
			result.Action = "skipped"
			result.Reason = "resource already exists"
			return result, nil
		case "fail":
			return nil, fmt.Errorf("resource %s/%s already exists", resource.GetKind(), resource.GetName())
		case "overwrite":
			// Update the existing resource
			resource.SetResourceVersion(existing.GetResourceVersion())
			if err := r.Update(ctx, &resource); err != nil {
				return nil, fmt.Errorf("failed to update resource: %w", err)
			}
			result.Action = "updated"
		}
	} else if errors.IsNotFound(getErr) {
		// Resource doesn't exist, create it
		if err := r.Create(ctx, &resource); err != nil {
			return nil, fmt.Errorf("failed to create resource: %w", err)
		}
		result.Action = "created"
	} else {
		return nil, fmt.Errorf("failed to check if resource exists: %w", getErr)
	}

	return result, nil
}

// cleanResourceForRestore removes fields that shouldn't be restored
func (r *RestoreReconciler) cleanResourceForRestore(resource *unstructured.Unstructured, targetNamespace string) {
	// Remove metadata fields that shouldn't be restored
	unstructured.RemoveNestedField(resource.Object, "metadata", "resourceVersion")
	unstructured.RemoveNestedField(resource.Object, "metadata", "uid")
	unstructured.RemoveNestedField(resource.Object, "metadata", "generation")
	unstructured.RemoveNestedField(resource.Object, "metadata", "creationTimestamp")
	unstructured.RemoveNestedField(resource.Object, "metadata", "deletionTimestamp")
	unstructured.RemoveNestedField(resource.Object, "metadata", "deletionGracePeriodSeconds")
	unstructured.RemoveNestedField(resource.Object, "metadata", "selfLink")
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

// getTargetNamespaces returns the list of target namespaces for restoration
func (r *RestoreReconciler) getTargetNamespaces(restore *backupv1.Restore) []string {
	if len(restore.Spec.Target.Namespaces) > 0 {
		return restore.Spec.Target.Namespaces
	}

	// If no target namespaces specified, use all namespaces from backup
	if restore.Status.BackupInfo != nil {
		return restore.Status.BackupInfo.Namespaces
	}

	return []string{}
}

// getTargetResourceTypes returns the list of resource types to restore
func (r *RestoreReconciler) getTargetResourceTypes(restore *backupv1.Restore) []string {
	if len(restore.Spec.Target.ResourceTypes) > 0 {
		return restore.Spec.Target.ResourceTypes
	}

	// If no resource types specified, restore all types from backup
	if restore.Status.BackupInfo != nil {
		return restore.Status.BackupInfo.ResourceTypes
	}

	return []string{}
}

// mapNamespace maps a source namespace to a target namespace
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

// ensureNamespaceExists creates a namespace if it doesn't exist
func (r *RestoreReconciler) ensureNamespaceExists(ctx context.Context, namespace string) error {
	ns := &corev1.Namespace{}
	err := r.Get(ctx, client.ObjectKey{Name: namespace}, ns)
	if err == nil {
		// Namespace already exists
		return nil
	}

	if !errors.IsNotFound(err) {
		return fmt.Errorf("failed to check namespace existence: %w", err)
	}

	// Create the namespace
	ns = &corev1.Namespace{
		ObjectMeta: metav1.ObjectMeta{
			Name: namespace,
		},
	}

	if err := r.Create(ctx, ns); err != nil {
		return fmt.Errorf("failed to create namespace: %w", err)
	}

	return nil
}

// initMinioClient creates and configures a Minio client for the restore operation
func (r *RestoreReconciler) initMinioClient(ctx context.Context, restore *backupv1.Restore) (*minio.Client, error) {
	storage := restore.Spec.Source.StorageLocation

	// Get credentials from restore spec (simplified for workshop)
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

// SetupWithManager sets up the controller with the Manager.
func (r *RestoreReconciler) SetupWithManager(mgr ctrl.Manager) error {
	return ctrl.NewControllerManagedBy(mgr).
		For(&backupv1.Restore{}).
		Complete(r)
}
