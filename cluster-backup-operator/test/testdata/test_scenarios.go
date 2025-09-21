package testdata

import (
	"fmt"

	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	networkingv1 "k8s.io/api/networking/v1"
	"k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/util/intstr"

	backupv1 "github.com/rfashwall/cnd-workshop/api/v1"
)

// TestScenario represents a complete test scenario with resources and expected outcomes
type TestScenario struct {
	Name        string
	Description string
	Resources   []TestResource
	Backup      *backupv1.Backup
	Restore     *backupv1.Restore
	Expected    ExpectedOutcome
}

// TestResource represents a Kubernetes resource for testing
type TestResource struct {
	Type         string
	Object       interface{}
	ShouldBackup bool
}

// ExpectedOutcome defines what should happen in a test scenario
type ExpectedOutcome struct {
	BackupPhase       backupv1.BackupPhase
	RestorePhase      backupv1.RestorePhase
	ResourcesBackedUp int
	ResourcesRestored int
	ShouldFail        bool
	ExpectedError     string
}

// GetBasicBackupScenario returns a basic backup scenario with common resources
func GetBasicBackupScenario() TestScenario {
	namespace := "test-basic"

	return TestScenario{
		Name:        "Basic Backup Scenario",
		Description: "Tests backup of basic Kubernetes resources",
		Resources: []TestResource{
			{
				Type:         "deployment",
				Object:       CreateTestDeployment("web-app", namespace),
				ShouldBackup: true,
			},
			{
				Type:         "service",
				Object:       CreateTestService("web-service", namespace),
				ShouldBackup: true,
			},
			{
				Type:         "configmap",
				Object:       CreateTestConfigMap("app-config", namespace),
				ShouldBackup: true,
			},
		},
		Backup: &backupv1.Backup{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "basic-backup",
				Namespace: namespace,
			},
			Spec: backupv1.BackupSpec{
				Source: backupv1.BackupSource{
					Namespace:     namespace,
					ResourceTypes: []string{"deployments", "services", "configmaps"},
				},
				Schedule: "0 2 * * *",
				StorageLocation: backupv1.StorageLocation{
					Provider: "minio",
					Bucket:   "test-bucket",
				},
			},
		},
		Expected: ExpectedOutcome{
			BackupPhase:       backupv1.BackupPhaseScheduled,
			ResourcesBackedUp: 3,
			ShouldFail:        false,
		},
	}
}

// GetLabelSelectorScenario returns a scenario testing label selector functionality
func GetLabelSelectorScenario() TestScenario {
	namespace := "test-labels"

	return TestScenario{
		Name:        "Label Selector Scenario",
		Description: "Tests backup with label selectors",
		Resources: []TestResource{
			{
				Type:         "deployment",
				Object:       CreateTestDeploymentWithLabels("app1", namespace, map[string]string{"backup": "enabled", "tier": "frontend"}),
				ShouldBackup: true,
			},
			{
				Type:         "deployment",
				Object:       CreateTestDeploymentWithLabels("app2", namespace, map[string]string{"tier": "backend"}),
				ShouldBackup: false,
			},
			{
				Type:         "service",
				Object:       CreateTestServiceWithLabels("service1", namespace, map[string]string{"backup": "enabled"}),
				ShouldBackup: true,
			},
		},
		Backup: &backupv1.Backup{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "label-backup",
				Namespace: namespace,
			},
			Spec: backupv1.BackupSpec{
				Source: backupv1.BackupSource{
					Namespace: namespace,
					LabelSelector: &metav1.LabelSelector{
						MatchLabels: map[string]string{"backup": "enabled"},
					},
				},
				Schedule: "0 2 * * *",
				StorageLocation: backupv1.StorageLocation{
					Provider: "minio",
					Bucket:   "label-test-bucket",
				},
			},
		},
		Expected: ExpectedOutcome{
			BackupPhase:       backupv1.BackupPhaseScheduled,
			ResourcesBackedUp: 2, // Only resources with backup=enabled label
			ShouldFail:        false,
		},
	}
}

