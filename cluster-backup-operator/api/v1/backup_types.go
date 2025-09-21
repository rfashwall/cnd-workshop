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

package v1

import (
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// EDIT THIS FILE!  THIS IS SCAFFOLDING FOR YOU TO OWN!
// NOTE: json tags are required.  Any new fields you add must have json tags for the fields to be serialized.

// BackupSpec defines the desired state of Backup.
type BackupSpec struct {
	// Source defines what to backup (e.g., namespace, deployment)
	Source BackupSource `json:"source"`

	// Schedule defines when to perform backups (cron format)
	Schedule string `json:"schedule"`

	// Retention defines how long to keep backups
	Retention string `json:"retention,omitempty"`

	// StorageLocation defines where to store backups
	StorageLocation StorageLocation `json:"storageLocation"`
}

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

// BackupPhase represents the current phase of a backup
type BackupPhase string

const (
	BackupPhaseNew       BackupPhase = "New"
	BackupPhaseScheduled BackupPhase = "Scheduled"
	BackupPhaseRunning   BackupPhase = "Running"
	BackupPhaseCompleted BackupPhase = "Completed"
	BackupPhaseFailed    BackupPhase = "Failed"
)

// +kubebuilder:object:root=true
// +kubebuilder:subresource:status

// Backup is the Schema for the backups API.
type Backup struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	Spec   BackupSpec   `json:"spec,omitempty"`
	Status BackupStatus `json:"status,omitempty"`
}

// +kubebuilder:object:root=true

// BackupList contains a list of Backup.
type BackupList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []Backup `json:"items"`
}

func init() {
	SchemeBuilder.Register(&Backup{}, &BackupList{})
}
