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

	"k8s.io/apimachinery/pkg/runtime"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	logf "sigs.k8s.io/controller-runtime/pkg/log"

	"github.com/minio/minio-go/v7"
	"github.com/minio/minio-go/v7/pkg/credentials"
	backupv1 "github.com/rfashwall/cnd-workshop/api/v1"
	"github.com/robfig/cron/v3"

	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// BackupReconciler reconciles a Backup object
type BackupReconciler struct {
	client.Client
	Scheme *runtime.Scheme
}

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

// Reconcile is part of the main kubernetes reconciliation loop which aims to
// move the current state of the cluster closer to the desired state.
func (r *BackupReconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	log := logf.FromContext(ctx)

	// Fetch the Backup instance
	backup := &backupv1.Backup{}
	err := r.Get(ctx, req.NamespacedName, backup)
	if err != nil {
		if client.IgnoreNotFound(err) == nil {
			log.Info("Backup resource not found. Ignoring since object must be deleted")
			return ctrl.Result{}, nil
		}
		log.Error(err, "Failed to get Backup")
		return ctrl.Result{}, err
	}

	log.Info("Reconciling Backup", "backup", backup.Name, "namespace", backup.Namespace)

	// Initialize status if not set
	if backup.Status.Phase == "" {
		backup.Status.Phase = backupv1.BackupPhaseNew
		backup.Status.Message = "Backup resource created"
		if err := r.Status().Update(ctx, backup); err != nil {
			log.Error(err, "Failed to update Backup status")
			return ctrl.Result{}, err
		}
		log.Info("Initialized backup status", "phase", backup.Status.Phase)
		return ctrl.Result{Requeue: true}, nil
	}

	// Log the current backup configuration
	log.Info("Backup configuration",
		"source.namespace", backup.Spec.Source.Namespace,
		"schedule", backup.Spec.Schedule,
		"storage.provider", backup.Spec.StorageLocation.Provider,
		"storage.bucket", backup.Spec.StorageLocation.Bucket,
		"current.phase", backup.Status.Phase,
		"nextBackupTime", backup.Status.NextBackupTime)

	// Update phase to scheduled if still new
	if backup.Status.Phase == backupv1.BackupPhaseNew {
		backup.Status.Phase = backupv1.BackupPhaseScheduled
		backup.Status.Message = "Backup scheduled according to cron schedule"

		// Set initial next backup time
		if nextTime, err := r.calculateNextBackupTime(backup.Spec.Schedule); err == nil {
			backup.Status.NextBackupTime = &metav1.Time{Time: nextTime}
		} else {
			log.Error(err, "Failed to parse cron schedule, backup will not be scheduled", "schedule", backup.Spec.Schedule)
			backup.Status.Message = fmt.Sprintf("Invalid cron schedule '%s': %v", backup.Spec.Schedule, err)
		}

		if err := r.Status().Update(ctx, backup); err != nil {
			log.Error(err, "Failed to update Backup status to scheduled")
			return ctrl.Result{}, err
		}
		log.Info("Updated backup status to scheduled", "nextBackupTime", backup.Status.NextBackupTime)
		return ctrl.Result{Requeue: true}, nil
	}

	// Check if it's time to perform a backup
	// Note: Backup lifecycle is New -> Scheduled -> Running -> Scheduled (for next run)
	if backup.Status.Phase == backupv1.BackupPhaseScheduled {
		// Check if it's time to run the backup
		now := time.Now()
		shouldRunBackup := false

		if backup.Status.NextBackupTime == nil {
			// No next backup time set, run immediately (for workshop)
			shouldRunBackup = true
			log.Info("No next backup time set, running backup immediately")
		} else if now.After(backup.Status.NextBackupTime.Time) {
			// It's time to run the backup
			shouldRunBackup = true
			log.Info("Backup time reached", "scheduledTime", backup.Status.NextBackupTime.Time, "currentTime", now)
		} else {
			// Not time yet, requeue for later
			timeUntilNext := backup.Status.NextBackupTime.Time.Sub(now)
			log.Info("Backup not due yet", "timeUntilNext", timeUntilNext, "nextBackupTime", backup.Status.NextBackupTime.Time)
			return ctrl.Result{RequeueAfter: timeUntilNext}, nil
		}

		if shouldRunBackup {
			if err := r.performBackup(ctx, backup); err != nil {
				backup.Status.Phase = backupv1.BackupPhaseFailed
				backup.Status.Message = fmt.Sprintf("Backup failed: %v", err)
				if updateErr := r.Status().Update(ctx, backup); updateErr != nil {
					log.Error(updateErr, "Failed to update backup status to failed")
				}
				return ctrl.Result{}, err
			}

			backup.Status.Phase = backupv1.BackupPhaseCompleted
			backup.Status.Message = "Backup completed successfully"
			backup.Status.LastBackupTime = &metav1.Time{Time: time.Now()}
			backup.Status.BackupCount++

			// Calculate next backup time and schedule next run
			if nextTime, err := r.calculateNextBackupTime(backup.Spec.Schedule); err == nil {
				backup.Status.NextBackupTime = &metav1.Time{Time: nextTime}

				// Set phase back to scheduled for next backup
				backup.Status.Phase = backupv1.BackupPhaseScheduled
				backup.Status.Message = fmt.Sprintf("Backup completed successfully. Next backup scheduled for %s", nextTime.Format("2006-01-02 15:04:05"))

				if err := r.Status().Update(ctx, backup); err != nil {
					log.Error(err, "Failed to update backup status")
					return ctrl.Result{}, err
				}

				// Calculate time until next backup
				timeUntilNext := nextTime.Sub(time.Now())
				log.Info("Backup completed successfully, scheduled next backup",
					"nextBackupTime", nextTime,
					"timeUntilNext", timeUntilNext)

				// Requeue for the next backup time
				return ctrl.Result{RequeueAfter: timeUntilNext}, nil
			} else {
				// Schedule parsing failed, mark as completed and don't reschedule
				log.Error(err, "Failed to parse cron schedule for next backup", "schedule", backup.Spec.Schedule)
				backup.Status.Phase = backupv1.BackupPhaseCompleted
				backup.Status.Message = fmt.Sprintf("Backup completed successfully, but failed to schedule next backup: invalid cron schedule '%s'", backup.Spec.Schedule)
				if err := r.Status().Update(ctx, backup); err != nil {
					log.Error(err, "Failed to update backup status to completed")
					return ctrl.Result{}, err
				}
				log.Info("Backup completed successfully, but next backup not scheduled due to invalid cron schedule")
			}
		}
	}

	log.Info("Backup reconciliation completed", "backup", backup.Name)
	return ctrl.Result{}, nil
}

