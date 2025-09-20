# Operator SDK and Backup Controller Implementation

In this stage, we'll use the Operator SDK to create a backup controller that manages Kubernetes resource backups.

## What is the Operator SDK?

The Operator SDK is a tool that helps you build Kubernetes Operators quickly and easily. Think of it as a project generator that creates all the boilerplate code you need.

### What does the Operator SDK create for you?

When you use the Operator SDK, it generates a complete project structure with:

1. **Custom Resource Definition (CRD)** - This defines a new type of Kubernetes resource (like `Backup`)
2. **Controller Code** - This watches for your custom resources and takes action
3. **Configuration Files** - RBAC permissions, deployment manifests, etc.
4. **Build Tools** - Makefile with commands to build, test, and deploy
5. **Project Structure** - Organized folders following Kubernetes best practices

### Simple Analogy

Think of the Operator SDK like a house blueprint generator:
- You tell it "I want a house for backups" 
- It gives you the complete blueprints, foundation, and basic structure
- You then customize the rooms (add your backup logic)

### Operator SDK vs Manual Development

| What you need | Manual Way | Operator SDK Way |
|---------------|------------|------------------|
| Project setup | Hours of boilerplate | 2 minutes |
| CRD definition | Write YAML by hand | Auto-generated |
| Controller skeleton | Start from scratch | Ready template |
| Build system | Create your own | Makefile included |
| RBAC permissions | Figure out yourself | Generated for you |

## What We'll Build

We're going to create a **Backup Operator** that:
1. Watches for `Backup` custom resources
2. Reads the backup configuration (what to backup, where to store it)
3. Performs the actual backup operations
4. Updates the status to show progress

## Prerequisites

The Operator SDK should already be installed in your Codespaces environment. Let's verify:

```bash
# Check Operator SDK version
operator-sdk version

# Expected output:
# operator-sdk version: "v1.32.0", commit: "...", kubernetes version: "v1.28.0", go version: "go1.21.3", GOOS: "linux", GOARCH: "amd64"
```

## Understanding the Generated Project Structure

Before we start building, let's understand what the Operator SDK will create for us:

```
cluster-backup-operator/          # Our project root
├── Dockerfile                   # How to build our operator as a container
├── Makefile                     # Commands to build, test, deploy
├── PROJECT                      # Metadata about our project
├── go.mod                       # Go dependencies
├── cmd/main.go                  # The main entry point - starts our operator
├── api/v1/                      # Custom Resource Definitions
│   └── backup_types.go          # Defines what a "Backup" looks like
├── internal/controller/         # The brain of our operator
│   └── backup_controller.go     # Logic for handling Backup resources
└── config/                      # Kubernetes configuration files
    ├── crd/                     # Custom Resource Definition manifests
    ├── rbac/                    # Permissions our operator needs
    └── samples/                 # Example Backup resources
```

### Key Files Explained

- **`api/v1/backup_types.go`** - Defines the structure of our Backup resource (what fields it has)
- **`internal/controller/backup_controller.go`** - Contains the logic that runs when Backup resources are created/updated
- **`config/crd/`** - Kubernetes manifests that register our new Backup resource type
- **`config/rbac/`** - Permissions that allow our controller to read/write Kubernetes resources

## Step-by-Step Implementation

Now let's build our backup operator step by step.

### Step 1: Initialize the Operator Project

Create a new directory for our operator and initialize it:

```bash
# Create and navigate to the operator directory
mkdir cluster-backup-operator
cd cluster-backup-operator

# Initialize the operator project
operator-sdk init --domain=cnd.dk --repo=github.com/rfashwall/cnd-workshop

# This creates the basic project structure
ls -la
```

**What this command does:**
- Creates a new Go project with all the necessary files
- Sets up `cnd.dk` as the domain for our custom resources
- Generates Makefile, Dockerfile, and basic configuration
- Creates the folder structure we saw above

### Step 2: Create a Custom Resource API

Now let's create our first Custom Resource for backups:

```bash
# Create the Backup API (make sure you're in the cluster-backup-operator directory)
operator-sdk create api --group=backup --version=v1 --kind=Backup --resource --controller

# Answer the prompts:
# Create Resource [y/n]: y
# Create Controller [y/n]: y
```

**What this command creates:**
- **Backup Resource Definition** (`api/v1/backup_types.go`) - Defines what a Backup looks like
- **Controller Logic** (`internal/controller/backup_controller.go`) - The code that handles Backup resources
- **Kubernetes Manifests** - Files that tell Kubernetes about our new Backup resource type
- **Sample Files** - Example Backup resources we can use for testing

### Step 3: Understand What Was Generated

