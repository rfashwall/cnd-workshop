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

// RestoreSpec defines the desired state of Restore.
type RestoreSpec struct {
	// Source defines where to restore from
	Source RestoreSource `json:"source"`

	// Target defines what and where to restore
	Target RestoreTarget `json:"target"`

	// Options for restore behavior
	Options RestoreOptions `json:"options,omitempty"`
}

// RestoreSource defines the backup source location
type RestoreSource struct {
	// StorageLocation where the backup is stored
	StorageLocation StorageLocation `json:"storageLocation"`

	// BackupPath is the specific backup to restore from
	BackupPath string `json:"backupPath"`
}

// RestoreTarget defines restoration targets and behavior
type RestoreTarget struct {
	// Namespaces to restore to. If empty, restores to original namespaces
	Namespaces []string `json:"namespaces,omitempty"`

	// ResourceTypes to restore. If empty, restores all resource types from backup
	ResourceTypes []string `json:"resourceTypes,omitempty"`

	// ConflictResolution strategy when resources already exist: skip, overwrite, fail
	ConflictResolution string `json:"conflictResolution,omitempty"`

	// LabelSelector for filtering resources to restore
	LabelSelector *metav1.LabelSelector `json:"labelSelector,omitempty"`

	// NamespaceMapping maps source namespaces to target namespaces
	// Format: {"source-ns": "target-ns"}
	NamespaceMapping map[string]string `json:"namespaceMapping,omitempty"`
}

// RestoreOptions defines additional restore options
type RestoreOptions struct {
	// DryRun performs validation without actually restoring resources
	DryRun bool `json:"dryRun,omitempty"`

	// ValidateOnly validates the backup without restoring
	ValidateOnly bool `json:"validateOnly,omitempty"`

	// CreateNamespaces automatically creates target namespaces if they don't exist
	CreateNamespaces bool `json:"createNamespaces,omitempty"`

	// SkipClusterResources skips restoration of cluster-scoped resources
	SkipClusterResources bool `json:"skipClusterResources,omitempty"`
}

// RestoreStatus defines the observed state of Restore.
type RestoreStatus struct {
	// Phase represents the current phase of the restore
	Phase RestorePhase `json:"phase,omitempty"`

	// Message provides additional information about the current state
	Message string `json:"message,omitempty"`

	// StartTime is when the restore operation started
	StartTime *metav1.Time `json:"startTime,omitempty"`

	// CompletionTime is when the restore operation completed
	CompletionTime *metav1.Time `json:"completionTime,omitempty"`

	// ResourceCounts tracks how many resources of each type were restored
	ResourceCounts map[string]int32 `json:"resourceCounts,omitempty"`

	// RestoredResources lists the resources that were successfully restored
	RestoredResources []RestoredResource `json:"restoredResources,omitempty"`

	// FailedResources lists the resources that failed to restore
	FailedResources []FailedResource `json:"failedResources,omitempty"`

	// SkippedResources lists the resources that were skipped due to conflicts
	SkippedResources []SkippedResource `json:"skippedResources,omitempty"`

	// BackupInfo contains information about the source backup
	BackupInfo *BackupInfo `json:"backupInfo,omitempty"`
}

// RestorePhase represents the current phase of a restore operation
type RestorePhase string

const (
	RestorePhaseNew         RestorePhase = "New"
	RestorePhaseValidating  RestorePhase = "Validating"
	RestorePhaseDownloading RestorePhase = "Downloading"
	RestorePhaseRestoring   RestorePhase = "Restoring"
	RestorePhaseCompleted   RestorePhase = "Completed"
	RestorePhaseFailed      RestorePhase = "Failed"
)

// RestoredResource represents a successfully restored resource
type RestoredResource struct {
	// APIVersion of the restored resource
	APIVersion string `json:"apiVersion"`

	// Kind of the restored resource
	Kind string `json:"kind"`

	// Name of the restored resource
	Name string `json:"name"`

	// Namespace of the restored resource (empty for cluster-scoped resources)
	Namespace string `json:"namespace,omitempty"`

	// Action taken during restoration (created, updated)
	Action string `json:"action"`
}

// FailedResource represents a resource that failed to restore
type FailedResource struct {
	// APIVersion of the failed resource
	APIVersion string `json:"apiVersion"`

	// Kind of the failed resource
	Kind string `json:"kind"`

	// Name of the failed resource
	Name string `json:"name"`

	// Namespace of the failed resource (empty for cluster-scoped resources)
	Namespace string `json:"namespace,omitempty"`

	// Error message describing why the restoration failed
	Error string `json:"error"`
}

// SkippedResource represents a resource that was skipped during restoration
type SkippedResource struct {
	// APIVersion of the skipped resource
	APIVersion string `json:"apiVersion"`

	// Kind of the skipped resource
	Kind string `json:"kind"`

	// Name of the skipped resource
	Name string `json:"name"`

	// Namespace of the skipped resource (empty for cluster-scoped resources)
	Namespace string `json:"namespace,omitempty"`

	// Reason why the resource was skipped
	Reason string `json:"reason"`
}

// BackupInfo contains information about the source backup
type BackupInfo struct {
	// BackupPath is the path in storage where the backup was found
	BackupPath string `json:"backupPath"`

	// BackupTime is when the backup was created
	BackupTime *metav1.Time `json:"backupTime,omitempty"`

	// TotalResources is the total number of resources in the backup
	TotalResources int32 `json:"totalResources,omitempty"`

	// ResourceTypes lists the types of resources found in the backup
	ResourceTypes []string `json:"resourceTypes,omitempty"`

	// Namespaces lists the namespaces found in the backup
	Namespaces []string `json:"namespaces,omitempty"`
}

//+kubebuilder:object:root=true
//+kubebuilder:subresource:status
//+kubebuilder:printcolumn:name="Phase",type=string,JSONPath=`.status.phase`
//+kubebuilder:printcolumn:name="Backup Path",type=string,JSONPath=`.spec.source.backupPath`
//+kubebuilder:printcolumn:name="Restored",type=integer,JSONPath=`.status.resourceCounts.total`
//+kubebuilder:printcolumn:name="Age",type=date,JSONPath=`.metadata.creationTimestamp`

// Restore is the Schema for the restores API
type Restore struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	Spec   RestoreSpec   `json:"spec,omitempty"`
	Status RestoreStatus `json:"status,omitempty"`
}

//+kubebuilder:object:root=true

// RestoreList contains a list of Restore
type RestoreList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []Restore `json:"items"`
}

func init() {
	SchemeBuilder.Register(&Restore{}, &RestoreList{})
}
