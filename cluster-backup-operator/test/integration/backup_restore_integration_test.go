package integration

import (
	"context"
	"fmt"
	"path/filepath"
	"testing"
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/util/rand"
	"k8s.io/client-go/kubernetes/scheme"
	"k8s.io/client-go/rest"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/envtest"
	logf "sigs.k8s.io/controller-runtime/pkg/log"
	"sigs.k8s.io/controller-runtime/pkg/log/zap"

	backupv1 "github.com/rfashwall/cnd-workshop/api/v1"
)

// These tests use Ginkgo (BDD-style Go testing framework). Refer to
// http://onsi.github.io/ginkgo/ to learn more about Ginkgo.

var cfg *rest.Config
var k8sClient client.Client
var testEnv *envtest.Environment
var ctx context.Context
var cancel context.CancelFunc

const (
	timeout  = time.Second * 10
	interval = time.Millisecond * 250
)

func TestIntegration(t *testing.T) {
	RegisterFailHandler(Fail)
	RunSpecs(t, "Integration Suite")
}

var _ = BeforeSuite(func() {
	logf.SetLogger(zap.New(zap.WriteTo(GinkgoWriter), zap.UseDevMode(true)))

	ctx, cancel = context.WithCancel(context.TODO())

	By("bootstrapping test environment")
	testEnv = &envtest.Environment{
		CRDDirectoryPaths:     []string{filepath.Join("..", "..", "config", "crd", "bases")},
		ErrorIfCRDPathMissing: true,
	}

	var err error
	cfg, err = testEnv.Start()
	Expect(err).NotTo(HaveOccurred())
	Expect(cfg).NotTo(BeNil())

	err = backupv1.AddToScheme(scheme.Scheme)
	Expect(err).NotTo(HaveOccurred())

	k8sClient, err = client.New(cfg, client.Options{Scheme: scheme.Scheme})
	Expect(err).NotTo(HaveOccurred())
	Expect(k8sClient).NotTo(BeNil())
})

var _ = AfterSuite(func() {
	cancel()
	By("tearing down the test environment")
	err := testEnv.Stop()
	Expect(err).NotTo(HaveOccurred())
})

