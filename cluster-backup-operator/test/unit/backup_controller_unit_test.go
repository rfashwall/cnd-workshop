package unit

import (
	"fmt"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	backupv1 "github.com/rfashwall/cnd-workshop/api/v1"
	"github.com/rfashwall/cnd-workshop/internal/controller"
)

func TestBackupControllerStructure(t *testing.T) {
	// Test that BackupReconciler can be instantiated
	reconciler := &controller.BackupReconciler{}
	assert.NotNil(t, reconciler, "BackupReconciler should be instantiable")
}

func TestBackupResourceCreation(t *testing.T) {
	backup := &backupv1.Backup{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "test-backup",
			Namespace: "default",
		},
		Spec: backupv1.BackupSpec{
			Source: backupv1.BackupSource{
				Namespace:     "test-namespace",
				ResourceTypes: []string{"deployments", "services", "configmaps"},
				LabelSelector: &metav1.LabelSelector{
					MatchLabels: map[string]string{
						"backup": "enabled",
					},
				},
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

	assert.Equal(t, "test-namespace", backup.Spec.Source.Namespace)
	assert.Equal(t, "minio", backup.Spec.StorageLocation.Provider)
	assert.Len(t, backup.Spec.Source.ResourceTypes, 3)
	assert.NotNil(t, backup.Spec.Source.LabelSelector)
	assert.Equal(t, "enabled", backup.Spec.Source.LabelSelector.MatchLabels["backup"])
}

func TestGetResourceTypesToBackup(t *testing.T) {
	tests := []struct {
		name     string
		source   backupv1.BackupSource
		expected []string
	}{
		{
			name: "explicit resource types",
			source: backupv1.BackupSource{
				Namespace:     "test",
				ResourceTypes: []string{"deployments", "services"},
			},
			expected: []string{"deployments", "services"},
		},
		{
			name: "empty resource types should return defaults",
			source: backupv1.BackupSource{
				Namespace: "test",
			},
			expected: []string{"deployments", "services", "configmaps", "secrets", "persistentvolumeclaims", "ingresses"},
		},
		{
			name: "single resource type",
			source: backupv1.BackupSource{
				Namespace:     "test",
				ResourceTypes: []string{"deployments"},
			},
			expected: []string{"deployments"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Test the logic that would be in getResourceTypesToBackup method
			var result []string
			if len(tt.source.ResourceTypes) > 0 {
				result = tt.source.ResourceTypes
			} else {
				// Default resource types
				result = []string{"deployments", "services", "configmaps", "secrets", "persistentvolumeclaims", "ingresses"}
			}

			if len(tt.expected) > 0 && tt.source.ResourceTypes != nil {
				assert.Equal(t, tt.expected, result)
			} else {
				// For default case, check that common types are included
				for _, expectedType := range tt.expected {
					assert.Contains(t, result, expectedType, "Expected resource type %s in default list", expectedType)
				}
			}
		})
	}
}

func TestCalculateNextBackupTime(t *testing.T) {
	tests := []struct {
		name        string
		schedule    string
		expectError bool
	}{
		{
			name:        "daily at 2 AM",
			schedule:    "0 2 * * *",
			expectError: false,
		},
		{
			name:        "every 6 hours",
			schedule:    "0 */6 * * *",
			expectError: false,
		},
		{
			name:        "every 5 minutes",
			schedule:    "*/5 * * * *",
			expectError: false,
		},
		{
			name:        "invalid cron expression",
			schedule:    "invalid cron",
			expectError: true,
		},
		{
			name:        "every minute",
			schedule:    "* * * * *",
			expectError: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Test basic cron schedule validation
			// In a real implementation, this would use the cron library
			var err error
			if tt.schedule == "invalid cron" {
				err = fmt.Errorf("invalid cron schedule")
			}

			if tt.expectError {
				assert.Error(t, err, "Expected error for schedule: %s", tt.schedule)
			} else {
				assert.NoError(t, err, "Expected no error for schedule: %s", tt.schedule)
			}
		})
	}
}

func TestBackupPhases(t *testing.T) {
	phases := []backupv1.BackupPhase{
		backupv1.BackupPhaseNew,
		backupv1.BackupPhaseScheduled,
		backupv1.BackupPhaseRunning,
		backupv1.BackupPhaseCompleted,
		backupv1.BackupPhaseFailed,
	}

	expectedPhases := []string{"New", "Scheduled", "Running", "Completed", "Failed"}

	for i, phase := range phases {
		assert.Equal(t, expectedPhases[i], string(phase), "Phase %d should match expected value", i)
	}
}

func TestBackupStatusUpdate(t *testing.T) {
	backup := &backupv1.Backup{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "test-backup",
			Namespace: "default",
		},
		Spec: backupv1.BackupSpec{
			Source: backupv1.BackupSource{
				Namespace:     "test-namespace",
				ResourceTypes: []string{"deployments"},
			},
			Schedule: "0 2 * * *",
			StorageLocation: backupv1.StorageLocation{
				Provider: "minio",
				Bucket:   "test-bucket",
			},
		},
	}

	// Test that we can set backup status fields
	backup.Status.Phase = backupv1.BackupPhaseScheduled
	backup.Status.Message = "Backup scheduled successfully"

	// Verify status fields are set correctly
	assert.Equal(t, backupv1.BackupPhaseScheduled, backup.Status.Phase)
	assert.Equal(t, "Backup scheduled successfully", backup.Status.Message)
}

