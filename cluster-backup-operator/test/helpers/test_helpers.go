package helpers

import (
	"context"
	"fmt"
	"time"

	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/util/rand"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"

	backupv1 "github.com/rfashwall/cnd-workshop/api/v1"
)

// TestHelper provides utilities for testing backup and restore controllers
type TestHelper struct {
	Client client.Client
	Scheme *runtime.Scheme
}

// NewTestHelper creates a new test helper with a fake Kubernetes client
func NewTestHelper(scheme *runtime.Scheme, objects ...client.Object) *TestHelper {
	fakeClient := fake.NewClientBuilder().
		WithScheme(scheme).
		WithObjects(objects...).
		Build()

	return &TestHelper{
		Client: fakeClient,
		Scheme: scheme,
	}
}

// CreateTestNamespace creates a test namespace with a random name
func (h *TestHelper) CreateTestNamespace(ctx context.Context, prefix string) (*corev1.Namespace, error) {
	ns := &corev1.Namespace{
		ObjectMeta: metav1.ObjectMeta{
			Name: fmt.Sprintf("%s-%s", prefix, rand.String(5)),
		},
	}

	err := h.Client.Create(ctx, ns)
	return ns, err
}

// CreateTestDeployment creates a test deployment with specified parameters
func (h *TestHelper) CreateTestDeployment(ctx context.Context, name, namespace string, labels map[string]string) (*appsv1.Deployment, error) {
	if labels == nil {
		labels = make(map[string]string)
	}
	labels["app"] = name

	deployment := &appsv1.Deployment{
		ObjectMeta: metav1.ObjectMeta{
			Name:      name,
			Namespace: namespace,
			Labels:    labels,
		},
		Spec: appsv1.DeploymentSpec{
			Replicas: int32Ptr(1),
			Selector: &metav1.LabelSelector{
				MatchLabels: map[string]string{"app": name},
			},
			Template: corev1.PodTemplateSpec{
				ObjectMeta: metav1.ObjectMeta{
					Labels: map[string]string{"app": name},
				},
				Spec: corev1.PodSpec{
					Containers: []corev1.Container{{
						Name:  "app",
						Image: "nginx:latest",
						Ports: []corev1.ContainerPort{{
							ContainerPort: 80,
						}},
					}},
				},
			},
		},
	}

	err := h.Client.Create(ctx, deployment)
	return deployment, err
}

// CreateTestService creates a test service
func (h *TestHelper) CreateTestService(ctx context.Context, name, namespace string, labels map[string]string) (*corev1.Service, error) {
	if labels == nil {
		labels = make(map[string]string)
	}

	service := &corev1.Service{
		ObjectMeta: metav1.ObjectMeta{
			Name:      name,
			Namespace: namespace,
			Labels:    labels,
		},
		Spec: corev1.ServiceSpec{
			Selector: map[string]string{"app": name},
			Ports: []corev1.ServicePort{{
				Port:     80,
				Protocol: corev1.ProtocolTCP,
			}},
		},
	}

	err := h.Client.Create(ctx, service)
	return service, err
}

// CreateTestConfigMap creates a test ConfigMap
func (h *TestHelper) CreateTestConfigMap(ctx context.Context, name, namespace string, data map[string]string, labels map[string]string) (*corev1.ConfigMap, error) {
	if data == nil {
		data = map[string]string{
			"config.yaml": "test: configuration",
		}
	}

	configMap := &corev1.ConfigMap{
		ObjectMeta: metav1.ObjectMeta{
			Name:      name,
			Namespace: namespace,
			Labels:    labels,
		},
		Data: data,
	}

	err := h.Client.Create(ctx, configMap)
	return configMap, err
}

