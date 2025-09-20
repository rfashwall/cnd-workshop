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
	"context"

	"k8s.io/apimachinery/pkg/runtime"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	logf "sigs.k8s.io/controller-runtime/pkg/log"

	backupv1 "github.com/rfashwall/cnd-workshop/api/v1"
)

// BackupReconciler reconciles a Backup object
type BackupReconciler struct {
	client.Client
	Scheme *runtime.Scheme
}

// +kubebuilder:rbac:groups=backup.cnd.dk,resources=backups,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=backup.cnd.dk,resources=backups/status,verbs=get;update;patch
// +kubebuilder:rbac:groups=backup.cnd.dk,resources=backups/finalizers,verbs=update

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
		"current.phase", backup.Status.Phase)

	// Update phase to scheduled if still new
	if backup.Status.Phase == backupv1.BackupPhaseNew {
		backup.Status.Phase = backupv1.BackupPhaseScheduled
		backup.Status.Message = "Backup scheduled according to cron schedule"
		if err := r.Status().Update(ctx, backup); err != nil {
			log.Error(err, "Failed to update Backup status to scheduled")
			return ctrl.Result{}, err
		}
		log.Info("Updated backup status to scheduled")
	}

	// TODO: Implement actual backup logic here
	// For now, just log that we would perform a backup
	log.Info("Backup reconciliation completed", "backup", backup.Name)

	return ctrl.Result{}, nil
}

// SetupWithManager sets up the controller with the Manager.
func (r *BackupReconciler) SetupWithManager(mgr ctrl.Manager) error {
	return ctrl.NewControllerManagedBy(mgr).
		For(&backupv1.Backup{}).
		Named("backup").
		Complete(r)
}