Let's look at what the Operator SDK created for us. First, check the generated Backup resource definition:

```bash
# Look at the generated backup types
cat api/v1/backup_types.go
```

You'll see it has placeholder fields like `Foo string`. We need to replace these with real backup configuration.

### Step 4: Customize the Backup Resource Types

Now let's define what our Backup resource should look like. We want to specify:
- **What to backup** (which namespace, which resources)
- **When to backup** (schedule)
- **Where to store it** (Minio configuration)
- **How long to keep it** (retention)

Edit `api/v1/backup_types.go` to define a proper backup specification:

```go
// BackupSpec defines the desired state of Backup
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
    // Namespace to backup
    Namespace string `json:"namespace"`
    
    // LabelSelector for resources to backup
    LabelSelector *metav1.LabelSelector `json:"labelSelector,omitempty"`
}

// StorageLocation defines backup storage configuration
type StorageLocation struct {
    // Provider (e.g., "minio", "s3", "gcs")
    Provider string `json:"provider"`
    
    // Bucket name
    Bucket string `json:"bucket"`
    
    // Endpoint URL (for Minio)
    Endpoint string `json:"endpoint,omitempty"`
    
    // Username for authentication (plain text for now)
    Username string `json:"username,omitempty"`
    
    // Password for authentication (plain text for now)
    Password string `json:"password,omitempty"`
}

// Note: In this workshop, we're using plain text credentials for simplicity.
// In the advanced section, we'll implement proper secret management.

// BackupStatus defines the observed state of Backup
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
}

// BackupPhase represents the current phase of a backup
type BackupPhase string

const (
    BackupPhaseNew        BackupPhase = "New"
    BackupPhaseScheduled  BackupPhase = "Scheduled"
    BackupPhaseRunning    BackupPhase = "Running"
    BackupPhaseCompleted  BackupPhase = "Completed"
    BackupPhaseFailed     BackupPhase = "Failed"
)
```

### Step 5: Generate Updated Code

After modifying the types, regenerate the code:

```bash
# Generate deepcopy methods and CRD manifests
make generate manifests

# Check what was generated
git status
```

### Step 6: Create a Sample Backup Resource

Update `config/samples/backup_v1_backup.yaml`:

```yaml
apiVersion: backup.example.com/v1
kind: Backup
metadata:
  labels:
    app.kubernetes.io/name: backup
    app.kubernetes.io/instance: backup-sample
    app.kubernetes.io/part-of: backup-operator
    app.kubernetes.io/managed-by: kustomize
    app.kubernetes.io/created-by: backup-operator
  name: backup-sample
spec:
  source:
    namespace: "default"
    labelSelector:
      matchLabels:
        app: "nginx"
  schedule: "0 2 * * *"  # Daily at 2 AM
  retention: "30d"       # Keep for 30 days
  storageLocation:
    provider: "minio"
    bucket: "backups"
    endpoint: "http://minio:9000"
    username: "minioadmin"
    password: "minioadmin"
```

### Step 7: Understand the Controller Skeleton

Before we add our logic, let's look at what the Operator SDK generated:

```bash
# Look at the generated controller
cat internal/controller/backup_controller.go
```

You'll see a `Reconcile` function with a TODO comment. This is where we'll add our backup logic.

### Step 8: Implement Basic Controller Logic

The controller's job is to:
1. **Watch** for Backup resources being created/updated/deleted
2. **Read** the backup configuration 
3. **Take action** based on what the user wants
4. **Update status** to show what's happening

Let's replace the TODO with some basic logic:

Now let's add some basic logic to our controller. Edit `internal/controller/backup_controller.go` and replace the Reconcile function:

```go
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
```

### Step 9: Test the CRD Installation

Let's install our CRD and test creating a backup resource:

```bash
# Install the CRDs
make install

# Verify CRD is installed
kubectl get crd backups.backup.cnd.dk

# Create a sample backup
kubectl apply -f config/samples/backup_v1_backup.yaml

# Check the created resource
kubectl get backups
kubectl describe backup backup-sample
```

### Step 10: Run and Test the Controller

1. **Install the CRDs:**
   ```bash
   make install
   ```

2. **Run the controller locally:**
   ```bash
   make run
   ```

3. **In another terminal, create a backup resource:**
   ```bash
   kubectl apply -f config/samples/backup_v1_backup.yaml
   ```

4. **Check the backup status:**
   ```bash
   kubectl get backups
   kubectl describe backup backup-sample
   ```

5. **Watch the controller logs in the first terminal** - you should see reconciliation events.

### Step 11: Create and Test Custom Backup Resources

Create a custom backup resource to test different configurations:

```yaml
# Save as my-backup.yaml
apiVersion: backup.cnd.dk/v1
kind: Backup
metadata:
  name: my-app-backup
  namespace: default
spec:
  source:
    namespace: "default"
    labelSelector:
      matchLabels:
        app: "my-app"
  schedule: "*/5 * * * *"  # Every 5 minutes for testing
  storageLocation:
    provider: "minio"
    bucket: "test-backups"
    endpoint: "http://localhost:9000"
    username: "minioadmin"
    password: "minioadmin"
```

Apply and monitor:
```bash
kubectl apply -f my-backup.yaml
kubectl get backup my-app-backup -o yaml
```

## Understanding What We Built

Now that we've implemented our backup controller, let's understand the key components and how they work together.

### Operator SDK Architecture

The Operator SDK generated a project with this structure:

```
cluster-backup-operator/
├── Dockerfile                 # Container image build
├── Makefile                  # Build and deployment targets
├── PROJECT                   # Project metadata
├── README.md                 # Project documentation
├── go.mod                    # Go module definition
├── go.sum                    # Go module checksums
├── cmd/main.go               # Operator entry point
├── api/                      # Custom Resource Definitions
│   └── v1/
│       ├── groupversion_info.go
│       ├── backup_types.go   # Custom Resource types
│       └── zz_generated.deepcopy.go
├── config/                   # Kubernetes manifests
│   ├── crd/                  # CRD definitions
│   ├── default/              # Default deployment
│   ├── manager/              # Manager deployment
│   ├── rbac/                 # RBAC permissions
│   └── samples/              # Sample custom resources
├── internal/controller/      # Controller implementations
│   ├── backup_controller.go  # Main controller logic
│   └── suite_test.go         # Test suite setup
└── hack/                     # Utility scripts
    └── boilerplate.go.txt
```

### Controller Architecture

A Kubernetes controller follows the controller pattern:

```
┌─────────────────┐    ┌──────────────────┐    ┌─────────────────┐
│   API Server    │◄──►│    Controller    │◄──►│  External APIs  │
│                 │    │                  │    │   (Minio, etc)  │
│  - Watch Events │    │  - Reconcile     │    │                 │
│  - Store State  │    │  - Update Status │    │                 │
└─────────────────┘    └──────────────────┘    └─────────────────┘
```

### Key Components Explained

1. **Custom Resource Definition (CRD)**: Defines the schema for `Backup` resources
2. **Controller Logic**: Implements the reconciliation loop
3. **Status Management**: Updates the backup status and progress
4. **RBAC Permissions**: Defines what the controller can access

### Understanding the Controller Logic

Our controller implements these key functions:

1. **Resource Fetching**: Gets the Backup resource from the API server
2. **Status Initialization**: Sets initial status for new resources
3. **Phase Management**: Tracks backup lifecycle (New → Scheduled → Running → Completed/Failed)
4. **Logging**: Provides visibility into controller operations

### Backup Status Management

The controller manages different phases:

- `New`: Backup resource just created
- `Scheduled`: Backup scheduled according to cron
- `Running`: Backup operation in progress (to be implemented)
- `Completed`: Backup successfully completed (to be implemented)
- `Failed`: Backup operation failed (to be implemented)

### RBAC Permissions

The controller needs specific permissions defined by kubebuilder annotations:

```go
// +kubebuilder:rbac:groups=backup.cnd.dk,resources=backups,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=backup.cnd.dk,resources=backups/status,verbs=get;update;patch
```

These annotations generate the necessary RBAC manifests in `config/rbac/`.

### Makefile Targets

The generated Makefile provides useful targets for development:

```bash
# View available targets
make help

# Common targets:
make manifests    # Generate CRD and RBAC manifests
make generate     # Generate code (deepcopy methods)
make fmt          # Format Go code
make vet          # Run go vet
make test         # Run unit tests
make build        # Build the operator binary
make run          # Run the operator locally
make docker-build # Build container image
make deploy       # Deploy to Kubernetes cluster
make undeploy     # Remove from Kubernetes cluster
```

## Key Takeaways

1. **Operator SDK accelerates development** by providing scaffolding and best practices
2. **Custom Resources extend Kubernetes** with domain-specific APIs
3. **Controllers watch for changes** and reconcile desired vs actual state
4. **Status management** provides visibility into resource lifecycle
5. **RBAC permissions** control what the controller can access
6. **Local development** allows fast iteration and testing

## Troubleshooting

### Common Issues

**Issue: `make generate` fails**
```bash
# Solution: Ensure controller-gen is installed
go install sigs.k8s.io/controller-tools/cmd/controller-gen@latest
```

**Issue: CRD installation fails**
```bash
# Check if you have cluster admin permissions
kubectl auth can-i create customresourcedefinitions

# If using kind, you should have full permissions
kubectl cluster-info
```