// CreateTestSecret creates a test Secret
func (h *TestHelper) CreateTestSecret(ctx context.Context, name, namespace string, data map[string][]byte, labels map[string]string) (*corev1.Secret, error) {
	if data == nil {
		data = map[string][]byte{
			"username": []byte("testuser"),
			"password": []byte("testpass"),
		}
	}

	secret := &corev1.Secret{
		ObjectMeta: metav1.ObjectMeta{
			Name:      name,
			Namespace: namespace,
			Labels:    labels,
		},
		Data: data,
		Type: corev1.SecretTypeOpaque,
	}

	err := h.Client.Create(ctx, secret)
	return secret, err
}

// CreateTestBackup creates a test backup resource
func (h *TestHelper) CreateTestBackup(ctx context.Context, name, namespace string, spec backupv1.BackupSpec) (*backupv1.Backup, error) {
	backup := &backupv1.Backup{
		ObjectMeta: metav1.ObjectMeta{
			Name:      name,
			Namespace: namespace,
		},
		Spec: spec,
	}

	err := h.Client.Create(ctx, backup)
	return backup, err
}

// CreateTestRestore creates a test restore resource
func (h *TestHelper) CreateTestRestore(ctx context.Context, name, namespace string, spec backupv1.RestoreSpec) (*backupv1.Restore, error) {
	restore := &backupv1.Restore{
		ObjectMeta: metav1.ObjectMeta{
			Name:      name,
			Namespace: namespace,
		},
		Spec: spec,
	}

	err := h.Client.Create(ctx, restore)
	return restore, err
}

// WaitForBackupPhase waits for a backup to reach a specific phase
func (h *TestHelper) WaitForBackupPhase(ctx context.Context, backup *backupv1.Backup, expectedPhase backupv1.BackupPhase, timeout time.Duration) error {
	deadline := time.Now().Add(timeout)

	for time.Now().Before(deadline) {
		updated := &backupv1.Backup{}
		err := h.Client.Get(ctx, client.ObjectKeyFromObject(backup), updated)
		if err != nil {
			return err
		}

		if updated.Status.Phase == expectedPhase {
			return nil
		}

		time.Sleep(100 * time.Millisecond)
	}

	return fmt.Errorf("timeout waiting for backup %s to reach phase %s", backup.Name, expectedPhase)
}

// WaitForRestorePhase waits for a restore to reach a specific phase
func (h *TestHelper) WaitForRestorePhase(ctx context.Context, restore *backupv1.Restore, expectedPhase backupv1.RestorePhase, timeout time.Duration) error {
	deadline := time.Now().Add(timeout)

	for time.Now().Before(deadline) {
		updated := &backupv1.Restore{}
		err := h.Client.Get(ctx, client.ObjectKeyFromObject(restore), updated)
		if err != nil {
			return err
		}

		if updated.Status.Phase == expectedPhase {
			return nil
		}

		time.Sleep(100 * time.Millisecond)
	}

	return fmt.Errorf("timeout waiting for restore %s to reach phase %s", restore.Name, expectedPhase)
}

// GetResourcesInNamespace returns all resources of specified types in a namespace
func (h *TestHelper) GetResourcesInNamespace(ctx context.Context, namespace string, resourceTypes []string) (map[string][]client.Object, error) {
	resources := make(map[string][]client.Object)

	for _, resourceType := range resourceTypes {
		var list client.ObjectList

		switch resourceType {
		case "deployments":
			list = &appsv1.DeploymentList{}
		case "services":
			list = &corev1.ServiceList{}
		case "configmaps":
			list = &corev1.ConfigMapList{}
		case "secrets":
			list = &corev1.SecretList{}
		default:
			continue
		}

		err := h.Client.List(ctx, list, client.InNamespace(namespace))
		if err != nil {
			return nil, err
		}

		// Extract items from the list
		switch typedList := list.(type) {
		case *appsv1.DeploymentList:
			for i := range typedList.Items {
				resources[resourceType] = append(resources[resourceType], &typedList.Items[i])
			}
		case *corev1.ServiceList:
			for i := range typedList.Items {
				resources[resourceType] = append(resources[resourceType], &typedList.Items[i])
			}
		case *corev1.ConfigMapList:
			for i := range typedList.Items {
				resources[resourceType] = append(resources[resourceType], &typedList.Items[i])
			}
		case *corev1.SecretList:
			for i := range typedList.Items {
				resources[resourceType] = append(resources[resourceType], &typedList.Items[i])
			}
		}
	}

	return resources, nil
}