// performBackup executes the actual backup operation
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

// getResourceTypesToBackup determines which resource types to backup based on the source configuration
func (r *BackupReconciler) getResourceTypesToBackup(source backupv1.BackupSource) []string {
	// If resource types are explicitly specified, use those
	if len(source.ResourceTypes) > 0 {
		return source.ResourceTypes
	}

	// Default resource types for cluster backup
	return []string{"deployments", "services", "configmaps", "secrets", "persistentvolumeclaims", "ingresses"}
}

// Note: backupResourceType function removed - replaced by backupNamespacedResourceType

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

// getClusterResourceTypes returns the list of cluster-scoped resource types to backup
func (r *BackupReconciler) getClusterResourceTypes() []string {
	return []string{"clusterroles", "clusterrolebindings", "persistentvolumes", "storageclasses"}
}

// calculateNextBackupTime calculates the next backup time based on the cron schedule
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

// backupNamespacedResourceType backs up all resources of a specific type in a specific namespace
func (r *BackupReconciler) backupNamespacedResourceType(ctx context.Context, minioClient *minio.Client, bucket, backupPath, namespace string, source backupv1.BackupSource, resourceType string) (int32, error) {
	switch resourceType {
	case "deployments":
		return r.backupDeployments(ctx, minioClient, bucket, backupPath, namespace, source)
	case "services":
		return r.backupServices(ctx, minioClient, bucket, backupPath, namespace, source)
	case "configmaps":
		return r.backupConfigMaps(ctx, minioClient, bucket, backupPath, namespace, source)
	case "secrets":
		return r.backupSecrets(ctx, minioClient, bucket, backupPath, namespace, source)
	case "persistentvolumeclaims":
		return r.backupPersistentVolumeClaims(ctx, minioClient, bucket, backupPath, namespace, source)
	case "ingresses":
		return r.backupIngresses(ctx, minioClient, bucket, backupPath, namespace, source)
	default:
		return 0, nil
	}
}

// backupClusterResourceType backs up cluster-scoped resources
func (r *BackupReconciler) backupClusterResourceType(ctx context.Context, minioClient *minio.Client, bucket, backupPath string, source backupv1.BackupSource, resourceType string) (int32, error) {
	switch resourceType {
	case "clusterroles":
		return r.backupClusterRoles(ctx, minioClient, bucket, backupPath, source)
	case "clusterrolebindings":
		return r.backupClusterRoleBindings(ctx, minioClient, bucket, backupPath, source)
	case "persistentvolumes":
		return r.backupPersistentVolumes(ctx, minioClient, bucket, backupPath, source)
	case "storageclasses":
		return r.backupStorageClasses(ctx, minioClient, bucket, backupPath, source)
	default:
		return 0, nil
	}
}

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