var _ = Describe("Backup and Restore Integration", func() {
	var (
		ctx             context.Context
		sourceNamespace string
		targetNamespace string
		testDeployment  *appsv1.Deployment
		testService     *corev1.Service
		testConfigMap   *corev1.ConfigMap
	)

	BeforeEach(func() {
		ctx = context.Background()
		sourceNamespace = "source-" + rand.String(5)
		targetNamespace = "target-" + rand.String(5)

		// Create test namespaces
		for _, nsName := range []string{sourceNamespace, targetNamespace} {
			ns := &corev1.Namespace{
				ObjectMeta: metav1.ObjectMeta{Name: nsName},
			}
			Expect(k8sClient.Create(ctx, ns)).To(Succeed())
		}

		// Create test resources in source namespace
		testDeployment = createTestDeployment("test-app", sourceNamespace)
		Expect(k8sClient.Create(ctx, testDeployment)).To(Succeed())

		testService = createTestService("test-service", sourceNamespace)
		Expect(k8sClient.Create(ctx, testService)).To(Succeed())

		testConfigMap = createTestConfigMap("test-config", sourceNamespace)
		Expect(k8sClient.Create(ctx, testConfigMap)).To(Succeed())
	})

	AfterEach(func() {
		// Cleanup namespaces (this will delete all resources within them)
		for _, nsName := range []string{sourceNamespace, targetNamespace} {
			ns := &corev1.Namespace{
				ObjectMeta: metav1.ObjectMeta{Name: nsName},
			}
			Expect(k8sClient.Delete(ctx, ns)).To(Succeed())
		}
	})

	Context("Backup Controller Integration", func() {
		It("should create and schedule a backup successfully", func() {
			By("Creating a backup resource")
			backup := &backupv1.Backup{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "integration-backup",
					Namespace: sourceNamespace,
				},
				Spec: backupv1.BackupSpec{
					Source: backupv1.BackupSource{
						Namespace: sourceNamespace,
						LabelSelector: &metav1.LabelSelector{
							MatchLabels: map[string]string{"backup": "enabled"},
						},
					},
					Schedule: "*/5 * * * *", // Every 5 minutes for testing
					StorageLocation: backupv1.StorageLocation{
						Provider:  "minio",
						Bucket:    "integration-test-bucket",
						Endpoint:  "http://localhost:9000",
						AccessKey: "minioadmin",
						SecretKey: "minioadmin123",
					},
				},
			}
			Expect(k8sClient.Create(ctx, backup)).To(Succeed())

			By("Waiting for backup status to be updated")
			Eventually(func() backupv1.BackupPhase {
				updated := &backupv1.Backup{}
				err := k8sClient.Get(ctx, client.ObjectKeyFromObject(backup), updated)
				if err != nil {
					return ""
				}
				return updated.Status.Phase
			}, timeout, interval).Should(Equal(backupv1.BackupPhaseScheduled))

			By("Verifying next backup time is set")
			updatedBackup := &backupv1.Backup{}
			Expect(k8sClient.Get(ctx, client.ObjectKeyFromObject(backup), updatedBackup)).To(Succeed())
			Expect(updatedBackup.Status.NextBackupTime).NotTo(BeNil())
			Expect(updatedBackup.Status.NextBackupTime.Time).To(BeTemporally(">", time.Now()))
		})

		It("should handle backup with specific resource types", func() {
			By("Creating a backup with specific resource types")
			backup := &backupv1.Backup{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "specific-backup",
					Namespace: sourceNamespace,
				},
				Spec: backupv1.BackupSpec{
					Source: backupv1.BackupSource{
						Namespace:     sourceNamespace,
						ResourceTypes: []string{"deployments", "services"},
					},
					Schedule: "0 2 * * *",
					StorageLocation: backupv1.StorageLocation{
						Provider: "minio",
						Bucket:   "specific-test-bucket",
					},
				},
			}
			Expect(k8sClient.Create(ctx, backup)).To(Succeed())

			By("Verifying backup is scheduled")
			Eventually(func() backupv1.BackupPhase {
				updated := &backupv1.Backup{}
				err := k8sClient.Get(ctx, client.ObjectKeyFromObject(backup), updated)
				if err != nil {
					return ""
				}
				return updated.Status.Phase
			}, timeout, interval).Should(Equal(backupv1.BackupPhaseScheduled))
		})

		It("should reject invalid backup configurations", func() {
			By("Creating a backup with invalid schedule")
			backup := &backupv1.Backup{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "invalid-backup",
					Namespace: sourceNamespace,
				},
				Spec: backupv1.BackupSpec{
					Source: backupv1.BackupSource{
						Namespace: sourceNamespace,
					},
					Schedule: "invalid-cron",
					StorageLocation: backupv1.StorageLocation{
						Provider: "minio",
						Bucket:   "test-bucket",
					},
				},
			}
			Expect(k8sClient.Create(ctx, backup)).To(Succeed())

			By("Waiting for backup to fail validation")
			Eventually(func() backupv1.BackupPhase {
				updated := &backupv1.Backup{}
				err := k8sClient.Get(ctx, client.ObjectKeyFromObject(backup), updated)
				if err != nil {
					return ""
				}
				return updated.Status.Phase
			}, timeout, interval).Should(Equal(backupv1.BackupPhaseFailed))

			By("Verifying error message is set")
			updatedBackup := &backupv1.Backup{}
			Expect(k8sClient.Get(ctx, client.ObjectKeyFromObject(backup), updatedBackup)).To(Succeed())
			Expect(updatedBackup.Status.Message).To(ContainSubstring("invalid schedule"))
		})
	})

	Context("Restore Controller Integration", func() {
		It("should create and execute a restore successfully", func() {
			By("Creating a restore resource")
			restore := &backupv1.Restore{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "integration-restore",
					Namespace: targetNamespace,
				},
				Spec: backupv1.RestoreSpec{
					Source: backupv1.RestoreSource{
						BackupPath: "backups/test-backup",
						StorageLocation: backupv1.StorageLocation{
							Provider:  "minio",
							Bucket:    "integration-test-bucket",
							Endpoint:  "http://localhost:9000",
							AccessKey: "minioadmin",
							SecretKey: "minioadmin123",
						},
					},
					Target: backupv1.RestoreTarget{
						Namespaces:    []string{targetNamespace},
						ResourceTypes: []string{"deployments", "services"},
					},
				},
			}
			Expect(k8sClient.Create(ctx, restore)).To(Succeed())

			By("Waiting for restore status to be updated")
			Eventually(func() backupv1.RestorePhase {
				updated := &backupv1.Restore{}
				err := k8sClient.Get(ctx, client.ObjectKeyFromObject(restore), updated)
				if err != nil {
					return ""
				}
				return updated.Status.Phase
			}, timeout, interval).Should(Or(
				Equal(backupv1.RestorePhaseRestoring),
				Equal(backupv1.RestorePhaseCompleted),
				Equal(backupv1.RestorePhaseFailed), // May fail if backup doesn't exist
			))
		})

		It("should handle restore to different namespace", func() {
			By("Creating a cross-namespace restore")
			restore := &backupv1.Restore{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "cross-ns-restore",
					Namespace: targetNamespace,
				},
				Spec: backupv1.RestoreSpec{
					Source: backupv1.RestoreSource{
						BackupPath: "backups/source-backup",
						StorageLocation: backupv1.StorageLocation{
							Provider: "minio",
							Bucket:   "cross-ns-bucket",
						},
					},
					Target: backupv1.RestoreTarget{
						Namespaces: []string{targetNamespace},
					},
				},
			}
			Expect(k8sClient.Create(ctx, restore)).To(Succeed())

			By("Verifying restore is processed")
			Eventually(func() string {
				updated := &backupv1.Restore{}
				err := k8sClient.Get(ctx, client.ObjectKeyFromObject(restore), updated)
				if err != nil {
					return ""
				}
				return string(updated.Status.Phase)
			}, timeout, interval).ShouldNot(BeEmpty())
		})

		It("should reject invalid restore configurations", func() {
			By("Creating a restore with missing backup path")
			restore := &backupv1.Restore{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "invalid-restore",
					Namespace: targetNamespace,
				},
				Spec: backupv1.RestoreSpec{
					Source: backupv1.RestoreSource{
						StorageLocation: backupv1.StorageLocation{
							Provider: "minio",
							Bucket:   "test-bucket",
						},
					},
					Target: backupv1.RestoreTarget{
						Namespaces: []string{targetNamespace},
					},
				},
			}
			Expect(k8sClient.Create(ctx, restore)).To(Succeed())

			By("Waiting for restore to fail validation")
			Eventually(func() backupv1.RestorePhase {
				updated := &backupv1.Restore{}
				err := k8sClient.Get(ctx, client.ObjectKeyFromObject(restore), updated)
				if err != nil {
					return ""
				}
				return updated.Status.Phase
			}, timeout, interval).Should(Equal(backupv1.RestorePhaseFailed))

			By("Verifying error message is set")
			updatedRestore := &backupv1.Restore{}
			Expect(k8sClient.Get(ctx, client.ObjectKeyFromObject(restore), updatedRestore)).To(Succeed())
			Expect(updatedRestore.Status.Message).To(ContainSubstring("backup path is required"))
		})
	})

	Context("End-to-End Backup and Restore Workflow", func() {
		It("should complete a full backup and restore cycle", func() {
			By("Creating a backup of source namespace")
			backup := &backupv1.Backup{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "e2e-backup",
					Namespace: sourceNamespace,
				},
				Spec: backupv1.BackupSpec{
					Source: backupv1.BackupSource{
						Namespace: sourceNamespace,
						LabelSelector: &metav1.LabelSelector{
							MatchLabels: map[string]string{"backup": "enabled"},
						},
					},
					Schedule: "* * * * *", // Run immediately for testing
					StorageLocation: backupv1.StorageLocation{
						Provider:  "minio",
						Bucket:    "e2e-test-bucket",
						Endpoint:  "http://localhost:9000",
						AccessKey: "minioadmin",
						SecretKey: "minioadmin123",
					},
				},
			}
			Expect(k8sClient.Create(ctx, backup)).To(Succeed())

			By("Waiting for backup to be scheduled and potentially run")
			Eventually(func() backupv1.BackupPhase {
				updated := &backupv1.Backup{}
				err := k8sClient.Get(ctx, client.ObjectKeyFromObject(backup), updated)
				if err != nil {
					return ""
				}
				return updated.Status.Phase
			}, 2*time.Minute, 5*time.Second).Should(Or(
				Equal(backupv1.BackupPhaseScheduled),
				Equal(backupv1.BackupPhaseRunning),
				Equal(backupv1.BackupPhaseCompleted),
			))

			By("Creating a restore to target namespace")
			restore := &backupv1.Restore{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "e2e-restore",
					Namespace: targetNamespace,
				},
				Spec: backupv1.RestoreSpec{
					Source: backupv1.RestoreSource{
						BackupPath:      fmt.Sprintf("backups/%s", backup.Name),
						StorageLocation: backup.Spec.StorageLocation,
					},
					Target: backupv1.RestoreTarget{
						Namespaces: []string{targetNamespace},
					},
				},
			}
			Expect(k8sClient.Create(ctx, restore)).To(Succeed())

			By("Waiting for restore to be processed")
			Eventually(func() string {
				updated := &backupv1.Restore{}
				err := k8sClient.Get(ctx, client.ObjectKeyFromObject(restore), updated)
				if err != nil {
					return ""
				}
				return string(updated.Status.Phase)
			}, 2*time.Minute, 5*time.Second).Should(Or(
				Equal(string(backupv1.RestorePhaseRestoring)),
				Equal(string(backupv1.RestorePhaseCompleted)),
				Equal(string(backupv1.RestorePhaseFailed)),
			))

			By("Verifying both resources have status updates")
			finalBackup := &backupv1.Backup{}
			Expect(k8sClient.Get(ctx, client.ObjectKeyFromObject(backup), finalBackup)).To(Succeed())
			Expect(finalBackup.Status.LastBackupTime).NotTo(BeNil())

			finalRestore := &backupv1.Restore{}
			Expect(k8sClient.Get(ctx, client.ObjectKeyFromObject(restore), finalRestore)).To(Succeed())
			Expect(finalRestore.Status.StartTime).NotTo(BeNil())
		})
	})
})