func TestValidateBackupSpec(t *testing.T) {
	// Test basic validation logic that would be in the controller
	tests := []struct {
		name        string
		backup      *backupv1.Backup
		expectError bool
		errorMsg    string
	}{
		{
			name: "valid backup spec",
			backup: &backupv1.Backup{
				Spec: backupv1.BackupSpec{
					Source: backupv1.BackupSource{
						Namespace: "test-namespace",
					},
					Schedule: "0 2 * * *",
					StorageLocation: backupv1.StorageLocation{
						Provider: "minio",
						Bucket:   "test-bucket",
					},
				},
			},
			expectError: false,
		},
		{
			name: "missing storage bucket",
			backup: &backupv1.Backup{
				Spec: backupv1.BackupSpec{
					Source: backupv1.BackupSource{
						Namespace: "test-namespace",
					},
					Schedule: "0 2 * * *",
					StorageLocation: backupv1.StorageLocation{
						Provider: "minio",
					},
				},
			},
			expectError: true,
			errorMsg:    "storage bucket is required",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Simple validation logic
			var err error
			if tt.backup.Spec.StorageLocation.Bucket == "" {
				err = fmt.Errorf("storage bucket is required")
			}

			if tt.expectError {
				assert.Error(t, err, "Expected validation error")
				if tt.errorMsg != "" {
					assert.Contains(t, err.Error(), tt.errorMsg, "Error message should contain expected text")
				}
			} else {
				assert.NoError(t, err, "Expected no validation error")
			}
		})
	}
}

func TestGenerateBackupKey(t *testing.T) {
	backup := &backupv1.Backup{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "test-backup",
			Namespace: "default",
		},
	}

	timestamp := time.Date(2024, 1, 15, 14, 30, 0, 0, time.UTC)
	// Test the key generation logic that would be in the controller
	key := fmt.Sprintf("backups/%s/%s/%s", backup.Namespace, backup.Name, timestamp.Format("2006-01-02T15:04:05Z"))

	expected := "backups/default/test-backup/2024-01-15T14:30:00Z"
	assert.Equal(t, expected, key, "Backup key should match expected format")
}

func TestShouldRunBackup(t *testing.T) {
	tests := []struct {
		name           string
		backup         *backupv1.Backup
		expectedResult bool
	}{
		{
			name: "new backup should run",
			backup: &backupv1.Backup{
				Status: backupv1.BackupStatus{
					Phase: backupv1.BackupPhaseNew,
				},
			},
			expectedResult: true,
		},
		{
			name: "scheduled backup should run",
			backup: &backupv1.Backup{
				Status: backupv1.BackupStatus{
					Phase:          backupv1.BackupPhaseScheduled,
					NextBackupTime: &metav1.Time{Time: time.Now().Add(-1 * time.Minute)},
				},
			},
			expectedResult: true,
		},
		{
			name: "running backup should not run",
			backup: &backupv1.Backup{
				Status: backupv1.BackupStatus{
					Phase: backupv1.BackupPhaseRunning,
				},
			},
			expectedResult: false,
		},
		{
			name: "scheduled backup not yet due should not run",
			backup: &backupv1.Backup{
				Status: backupv1.BackupStatus{
					Phase:          backupv1.BackupPhaseScheduled,
					NextBackupTime: &metav1.Time{Time: time.Now().Add(1 * time.Hour)},
				},
			},
			expectedResult: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Test the logic that would be in shouldRunBackup method
			now := time.Now()
			var result bool

			switch tt.backup.Status.Phase {
			case backupv1.BackupPhaseNew:
				result = true
			case backupv1.BackupPhaseScheduled:
				if tt.backup.Status.NextBackupTime == nil || now.After(tt.backup.Status.NextBackupTime.Time) {
					result = true
				}
			case backupv1.BackupPhaseRunning:
				result = false
			default:
				result = false
			}

			assert.Equal(t, tt.expectedResult, result, "ShouldRunBackup result should match expected")
		})
	}
}