// backupConfigMaps backs up all configmaps in the specified namespace
func (r *BackupReconciler) backupConfigMaps(ctx context.Context, minioClient *minio.Client, bucket, backupPath, namespace string, source backupv1.BackupSource) (int32, error) {
	configMaps := &corev1.ConfigMapList{}

	// Build list options with namespace and label selector
	listOpts := []client.ListOption{client.InNamespace(namespace)}
	if source.LabelSelector != nil {
		selector, err := metav1.LabelSelectorAsSelector(source.LabelSelector)
		if err != nil {
			return 0, fmt.Errorf("failed to convert label selector: %w", err)
		}
		listOpts = append(listOpts, client.MatchingLabelsSelector{Selector: selector})
	}

	if err := r.List(ctx, configMaps, listOpts...); err != nil {
		return 0, fmt.Errorf("failed to list configmaps: %w", err)
	}

	count := int32(0)
	for _, cm := range configMaps.Items {
		objectName := fmt.Sprintf("%s/namespaces/%s/configmaps/%s.json", backupPath, namespace, cm.Name)
		if err := r.uploadResource(ctx, minioClient, bucket, objectName, cm); err != nil {
			return 0, fmt.Errorf("failed to backup configmap %s: %w", cm.Name, err)
		}
		count++
	}

	return count, nil
}

// backupSecrets backs up all secrets in the specified namespace
func (r *BackupReconciler) backupSecrets(ctx context.Context, minioClient *minio.Client, bucket, backupPath, namespace string, source backupv1.BackupSource) (int32, error) {
	secrets := &corev1.SecretList{}

	// Build list options with namespace and label selector
	listOpts := []client.ListOption{client.InNamespace(namespace)}
	if source.LabelSelector != nil {
		selector, err := metav1.LabelSelectorAsSelector(source.LabelSelector)
		if err != nil {
			return 0, fmt.Errorf("failed to convert label selector: %w", err)
		}
		listOpts = append(listOpts, client.MatchingLabelsSelector{Selector: selector})
	}

	if err := r.List(ctx, secrets, listOpts...); err != nil {
		return 0, fmt.Errorf("failed to list secrets: %w", err)
	}

	count := int32(0)
	for _, secret := range secrets.Items {
		// Skip service account tokens and other system secrets
		if secret.Type == corev1.SecretTypeServiceAccountToken ||
			strings.HasPrefix(secret.Name, "default-token-") ||
			strings.Contains(secret.Name, "token-") {
			continue
		}

		objectName := fmt.Sprintf("%s/namespaces/%s/secrets/%s.json", backupPath, namespace, secret.Name)
		if err := r.uploadResource(ctx, minioClient, bucket, objectName, secret); err != nil {
			return 0, fmt.Errorf("failed to backup secret %s: %w", secret.Name, err)
		}
		count++
	}

	return count, nil
}

// backupServices backs up all services in the specified namespace
func (r *BackupReconciler) backupServices(ctx context.Context, minioClient *minio.Client, bucket, backupPath, namespace string, source backupv1.BackupSource) (int32, error) {
	services := &corev1.ServiceList{}

	// Build list options with namespace and label selector
	listOpts := []client.ListOption{client.InNamespace(namespace)}
	if source.LabelSelector != nil {
		selector, err := metav1.LabelSelectorAsSelector(source.LabelSelector)
		if err != nil {
			return 0, fmt.Errorf("failed to convert label selector: %w", err)
		}
		listOpts = append(listOpts, client.MatchingLabelsSelector{Selector: selector})
	}

	if err := r.List(ctx, services, listOpts...); err != nil {
		return 0, fmt.Errorf("failed to list services: %w", err)
	}

	count := int32(0)
	for _, service := range services.Items {
		// Skip default kubernetes service and system services
		if service.Name == "kubernetes" ||
			service.Namespace == "kube-system" ||
			service.Namespace == "kube-public" {
			continue
		}

		objectName := fmt.Sprintf("%s/namespaces/%s/services/%s.json", backupPath, namespace, service.Name)
		if err := r.uploadResource(ctx, minioClient, bucket, objectName, service); err != nil {
			return 0, fmt.Errorf("failed to backup service %s: %w", service.Name, err)
		}
		count++
	}

	return count, nil
}