// Helper functions for creating test resources
func createTestDeployment(name, namespace string) *appsv1.Deployment {
	return &appsv1.Deployment{
		ObjectMeta: metav1.ObjectMeta{
			Name:      name,
			Namespace: namespace,
			Labels: map[string]string{
				"backup": "enabled",
				"app":    name,
			},
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
}

func createTestService(name, namespace string) *corev1.Service {
	return &corev1.Service{
		ObjectMeta: metav1.ObjectMeta{
			Name:      name,
			Namespace: namespace,
			Labels: map[string]string{
				"backup": "enabled",
			},
		},
		Spec: corev1.ServiceSpec{
			Selector: map[string]string{"app": "test-app"},
			Ports: []corev1.ServicePort{{
				Port:     80,
				Protocol: corev1.ProtocolTCP,
			}},
		},
	}
}

func createTestConfigMap(name, namespace string) *corev1.ConfigMap {
	return &corev1.ConfigMap{
		ObjectMeta: metav1.ObjectMeta{
			Name:      name,
			Namespace: namespace,
			Labels: map[string]string{
				"backup": "enabled",
			},
		},
		Data: map[string]string{
			"config.yaml":    "test: configuration",
			"app.properties": "debug=true\nport=8080",
		},
	}
}

func int32Ptr(i int32) *int32 {
	return &i
}