// GetCrossNamespaceRestoreScenario returns a scenario for cross-namespace restore
func GetCrossNamespaceRestoreScenario() TestScenario {
	sourceNamespace := "source-ns"
	targetNamespace := "target-ns"

	return TestScenario{
		Name:        "Cross-Namespace Restore",
		Description: "Tests restoring resources to a different namespace",
		Resources: []TestResource{
			{
				Type:         "deployment",
				Object:       CreateTestDeployment("web-app", sourceNamespace),
				ShouldBackup: true,
			},
			{
				Type:         "configmap",
				Object:       CreateTestConfigMap("app-config", sourceNamespace),
				ShouldBackup: true,
			},
		},
		Backup: &backupv1.Backup{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "cross-ns-backup",
				Namespace: sourceNamespace,
			},
			Spec: backupv1.BackupSpec{
				Source: backupv1.BackupSource{
					Namespace: sourceNamespace,
				},
				Schedule: "0 2 * * *",
				StorageLocation: backupv1.StorageLocation{
					Provider: "minio",
					Bucket:   "cross-ns-bucket",
				},
			},
		},
		Restore: &backupv1.Restore{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "cross-ns-restore",
				Namespace: targetNamespace,
			},
			Spec: backupv1.RestoreSpec{
				Source: backupv1.RestoreSource{
					BackupPath: "backups/cross-ns-backup",
					StorageLocation: backupv1.StorageLocation{
						Provider: "minio",
						Bucket:   "cross-ns-bucket",
					},
				},
				Target: backupv1.RestoreTarget{
					Namespaces: []string{targetNamespace},
				},
			},
		},
		Expected: ExpectedOutcome{
			BackupPhase:       backupv1.BackupPhaseScheduled,
			RestorePhase:      backupv1.RestorePhaseCompleted,
			ResourcesBackedUp: 2,
			ResourcesRestored: 2,
			ShouldFail:        false,
		},
	}
}

// GetInvalidScheduleScenario returns a scenario with invalid cron schedule
func GetInvalidScheduleScenario() TestScenario {
	namespace := "test-invalid"

	return TestScenario{
		Name:        "Invalid Schedule Scenario",
		Description: "Tests handling of invalid cron schedules",
		Resources:   []TestResource{},
		Backup: &backupv1.Backup{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "invalid-backup",
				Namespace: namespace,
			},
			Spec: backupv1.BackupSpec{
				Source: backupv1.BackupSource{
					Namespace: namespace,
				},
				Schedule: "invalid-cron-expression",
				StorageLocation: backupv1.StorageLocation{
					Provider: "minio",
					Bucket:   "test-bucket",
				},
			},
		},
		Expected: ExpectedOutcome{
			BackupPhase:   backupv1.BackupPhaseFailed,
			ShouldFail:    true,
			ExpectedError: "invalid schedule",
		},
	}
}

// GetLargeResourceScenario returns a scenario with many resources for performance testing
func GetLargeResourceScenario() TestScenario {
	namespace := "test-large"
	resources := make([]TestResource, 0, 50)

	// Create 20 deployments
	for i := 0; i < 20; i++ {
		resources = append(resources, TestResource{
			Type:         "deployment",
			Object:       CreateTestDeployment(fmt.Sprintf("app-%d", i), namespace),
			ShouldBackup: true,
		})
	}

	// Create 15 services
	for i := 0; i < 15; i++ {
		resources = append(resources, TestResource{
			Type:         "service",
			Object:       CreateTestService(fmt.Sprintf("service-%d", i), namespace),
			ShouldBackup: true,
		})
	}

	// Create 15 configmaps
	for i := 0; i < 15; i++ {
		resources = append(resources, TestResource{
			Type:         "configmap",
			Object:       CreateTestConfigMap(fmt.Sprintf("config-%d", i), namespace),
			ShouldBackup: true,
		})
	}

	return TestScenario{
		Name:        "Large Resource Set",
		Description: "Tests backup performance with many resources",
		Resources:   resources,
		Backup: &backupv1.Backup{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "large-backup",
				Namespace: namespace,
			},
			Spec: backupv1.BackupSpec{
				Source: backupv1.BackupSource{
					Namespace: namespace,
				},
				Schedule: "0 2 * * *",
				StorageLocation: backupv1.StorageLocation{
					Provider: "minio",
					Bucket:   "large-test-bucket",
				},
			},
		},
		Expected: ExpectedOutcome{
			BackupPhase:       backupv1.BackupPhaseScheduled,
			ResourcesBackedUp: 50,
			ShouldFail:        false,
		},
	}
}

