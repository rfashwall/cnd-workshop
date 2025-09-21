package controller

import (
	"testing"
	"time"

	backupv1 "github.com/rfashwall/cnd-workshop/api/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// TestBackupControllerStructure tests the basic structure and methods of BackupReconciler
func TestBackupControllerStructure(t *testing.T) {
	// Test that BackupReconciler can be instantiated
	reconciler := &BackupReconciler{}
	if reconciler == nil {
		t.Error("BackupReconciler should be instantiable")
	}
}

// TestBackupResourceCreation tests creating a Backup resource
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

	if backup.Spec.Source.Namespace != "test-namespace" {
		t.Errorf("Expected namespace 'test-namespace', got '%s'", backup.Spec.Source.Namespace)
	}

	if backup.Spec.StorageLocation.Provider != "minio" {
		t.Errorf("Expected provider 'minio', got '%s'", backup.Spec.StorageLocation.Provider)
	}

	if len(backup.Spec.Source.ResourceTypes) != 3 {
		t.Errorf("Expected 3 resource types, got %d", len(backup.Spec.Source.ResourceTypes))
	}

	if backup.Spec.Source.LabelSelector == nil {
		t.Error("Expected label selector to be set")
	}
}

// TestGetResourceTypesToBackup tests the resource type selection logic
func TestGetResourceTypesToBackup(t *testing.T) {
	reconciler := &BackupReconciler{}

	// Test with explicit resource types
	source1 := backupv1.BackupSource{
		Namespace:     "test",
		ResourceTypes: []string{"deployments", "services"},
	}
	types1 := reconciler.getResourceTypesToBackup(source1)
	if len(types1) != 2 {
		t.Errorf("Expected 2 resource types, got %d", len(types1))
	}

	// Test with default resource types (empty list)
	source2 := backupv1.BackupSource{
		Namespace: "test",
	}
	types2 := reconciler.getResourceTypesToBackup(source2)
	if len(types2) == 0 {
		t.Error("Expected default resource types, got empty list")
	}

	// Should include common resource types
	expectedTypes := []string{"deployments", "services", "configmaps", "secrets", "persistentvolumeclaims", "ingresses"}
	for _, expectedType := range expectedTypes {
		found := false
		for _, actualType := range types2 {
			if actualType == expectedType {
				found = true
				break
			}
		}
		if !found {
			t.Errorf("Expected resource type '%s' in default list", expectedType)
		}
	}
}

// TestCalculateNextBackupTime tests the backup scheduling logic
func TestCalculateNextBackupTime(t *testing.T) {
	reconciler := &BackupReconciler{}

	// Test daily schedule
	nextTime, err := reconciler.calculateNextBackupTime("0 2 * * *")
	if err != nil {
		t.Errorf("Expected no error for daily schedule, got %v", err)
	}
	if nextTime.Hour() != 2 && nextTime.Hour() != 2 { // Handle day rollover
		// If it's already past 2 AM today, it should be 2 AM tomorrow
		if nextTime.Hour() != 2 {
			t.Errorf("Expected next backup at 2 AM, got %d", nextTime.Hour())
		}
	}

	// Test every 6 hours
	now := time.Now()
	nextTime, err = reconciler.calculateNextBackupTime("0 */6 * * *")
	if err != nil {
		t.Errorf("Expected no error for 6-hour schedule, got %v", err)
	}
	// With proper cron parsing, this should be the next 6-hour boundary (0, 6, 12, 18)
	if nextTime.Before(now) {
		t.Errorf("Next backup time should be in the future, got %v", nextTime)
	}

	// Test every 5 minutes (testing schedule)
	nextTime, err = reconciler.calculateNextBackupTime("*/5 * * * *")
	if err != nil {
		t.Errorf("Expected no error for 5-minute schedule, got %v", err)
	}
	// Should be within the next 5 minutes
	if nextTime.Sub(now) > 5*time.Minute || nextTime.Before(now) {
		t.Errorf("Expected next backup within 5 minutes, got %v from now", nextTime.Sub(now))
	}

	// Test invalid schedule (should return error)
	_, err = reconciler.calculateNextBackupTime("invalid cron")
	if err == nil {
		t.Error("Expected error for invalid cron schedule, got nil")
	}

	// Test another valid schedule - every minute
	nextTime, err = reconciler.calculateNextBackupTime("* * * * *")
	if err != nil {
		t.Errorf("Expected no error for every-minute schedule, got %v", err)
	}
	if nextTime.Sub(now) > time.Minute || nextTime.Before(now) {
		t.Errorf("Expected next backup within 1 minute, got %v from now", nextTime.Sub(now))
	}
}

// TestBackupPhases tests the backup phase constants
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
		if string(phase) != expectedPhases[i] {
			t.Errorf("Expected phase '%s', got '%s'", expectedPhases[i], string(phase))
		}
	}

	// Note: In normal operation, backups cycle between Scheduled -> Running -> Scheduled
	// The "Completed" phase is only used as a fallback when scheduling fails
}
