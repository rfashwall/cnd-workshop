# Operator SDK Introduction

The Operator SDK is a framework that makes it easier to build Kubernetes Operators. It provides tools and libraries to generate, build, and package Operators following best practices.

## What is the Operator SDK?

The Operator SDK is part of the Operator Framework and provides:

- **Project scaffolding**: Generate boilerplate code for new Operators
- **Code generation**: Automatically generate Kubernetes client code
- **Testing utilities**: Tools for unit and integration testing
- **Build and deployment**: Streamlined build and deployment workflows
- **Best practices**: Opinionated structure following Kubernetes conventions

### Operator SDK vs Manual Development

| Aspect | Manual Development | Operator SDK |
|--------|-------------------|--------------|
| Setup Time | Hours/Days | Minutes |
| Boilerplate Code | Write everything | Auto-generated |
| Best Practices | Must research | Built-in |
| Testing | Manual setup | Integrated tools |
| Deployment | Custom scripts | Built-in commands |

## Operator SDK Architecture

The Operator SDK generates projects with a standard structure:

```
my-operator/
├── Dockerfile                 # Container image build
├── Makefile                  # Build and deployment targets
├── PROJECT                   # Project metadata
├── README.md                 # Project documentation
├── go.mod                    # Go module definition
├── go.sum                    # Go module checksums
├── main.go                   # Operator entry point
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
├── controllers/              # Controller implementations
│   ├── backup_controller.go  # Main controller logic
│   └── suite_test.go         # Test suite setup
└── hack/                     # Utility scripts
    └── boilerplate.go.txt
```

## Installing Operator SDK

The Operator SDK should already be installed in your Codespaces environment. Let's verify:

```bash
# Check Operator SDK version
operator-sdk version

# Expected output:
# operator-sdk version: "v1.32.0", commit: "...", kubernetes version: "v1.28.0", go version: "go1.21.3", GOOS: "linux", GOARCH: "amd64"
```

If you need to install it locally, follow the installation guide from the [setup documentation](00-setup.md).

## Creating Your First Operator

Let's create a backup operator that will manage backup operations for applications.

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
- Creates a new Go module with the specified repository path
- Generates the basic project structure
- Sets up the domain for your Custom Resources
- Creates initial configuration files
- Generates the basic project structure
- Sets up the domain for your Custom Resources
- Creates initial configuration files

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
- `api/v1/backup_types.go`: Defines the Backup Custom Resource structure
- `internal/controller/backup_controller.go`: Contains the controller logic
- CRD manifests in `config/crd/bases/`
- RBAC permissions in `config/rbac/`
- Sample resources in `config/samples/`

## Understanding the Generated Code

### Custom Resource Types (api/v1/backup_types.go)

The generated types file defines the structure of your Custom Resource:

```go
// BackupSpec defines the desired state of Backup
type BackupSpec struct {
    // INSERT ADDITIONAL SPEC FIELDS - desired state of cluster
    // Important: Run "make" to regenerate code after modifying this file

    // Foo is an example field of Backup. Edit backup_types.go to remove/update
    Foo string `json:"foo,omitempty"`
}

// BackupStatus defines the observed state of Backup
type BackupStatus struct {
    // INSERT ADDITIONAL STATUS FIELDS - define observed state of cluster
    // Important: Run "make" to regenerate code after modifying this file
}

// Backup is the Schema for the backups API
type Backup struct {
    metav1.TypeMeta   `json:",inline"`
    metav1.ObjectMeta `json:"metadata,omitempty"`

    Spec   BackupSpec   `json:"spec,omitempty"`
    Status BackupStatus `json:"status,omitempty"`
}
```

### Controller Skeleton (controllers/backup_controller.go)

The controller contains the reconciliation logic:

```go
func (r *BackupReconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
    _ = log.FromContext(ctx)

    // TODO(user): your logic here

    return ctrl.Result{}, nil
}
```

### Main Entry Point (main.go)

The main.go file sets up the manager and registers controllers:

```go
func main() {
    // Manager setup
    mgr, err := ctrl.NewManager(ctrl.GetConfigOrDie(), ctrl.Options{
        Scheme:                 scheme,
        MetricsBindAddress:     metricsAddr,
        Port:                   9443,
        HealthProbeBindAddress: probeAddr,
        LeaderElection:         enableLeaderElection,
        LeaderElectionID:       "backup-operator",
    })

    // Controller registration
    if err = (&controllers.BackupReconciler{
        Client: mgr.GetClient(),
        Scheme: mgr.GetScheme(),
    }).SetupWithManager(mgr); err != nil {
        setupLog.Error(err, "unable to create controller", "controller", "Backup")
        os.Exit(1)
    }
}
```

## Makefile Targets

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

## Hands-on Exercise: Customize the Backup Resource

Let's customize our Backup resource to make it more realistic:

### Step 1: Update the Backup Types

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

### Step 2: Generate Updated Code

After modifying the types, regenerate the code:

```bash
# Generate deepcopy methods and CRD manifests
make generate manifests

# Check what was generated
git status
```

### Step 3: Create a Sample Backup Resource

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

### Step 4: Test the CRD Installation

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

## Running the Operator

### Option 1: Run Locally (Development)

For development, you can run the operator outside the cluster:

```bash
# Run the operator locally
make run

# In another terminal, watch the logs and test
kubectl get backups --watch
```

### Option 2: Deploy to Cluster (Production-like)

For a more production-like setup:

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

## Next Steps: Implementing Controller Logic

In the next section, we'll implement the actual backup controller logic. For now, you have:

1. ✅ A working Operator SDK project structure
2. ✅ Custom Resource Definition for Backup
3. ✅ Generated controller skeleton
4. ✅ Build and deployment configuration

The controller currently doesn't do anything meaningful - it just logs that it received a reconciliation request. In the next stage, we'll implement the actual backup logic.

## Key Takeaways

1. **Operator SDK accelerates development** by providing scaffolding and best practices
2. **Custom Resources extend Kubernetes** with domain-specific APIs
3. **Generated code follows conventions** and includes necessary boilerplate
4. **Makefile provides standard targets** for common development tasks
5. **Local development is supported** for faster iteration
6. **The project structure is opinionated** but follows Kubernetes community standards

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

## Next Steps

Now that you have a working Operator SDK project, let's implement the backup controller logic:
- [04 - Backup Controller Implementation](04-Backup-Controller)

---

**Navigation:**
- **Previous:** [← Controllers](02-Controllers)
- **Next:** [Backup Controller →](04-Backup-Controller)
- **Home:** [Workshop Overview](Home)