// GetSecretsAndConfigMapsScenario returns a scenario focusing on sensitive data
func GetSecretsAndConfigMapsScenario() TestScenario {
	namespace := "test-secrets"

	return TestScenario{
		Name:        "Secrets and ConfigMaps",
		Description: "Tests backup of sensitive configuration data",
		Resources: []TestResource{
			{
				Type:         "secret",
				Object:       CreateTestSecret("db-credentials", namespace),
				ShouldBackup: true,
			},
			{
				Type:         "secret",
				Object:       CreateTestTLSSecret("tls-cert", namespace),
				ShouldBackup: true,
			},
			{
				Type:         "configmap",
				Object:       CreateTestConfigMapWithBinaryData("app-config", namespace),
				ShouldBackup: true,
			},
		},
		Backup: &backupv1.Backup{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "secrets-backup",
				Namespace: namespace,
			},
			Spec: backupv1.BackupSpec{
				Source: backupv1.BackupSource{
					Namespace:     namespace,
					ResourceTypes: []string{"secrets", "configmaps"},
				},
				Schedule: "0 2 * * *",
				StorageLocation: backupv1.StorageLocation{
					Provider: "minio",
					Bucket:   "secrets-bucket",
				},
			},
		},
		Expected: ExpectedOutcome{
			BackupPhase:       backupv1.BackupPhaseScheduled,
			ResourcesBackedUp: 3,
			ShouldFail:        false,
		},
	}
}

// GetFrequentBackupScenario returns a scenario with frequent backup schedule
func GetFrequentBackupScenario() TestScenario {
	namespace := "test-frequent"

	return TestScenario{
		Name:        "Frequent Backup Schedule",
		Description: "Tests backup with frequent scheduling (every minute)",
		Resources: []TestResource{
			{
				Type:         "deployment",
				Object:       CreateTestDeployment("frequent-app", namespace),
				ShouldBackup: true,
			},
		},
		Backup: &backupv1.Backup{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "frequent-backup",
				Namespace: namespace,
			},
			Spec: backupv1.BackupSpec{
				Source: backupv1.BackupSource{
					Namespace: namespace,
				},
				Schedule: "* * * * *", // Every minute
				StorageLocation: backupv1.StorageLocation{
					Provider: "minio",
					Bucket:   "frequent-bucket",
				},
			},
		},
		Expected: ExpectedOutcome{
			BackupPhase:       backupv1.BackupPhaseScheduled,
			ResourcesBackedUp: 1,
			ShouldFail:        false,
		},
	}
}

// Helper functions to create test resources

func CreateTestDeployment(name, namespace string) *appsv1.Deployment {
	return CreateTestDeploymentWithLabels(name, namespace, map[string]string{"app": name})
}

func CreateTestDeploymentWithLabels(name, namespace string, labels map[string]string) *appsv1.Deployment {
	return &appsv1.Deployment{
		ObjectMeta: metav1.ObjectMeta{
			Name:      name,
			Namespace: namespace,
			Labels:    labels,
		},
		Spec: appsv1.DeploymentSpec{
			Replicas: int32Ptr(2),
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
						Image: "nginx:1.20",
						Ports: []corev1.ContainerPort{{
							ContainerPort: 80,
						}},
						Resources: corev1.ResourceRequirements{
							Requests: corev1.ResourceList{
								corev1.ResourceCPU:    resource.MustParse("100m"),
								corev1.ResourceMemory: resource.MustParse("128Mi"),
							},
							Limits: corev1.ResourceList{
								corev1.ResourceCPU:    resource.MustParse("500m"),
								corev1.ResourceMemory: resource.MustParse("512Mi"),
							},
						},
					}},
				},
			},
		},
	}
}

func CreateTestService(name, namespace string) *corev1.Service {
	return CreateTestServiceWithLabels(name, namespace, map[string]string{"app": name})
}