// backupPersistentVolumeClaims backs up all PVCs in the specified namespace
func (r *BackupReconciler) backupPersistentVolumeClaims(ctx context.Context, minioClient *minio.Client, bucket, backupPath, namespace string, source backupv1.BackupSource) (int32, error) {
	pvcs := &corev1.PersistentVolumeClaimList{}

	// Build list options with namespace and label selector
	listOpts := []client.ListOption{client.InNamespace(namespace)}
	if source.LabelSelector != nil {
		selector, err := metav1.LabelSelectorAsSelector(source.LabelSelector)
		if err != nil {
			return 0, fmt.Errorf("failed to convert label selector: %w", err)
		}
		listOpts = append(listOpts, client.MatchingLabelsSelector{Selector: selector})
	}

	if err := r.List(ctx, pvcs, listOpts...); err != nil {
		return 0, fmt.Errorf("failed to list persistentvolumeclaims: %w", err)
	}

	count := int32(0)
	for _, pvc := range pvcs.Items {
		objectName := fmt.Sprintf("%s/namespaces/%s/persistentvolumeclaims/%s.json", backupPath, namespace, pvc.Name)
		if err := r.uploadResource(ctx, minioClient, bucket, objectName, pvc); err != nil {
			return 0, fmt.Errorf("failed to backup persistentvolumeclaim %s: %w", pvc.Name, err)
		}
		count++
	}

	return count, nil
}

// backupIngresses backs up all ingresses in the specified namespace
func (r *BackupReconciler) backupIngresses(ctx context.Context, minioClient *minio.Client, bucket, backupPath, namespace string, source backupv1.BackupSource) (int32, error) {
	// Note: Using unversioned client for ingresses as they might be in different API versions
	// For simplicity in the workshop, we'll skip ingresses if they're not available
	// In a real implementation, you'd handle multiple API versions

	// For now, return 0 count as ingresses require more complex API version handling
	return 0, nil
}

// backupClusterRoles backs up all cluster roles
func (r *BackupReconciler) backupClusterRoles(ctx context.Context, minioClient *minio.Client, bucket, backupPath string, source backupv1.BackupSource) (int32, error) {
	// For workshop simplicity, we'll skip cluster roles
	// In a real implementation, you'd need to import rbacv1 and implement this
	return 0, nil
}

// backupClusterRoleBindings backs up all cluster role bindings
func (r *BackupReconciler) backupClusterRoleBindings(ctx context.Context, minioClient *minio.Client, bucket, backupPath string, source backupv1.BackupSource) (int32, error) {
	// For workshop simplicity, we'll skip cluster role bindings
	// In a real implementation, you'd need to import rbacv1 and implement this
	return 0, nil
}

// backupPersistentVolumes backs up all persistent volumes
func (r *BackupReconciler) backupPersistentVolumes(ctx context.Context, minioClient *minio.Client, bucket, backupPath string, source backupv1.BackupSource) (int32, error) {
	pvs := &corev1.PersistentVolumeList{}

	// Build list options with label selector (no namespace for cluster resources)
	var listOpts []client.ListOption
	if source.LabelSelector != nil {
		selector, err := metav1.LabelSelectorAsSelector(source.LabelSelector)
		if err != nil {
			return 0, fmt.Errorf("failed to convert label selector: %w", err)
		}
		listOpts = append(listOpts, client.MatchingLabelsSelector{Selector: selector})
	}

	if err := r.List(ctx, pvs, listOpts...); err != nil {
		return 0, fmt.Errorf("failed to list persistentvolumes: %w", err)
	}

	count := int32(0)
	for _, pv := range pvs.Items {
		objectName := fmt.Sprintf("%s/cluster/persistentvolumes/%s.json", backupPath, pv.Name)
		if err := r.uploadResource(ctx, minioClient, bucket, objectName, pv); err != nil {
			return 0, fmt.Errorf("failed to backup persistentvolume %s: %w", pv.Name, err)
		}
		count++
	}

	return count, nil
}

// backupStorageClasses backs up all storage classes
func (r *BackupReconciler) backupStorageClasses(ctx context.Context, minioClient *minio.Client, bucket, backupPath string, source backupv1.BackupSource) (int32, error) {
	// For workshop simplicity, we'll skip storage classes
	// In a real implementation, you'd need to import storagev1 and implement this
	return 0, nil
}

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

// SetupWithManager sets up the controller with the Manager.
func (r *BackupReconciler) SetupWithManager(mgr ctrl.Manager) error {
	return ctrl.NewControllerManagedBy(mgr).
		For(&backupv1.Backup{}).
		Named("backup").
		Complete(r)
}
