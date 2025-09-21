## Overview

Testing is a critical aspect of building reliable Kubernetes Operators. This stage covers comprehensive testing strategies including unit tests, integration tests, and end-to-end testing for our backup and restore controllers. We'll explore testing patterns specific to Kubernetes controllers and learn how to validate operator behavior in realistic scenarios.

## Learning Objectives

By the end of this stage, you will:
- Understand different levels of testing for Kubernetes Operators
- Write effective unit tests for controller logic
- Implement integration tests using the Kubernetes test framework
- Create end-to-end tests that validate complete workflows
- Use mock objects and test helpers for isolated testing
- Validate backup and restore operations across different scenarios

## Testing Strategy Overview

### Testing Pyramid for Operators

```
    E2E Tests
   /           \
  Integration Tests
 /                 \
Unit Tests (Base)
```

1. **Unit Tests**: Test individual functions and methods in isolation
2. **Integration Tests**: Test controller reconciliation with real Kubernetes APIs
3. **End-to-End Tests**: Test complete workflows including external dependencies

## Unit Testing

### Setting Up Unit Tests

Unit tests focus on testing individual functions and business logic without requiring a Kubernetes cluster. They should be fast, reliable, and cover edge cases.

#### Example: Testing Backup Resource Creation

```go
func TestBackupResourceCreation(t *testing.T) {
    backup := &backupv1.Backup{
        ObjectMeta: metav1.ObjectMeta{
            Name:      "test-backup",
            Namespace: "default",
        },
        Spec: backupv1.BackupSpec{
            Source: backupv1.BackupSource{
                Namespace:     "test-namespace",
                ResourceTypes: []string{"deployments", "services"},
            },
            Schedule: "0 2 * * *",
            StorageLocation: backupv1.StorageLocation{
                Provider: "minio",
                Bucket:   "test-bucket",
            },
        },
    }

    // Validate the backup resource was created correctly
    assert.Equal(t, "test-namespace", backup.Spec.Source.Namespace)
    assert.Equal(t, "minio", backup.Spec.StorageLocation.Provider)
    assert.Len(t, backup.Spec.Source.ResourceTypes, 2)
}
```

#### Testing Controller Helper Functions

```go
func TestGetResourceTypesToBackup(t *testing.T) {
    reconciler := &BackupReconciler{}

    // Test with explicit resource types
    source := backupv1.BackupSource{
        Namespace:     "test",
        ResourceTypes: []string{"deployments", "services"},
    }
    
    types := reconciler.getResourceTypesToBackup(source)
    assert.Len(t, types, 2)
    assert.Contains(t, types, "deployments")
    assert.Contains(t, types, "services")
}
```

### Running Unit Tests

```bash
# Run all unit tests
cd cluster-backup-operator
go test ./internal/controller -v

# Run specific test
go test ./internal/controller -run TestBackupResourceCreation -v

# Run tests with coverage
go test ./internal/controller -cover -v
```

## Integration Testing

Integration tests use the Kubernetes test framework to test controller reconciliation with real Kubernetes APIs. These tests run against a test control plane.

### Setting Up Integration Tests

Integration tests use Ginkgo and Gomega testing frameworks, which are standard for Kubernetes projects.

#### Example: Testing Backup Controller Reconciliation

```go
var _ = Describe("Backup Controller Integration", func() {
    var (
        backup    *backupv1.Backup
        namespace string
    )

    BeforeEach(func() {
        namespace = "test-" + rand.String(5)
        
        // Create test namespace
        ns := &corev1.Namespace{
            ObjectMeta: metav1.ObjectMeta{Name: namespace},
        }
        Expect(k8sClient.Create(ctx, ns)).To(Succeed())

        // Create backup resource
        backup = &backupv1.Backup{
            ObjectMeta: metav1.ObjectMeta{
                Name:      "test-backup",
                Namespace: namespace,
            },
            Spec: backupv1.BackupSpec{
                Source: backupv1.BackupSource{
                    Namespace:     namespace,
                    ResourceTypes: []string{"deployments"},
                },
                Schedule: "*/5 * * * *",
                StorageLocation: backupv1.StorageLocation{
                    Provider: "minio",
                    Bucket:   "test-bucket",
                },
            },
        }
    })

    AfterEach(func() {
        // Cleanup
        Expect(k8sClient.Delete(ctx, backup)).To(Succeed())
        
        ns := &corev1.Namespace{
            ObjectMeta: metav1.ObjectMeta{Name: namespace},
        }
        Expect(k8sClient.Delete(ctx, ns)).To(Succeed())
    })

    It("should update backup status after reconciliation", func() {
        By("Creating the backup resource")
        Expect(k8sClient.Create(ctx, backup)).To(Succeed())

        By("Waiting for status to be updated")
        Eventually(func() backupv1.BackupPhase {
            updated := &backupv1.Backup{}
            err := k8sClient.Get(ctx, client.ObjectKeyFromObject(backup), updated)
            if err != nil {
                return ""
            }
            return updated.Status.Phase
        }, timeout, interval).Should(Equal(backupv1.BackupPhaseScheduled))
    })
})
```