// CleanupNamespace deletes a namespace and all its resources
func (h *TestHelper) CleanupNamespace(ctx context.Context, namespace string) error {
	ns := &corev1.Namespace{
		ObjectMeta: metav1.ObjectMeta{Name: namespace},
	}
	return h.Client.Delete(ctx, ns)
}

// CreateCompleteTestEnvironment creates a full test environment with multiple resources
func (h *TestHelper) CreateCompleteTestEnvironment(ctx context.Context, namespace string) error {
	// Create namespace
	ns := &corev1.Namespace{
		ObjectMeta: metav1.ObjectMeta{Name: namespace},
	}
	if err := h.Client.Create(ctx, ns); err != nil {
		return err
	}

	// Create test resources with backup labels
	labels := map[string]string{"backup": "enabled"}

	_, err := h.CreateTestDeployment(ctx, "web-app", namespace, labels)
	if err != nil {
		return err
	}

	_, err = h.CreateTestService(ctx, "web-service", namespace, labels)
	if err != nil {
		return err
	}

	_, err = h.CreateTestConfigMap(ctx, "app-config", namespace, nil, labels)
	if err != nil {
		return err
	}

	_, err = h.CreateTestSecret(ctx, "app-secret", namespace, nil, labels)
	if err != nil {
		return err
	}

	return nil
}

// ValidateResourcesRestored checks if resources were properly restored to target namespace
func (h *TestHelper) ValidateResourcesRestored(ctx context.Context, sourceNamespace, targetNamespace string, resourceTypes []string) error {
	sourceResources, err := h.GetResourcesInNamespace(ctx, sourceNamespace, resourceTypes)
	if err != nil {
		return fmt.Errorf("failed to get source resources: %w", err)
	}

	targetResources, err := h.GetResourcesInNamespace(ctx, targetNamespace, resourceTypes)
	if err != nil {
		return fmt.Errorf("failed to get target resources: %w", err)
	}

	for resourceType, sourceList := range sourceResources {
		targetList, exists := targetResources[resourceType]
		if !exists {
			return fmt.Errorf("resource type %s not found in target namespace", resourceType)
		}

		if len(sourceList) != len(targetList) {
			return fmt.Errorf("resource count mismatch for %s: source=%d, target=%d",
				resourceType, len(sourceList), len(targetList))
		}
	}

	return nil
}

// Utility functions
func int32Ptr(i int32) *int32 {
	return &i
}

// DefaultBackupSpec returns a default backup specification for testing
func DefaultBackupSpec(namespace string) backupv1.BackupSpec {
	return backupv1.BackupSpec{
		Source: backupv1.BackupSource{
			Namespace:     namespace,
			ResourceTypes: []string{"deployments", "services", "configmaps"},
		},
		Schedule: "0 2 * * *",
		StorageLocation: backupv1.StorageLocation{
			Provider:  "minio",
			Bucket:    "test-bucket",
			Endpoint:  "http://localhost:9000",
			AccessKey: "minioadmin",
			SecretKey: "minioadmin123",
		},
	}
}

// DefaultRestoreSpec returns a default restore specification for testing
func DefaultRestoreSpec(backupName, targetNamespace string) backupv1.RestoreSpec {
	return backupv1.RestoreSpec{
		Source: backupv1.RestoreSource{
			BackupPath: fmt.Sprintf("backups/%s", backupName),
			StorageLocation: backupv1.StorageLocation{
				Provider:  "minio",
				Bucket:    "test-bucket",
				Endpoint:  "http://localhost:9000",
				AccessKey: "minioadmin",
				SecretKey: "minioadmin123",
			},
		},
		Target: backupv1.RestoreTarget{
			Namespaces: []string{targetNamespace},
		},
	}
}