**Issue: Operator fails to start**
```bash
# Check the logs
kubectl logs -n backup-operator-system deployment/backup-operator-controller-manager

# Common causes:
# - RBAC permissions missing
# - CRDs not installed
# - Image not available in cluster
```

**Issue: `make run` fails with permission errors**
```bash
# Ensure you have a valid kubeconfig
kubectl config current-context

# Check if you can access the cluster
kubectl get nodes
```

## Cleanup

```bash
# Remove the sample backup
kubectl delete -f config/samples/backup_v1_backup.yaml

# Uninstall CRDs (optional)
make uninstall

# If deployed to cluster
make undeploy
```

## Checkpoint: Operator SDK Setup Complete

🎉 **Congratulations!** You've successfully completed the Operator SDK setup. At this point you should have:

- ✅ A complete operator project structure in `cluster-backup-operator/`
- ✅ Custom Resource Definition for Backup resources
- ✅ Generated controller skeleton
- ✅ Working Makefile with build targets
- ✅ Sample Backup resource with plain text credentials

### Checkpoint Branch

If you need to catch up or start fresh from this point, you can checkout the checkpoint branch:

```bash
# Save any current work
git stash

# Checkout the checkpoint branch
git checkout checkpoint-03-operator-sdk

# Or if you want to start fresh
git checkout checkpoint-03-operator-sdk
git checkout -b my-operator-work
```

This checkpoint contains all the scaffolding and setup completed in this section.

## Optional: Deploy to Cluster

For a more production-like setup, you can also deploy the controller to your cluster:

```bash
# Build and load the image into kind
make docker-build IMG=backup-operator:latest
kind load docker-image backup-operator:latest --name workshop

# Deploy to the cluster
make deploy IMG=backup-operator:latest

# Check the deployment
kubectl get deployment -n backup-operator-system
kubectl logs -n backup-operator-system deployment/backup-operator-controller-manager
```

## Current Implementation Status

At this point, our controller:
1. ✅ Watches for Backup custom resources
2. ✅ Initializes status for new resources  
3. ✅ Updates phase from New to Scheduled
4. ✅ Logs configuration details
5. ❌ **TODO**: Implement actual backup functionality (next stage)

## Common Issues and Troubleshooting

### Controller Not Starting
- **Issue**: `make run` fails with permission errors
- **Solution**: Ensure you have proper kubeconfig and cluster access

### CRD Installation Fails
- **Issue**: `make install` fails
- **Solution**: Check if you have cluster-admin permissions

### Backup Resource Not Reconciling
- **Issue**: Controller doesn't process backup resources
- **Solution**: Check controller logs for errors, verify RBAC permissions

## Validation Checklist

To verify your understanding:

1. ✅ Can you explain what a Kubernetes controller does?
2. ✅ Can you identify the main components of the backup controller?
3. ✅ Can you create and apply a Backup custom resource?
4. ✅ Can you observe the controller reconciliation in the logs?
5. ✅ Can you explain the different backup phases?

## Checkpoint: Operator and Controller Setup Complete

🎉 **Congratulations!** You've successfully completed the Operator SDK setup and backup controller implementation. At this point you should have:

- ✅ A complete operator project structure in `cluster-backup-operator/`
- ✅ Custom Resource Definition for Backup resources with proper types
- ✅ Working controller with basic reconciliation logic
- ✅ Status management for backup phases
- ✅ RBAC permissions configured
- ✅ Working Makefile with build targets
- ✅ Sample Backup resources for testing

### Checkpoint Branch

If you need to catch up or start fresh from this point, you can checkout the checkpoint branch:

```bash
# Save any current work
git stash

# Checkout the checkpoint branch
git checkout checkpoint-03-operator-sdk

# Or if you want to start fresh
git checkout checkpoint-03-operator-sdk
git checkout -b my-operator-work
```

This checkpoint contains all the scaffolding and basic controller logic completed in this section.

## Next Steps

Now that you have a working controller foundation, the next stage will implement the actual backup functionality:

**Next Stage**: [Buckup Controller Implementation →](04-Buckup-Controller-Implementation) - Implementing actual backup functionality with Minio storage integration.

In the next stage, we'll:
1. Implement the actual backup functionality to replace the TODO comments
2. Connect to Minio storage
3. Backup Kubernetes resources as YAML
4. Handle backup scheduling with cron
5. Implement retention policies

---


[← Controllers](02-Controllers)&nbsp;&nbsp;&nbsp;&nbsp;&nbsp;&nbsp;[Buckup Controller Implementation →](04-Buckup-Controller-Implementation)

[Home](Home)