### Running Integration Tests

```bash
# Run integration tests
cd cluster-backup-operator
make test

# Run specific integration test suite
ginkgo -v ./internal/controller
```

## End-to-End Testing

E2E tests validate complete workflows including external dependencies like Minio storage. These tests run against a real Kubernetes cluster.

### Setting Up E2E Tests

E2E tests require:
- A running Kubernetes cluster (kind, minikube, or real cluster)
- Deployed operator
- External dependencies (Minio container)

#### Example: Testing Complete Backup and Restore Workflow

```go
var _ = Describe("Backup and Restore E2E", func() {
    var (
        sourceNamespace string
        targetNamespace string
        testDeployment  *appsv1.Deployment
    )

    BeforeEach(func() {
        sourceNamespace = "source-" + rand.String(5)
        targetNamespace = "target-" + rand.String(5)

        // Create namespaces
        for _, ns := range []string{sourceNamespace, targetNamespace} {
            namespace := &corev1.Namespace{
                ObjectMeta: metav1.ObjectMeta{Name: ns},
            }
            Expect(k8sClient.Create(ctx, namespace)).To(Succeed())
        }

        // Create test deployment in source namespace
        testDeployment = &appsv1.Deployment{
            ObjectMeta: metav1.ObjectMeta{
                Name:      "test-app",
                Namespace: sourceNamespace,
                Labels:    map[string]string{"backup": "enabled"},
            },
            Spec: appsv1.DeploymentSpec{
                Replicas: int32Ptr(1),
                Selector: &metav1.LabelSelector{
                    MatchLabels: map[string]string{"app": "test"},
                },
                Template: corev1.PodTemplateSpec{
                    ObjectMeta: metav1.ObjectMeta{
                        Labels: map[string]string{"app": "test"},
                    },
                    Spec: corev1.PodSpec{
                        Containers: []corev1.Container{{
                            Name:  "test",
                            Image: "nginx:latest",
                        }},
                    },
                },
            },
        }
        Expect(k8sClient.Create(ctx, testDeployment)).To(Succeed())
    })

    It("should backup and restore resources successfully", func() {
        By("Creating a backup resource")
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

        By("Waiting for backup to complete")
        Eventually(func() backupv1.BackupPhase {
            updated := &backupv1.Backup{}
            err := k8sClient.Get(ctx, client.ObjectKeyFromObject(backup), updated)
            if err != nil {
                return ""
            }
            return updated.Status.Phase
        }, 2*time.Minute, 5*time.Second).Should(Equal(backupv1.BackupPhaseCompleted))

        By("Creating a restore resource")
        restore := &backupv1.Restore{
            ObjectMeta: metav1.ObjectMeta{
                Name:      "e2e-restore",
                Namespace: targetNamespace,
            },
            Spec: backupv1.RestoreSpec{
                BackupName: backup.Name,
                StorageLocation: backup.Spec.StorageLocation,
                TargetNamespace: targetNamespace,
            },
        }
        Expect(k8sClient.Create(ctx, restore)).To(Succeed())

        By("Waiting for restore to complete")
        Eventually(func() backupv1.RestorePhase {
            updated := &backupv1.Restore{}
            err := k8sClient.Get(ctx, client.ObjectKeyFromObject(restore), updated)
            if err != nil {
                return ""
            }
            return updated.Status.Phase
        }, 2*time.Minute, 5*time.Second).Should(Equal(backupv1.RestorePhaseCompleted))

        By("Verifying the deployment was restored")
        restoredDeployment := &appsv1.Deployment{}
        Eventually(func() error {
            return k8sClient.Get(ctx, types.NamespacedName{
                Name:      testDeployment.Name,
                Namespace: targetNamespace,
            }, restoredDeployment)
        }, timeout, interval).Should(Succeed())

        // Verify deployment properties
        Expect(restoredDeployment.Spec.Replicas).To(Equal(testDeployment.Spec.Replicas))
        Expect(restoredDeployment.Labels["backup"]).To(Equal("enabled"))
    })
})
```

### Running E2E Tests

```bash
# Start Minio container
./scripts/start-minio-docker.sh

# Deploy the operator
make deploy IMG=controller:latest

# Run E2E tests
make test-e2e

# Cleanup
make undeploy
```

## Test Helpers and Utilities

### Mock Objects for External Dependencies