func CreateTestServiceWithLabels(name, namespace string, labels map[string]string) *corev1.Service {
	return &corev1.Service{
		ObjectMeta: metav1.ObjectMeta{
			Name:      name,
			Namespace: namespace,
			Labels:    labels,
		},
		Spec: corev1.ServiceSpec{
			Selector: map[string]string{"app": name},
			Ports: []corev1.ServicePort{{
				Port:       80,
				TargetPort: intstr.FromInt(80),
				Protocol:   corev1.ProtocolTCP,
			}},
			Type: corev1.ServiceTypeClusterIP,
		},
	}
}

func CreateTestConfigMap(name, namespace string) *corev1.ConfigMap {
	return &corev1.ConfigMap{
		ObjectMeta: metav1.ObjectMeta{
			Name:      name,
			Namespace: namespace,
		},
		Data: map[string]string{
			"config.yaml": `
server:
  port: 8080
  host: 0.0.0.0
database:
  host: db.example.com
  port: 5432
  name: myapp
logging:
  level: info
  format: json
`,
			"app.properties": `
debug=false
max.connections=100
timeout=30s
`,
		},
	}
}

func CreateTestConfigMapWithBinaryData(name, namespace string) *corev1.ConfigMap {
	return &corev1.ConfigMap{
		ObjectMeta: metav1.ObjectMeta{
			Name:      name,
			Namespace: namespace,
		},
		Data: map[string]string{
			"config.yaml": "server:\n  port: 8080",
		},
		BinaryData: map[string][]byte{
			"binary.dat": {0x89, 0x50, 0x4E, 0x47, 0x0D, 0x0A, 0x1A, 0x0A}, // PNG header
		},
	}
}

func CreateTestSecret(name, namespace string) *corev1.Secret {
	return &corev1.Secret{
		ObjectMeta: metav1.ObjectMeta{
			Name:      name,
			Namespace: namespace,
		},
		Type: corev1.SecretTypeOpaque,
		Data: map[string][]byte{
			"username": []byte("admin"),
			"password": []byte("super-secret-password"),
			"api-key":  []byte("abcd1234-5678-90ef-ghij-klmnopqrstuv"),
		},
	}
}

func CreateTestTLSSecret(name, namespace string) *corev1.Secret {
	return &corev1.Secret{
		ObjectMeta: metav1.ObjectMeta{
			Name:      name,
			Namespace: namespace,
		},
		Type: corev1.SecretTypeTLS,
		Data: map[string][]byte{
			"tls.crt": []byte("-----BEGIN CERTIFICATE-----\nMIIC...fake cert...\n-----END CERTIFICATE-----"),
			"tls.key": []byte("-----BEGIN PRIVATE KEY-----\nMIIE...fake key...\n-----END PRIVATE KEY-----"),
		},
	}
}

func CreateTestIngress(name, namespace string) *networkingv1.Ingress {
	pathType := networkingv1.PathTypePrefix
	return &networkingv1.Ingress{
		ObjectMeta: metav1.ObjectMeta{
			Name:      name,
			Namespace: namespace,
		},
		Spec: networkingv1.IngressSpec{
			Rules: []networkingv1.IngressRule{{
				Host: "example.com",
				IngressRuleValue: networkingv1.IngressRuleValue{
					HTTP: &networkingv1.HTTPIngressRuleValue{
						Paths: []networkingv1.HTTPIngressPath{{
							Path:     "/",
							PathType: &pathType,
							Backend: networkingv1.IngressBackend{
								Service: &networkingv1.IngressServiceBackend{
									Name: name,
									Port: networkingv1.ServiceBackendPort{
										Number: 80,
									},
								},
							},
						}},
					},
				},
			}},
		},
	}
}

func int32Ptr(i int32) *int32 {
	return &i
}

// GetAllTestScenarios returns all available test scenarios
func GetAllTestScenarios() []TestScenario {
	return []TestScenario{
		GetBasicBackupScenario(),
		GetLabelSelectorScenario(),
		GetCrossNamespaceRestoreScenario(),
		GetInvalidScheduleScenario(),
		GetLargeResourceScenario(),
		GetSecretsAndConfigMapsScenario(),
		GetFrequentBackupScenario(),
	}
}