When testing controller logic that interacts with external services like Minio, use mock objects:

```go
type MockMinioClient struct {
    objects map[string][]byte
}

func (m *MockMinioClient) PutObject(bucket, key string, data []byte) error {
    if m.objects == nil {
        m.objects = make(map[string][]byte)
    }
    m.objects[fmt.Sprintf("%s/%s", bucket, key)] = data
    return nil
}

func (m *MockMinioClient) GetObject(bucket, key string) ([]byte, error) {
    data, exists := m.objects[fmt.Sprintf("%s/%s", bucket, key)]
    if !exists {
        return nil, fmt.Errorf("object not found")
    }
    return data, nil
}
```

### Test Data Generators

Create helper functions to generate test data:

```go
func createTestDeployment(name, namespace string) *appsv1.Deployment {
    return &appsv1.Deployment{
        ObjectMeta: metav1.ObjectMeta{
            Name:      name,
            Namespace: namespace,
            Labels:    map[string]string{"test": "true"},
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
                    }},
                },
            },
        },
    }
}

func createTestBackup(name, namespace string) *backupv1.Backup {
    return &backupv1.Backup{
        ObjectMeta: metav1.ObjectMeta{
            Name:      name,
            Namespace: namespace,
        },
        Spec: backupv1.BackupSpec{
            Source: backupv1.BackupSource{
                Namespace:     namespace,
                ResourceTypes: []string{"deployments", "services"},
            },
            Schedule: "0 2 * * *",
            StorageLocation: backupv1.StorageLocation{
                Provider:  "minio",
                Bucket:    "test-bucket",
                Endpoint:  "http://localhost:9000",
                AccessKey: "minioadmin",
                SecretKey: "minioadmin123",
            },
        },
    }
}
```

## Test Scenarios and Coverage

### Critical Test Scenarios

1. **Backup Scenarios**:
   - Successful backup of multiple resource types
   - Backup with label selectors
   - Backup scheduling and timing
   - Backup failure handling
   - Large resource backup performance

2. **Restore Scenarios**:
   - Cross-namespace restore
   - Cross-cluster restore
   - Partial restore (specific resources)
   - Restore conflict handling
   - Restore validation and verification

3. **Error Scenarios**:
   - Minio connection failures
   - Invalid backup configurations
   - Resource conflicts during restore
   - Network timeouts
   - Insufficient permissions

### Coverage Goals

- **Unit Tests**: 80%+ code coverage for controller logic
- **Integration Tests**: Cover all reconciliation paths
- **E2E Tests**: Cover primary user workflows

## Continuous Testing

### GitHub Actions Integration

```yaml
name: Test
on: [push, pull_request]
jobs:
  unit-tests:
    runs-on: ubuntu-latest
    steps:
    - uses: actions/checkout@v3
    - uses: actions/setup-go@v3
      with:
        go-version: 1.21
    - run: make test-unit
    
  integration-tests:
    runs-on: ubuntu-latest
    steps:
    - uses: actions/checkout@v3
    - uses: actions/setup-go@v3
      with:
        go-version: 1.21
    - run: make test-integration
    
  e2e-tests:
    runs-on: ubuntu-latest
    steps:
    - uses: actions/checkout@v3
    - uses: actions/setup-go@v3
      with:
        go-version: 1.21
    - run: |
        kind create cluster
        make deploy IMG=controller:latest
        make test-e2e
```

## Quick Reference

### Test Commands
```bash
# Run all tests
make test-all

# Run unit tests only (works without additional setup)
make test-unit

# Run integration tests (requires envtest setup)
make setup-envtest  # First time setup
make test-integration

# Generate coverage report
make test-coverage

# Run performance tests (integration tests with longer timeout)
make test-performance
```

### Setup Requirements

**Unit Tests**: No additional setup required - these test individual functions in isolation.

**Integration Tests**: Require envtest binaries for running a test Kubernetes control plane:
```bash
# Install envtest binaries
make setup-envtest

# Or manually install kubebuilder which includes envtest
# See: https://book.kubebuilder.io/quick-start.html#installation
```

**E2E Tests**: Require a running Kubernetes cluster and Minio instance.

### Test Structure
```
cluster-backup-operator/test/
├── unit/                    # Unit tests (✅ Working)
├── integration/             # Integration tests (requires envtest setup)
├── helpers/                 # Test utilities
├── mocks/                   # Mock objects
└── testdata/               # Test scenarios and data
```


## Next Steps

The next stage will cover advanced features like secrets management and production-ready patterns.


[← Restore Controller Implementation](05-Restore-Controller-Implementation)&nbsp;&nbsp;&nbsp;&nbsp;&nbsp;&nbsp;[Advanced Features →](07-Advanced-Features)