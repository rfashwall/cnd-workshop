# Controllers and Reconciliation Loops

Controllers are the heart of Kubernetes. They implement the control loop pattern that makes Kubernetes declarative and self-healing.

## What is a Controller?

A controller is a control loop that:
1. **Watches** the desired state of resources
2. **Observes** the current state of the system
3. **Takes action** to make current state match desired state

Controllers are responsible for the "magic" that makes Kubernetes work automatically.

## The Controller Pattern

### Basic Control Loop

```
┌─────────────────┐
│  Watch Events   │
│  (API Server)   │
└─────────┬───────┘
          │
          ▼
┌─────────────────┐
│  Get Current    │
│  State          │
└─────────┬───────┘
          │
          ▼
┌─────────────────┐
│  Compare with   │
│  Desired State  │
└─────────┬───────┘
          │
          ▼
┌─────────────────┐    No    ┌─────────────────┐
│  States Match?  │─────────▶│  Take Action    │
└─────────┬───────┘          │  (Reconcile)    │
          │                  └─────────┬───────┘
          │ Yes                        │
          ▼                            │
┌─────────────────┐                    │
│  Wait for Next  │◀───────────────────┘
│  Event          │
└─────────────────┘
```

### Key Principles

1. **Level-based, not Edge-based**: Controllers care about the current state, not how it got there
2. **Idempotent**: Running the same reconciliation multiple times has the same effect
3. **Eventually Consistent**: The system will eventually reach the desired state
4. **Autonomous**: Controllers work independently without external coordination

## Built-in Controllers

Kubernetes includes many built-in controllers:

### Deployment Controller

Manages ReplicaSets to ensure the desired number of Pod replicas:

```yaml
apiVersion: apps/v1
kind: Deployment
metadata:
  name: nginx-deployment
spec:
  replicas: 3  # Desired state: 3 replicas
  selector:
    matchLabels:
      app: nginx
  template:
    metadata:
      labels:
        app: nginx
    spec:
      containers:
      - name: nginx
        image: nginx:1.21
```

**What the Deployment Controller does:**
- Watches Deployment resources
- Creates/updates ReplicaSets
- Handles rolling updates
- Manages rollbacks

### ReplicaSet Controller

Ensures the correct number of Pod replicas are running:

**Reconciliation logic:**
```
Current Pods = 2, Desired Pods = 3
→ Action: Create 1 new Pod

Current Pods = 4, Desired Pods = 3  
→ Action: Delete 1 Pod

Current Pods = 3, Desired Pods = 3
→ Action: None (desired state achieved)
```

### Node Controller

Monitors node health and manages node lifecycle:
- Marks nodes as Ready/NotReady
- Evicts Pods from unhealthy nodes
- Updates node conditions

## Custom Controllers and Operators

### What is an Operator?

An Operator is a custom controller that:
- Manages application-specific resources (Custom Resources)
- Encodes operational knowledge about an application
- Automates Day 1 (installation) and Day 2 (operations) tasks

**Operator = Custom Controller + Custom Resource + Domain Knowledge**

### Custom Resource Definitions (CRDs)

CRDs extend the Kubernetes API with new resource types:

```yaml
apiVersion: apiextensions.k8s.io/v1
kind: CustomResourceDefinition
metadata:
  name: backups.backup.example.com
spec:
  group: backup.example.com
  versions:
  - name: v1
    served: true
    storage: true
    schema:
      openAPIV3Schema:
        type: object
        properties:
          spec:
            type: object
            properties:
              source:
                type: string
              schedule:
                type: string
          status:
            type: object
            properties:
              lastBackup:
                type: string
              state:
                type: string
  scope: Namespaced
  names:
    plural: backups
    singular: backup
    kind: Backup
```

### Custom Resource Example

Once the CRD is installed, you can create custom resources:

```yaml
apiVersion: backup.example.com/v1
kind: Backup
metadata:
  name: database-backup
  namespace: production
spec:
  source: "postgresql://db:5432/myapp"
  schedule: "0 2 * * *"  # Daily at 2 AM
```

## Reconciliation in Practice

### Example: Backup Controller Logic

Let's walk through how a backup controller might work:

```go
func (r *BackupReconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
    // 1. Fetch the Backup resource
    backup := &backupv1.Backup{}
    err := r.Get(ctx, req.NamespacedName, backup)
    if err != nil {
        return ctrl.Result{}, client.IgnoreNotFound(err)
    }

    // 2. Check current state
    lastBackup := backup.Status.LastBackup
    nextScheduled := calculateNextBackup(backup.Spec.Schedule, lastBackup)
    
    // 3. Determine if action is needed
    if time.Now().After(nextScheduled) {
        // 4. Take action - perform backup
        err := r.performBackup(ctx, backup)
        if err != nil {
            return ctrl.Result{RequeueAfter: time.Minute * 5}, err
        }
        
        // 5. Update status
        backup.Status.LastBackup = time.Now().Format(time.RFC3339)
        backup.Status.State = "Completed"
        err = r.Status().Update(ctx, backup)
        if err != nil {
            return ctrl.Result{}, err
        }
    }
    
    // 6. Schedule next reconciliation
    return ctrl.Result{RequeueAfter: time.Until(nextScheduled)}, nil
}
```

### Reconciliation Triggers

Controllers are triggered by:

1. **Resource Events**: Create, Update, Delete of watched resources
2. **Periodic Reconciliation**: Scheduled re-checks (every 10 hours by default)
3. **External Events**: Webhooks, manual triggers
4. **Requeue Requests**: Controller requests re-reconciliation

## Hands-on Exercise: Observe Controller Behavior

Let's observe how controllers work in practice:

### Step 1: Create a Deployment

```bash
kubectl create namespace controller-demo
kubectl config set-context --current --namespace=controller-demo

# Create deployment
kubectl create deployment nginx --image=nginx:1.21 --replicas=3
```

### Step 2: Watch Controller Actions

In one terminal, watch events:
```bash
kubectl get events --watch
```

In another terminal, watch pods:
```bash
kubectl get pods --watch
```

### Step 3: Trigger Reconciliation

Delete a pod and watch the ReplicaSet controller recreate it:
```bash
# Get pod name
kubectl get pods

# Delete one pod
kubectl delete pod <pod-name>

# Watch the controller recreate it
```

### Step 4: Scale the Deployment

```bash
# Scale up
kubectl scale deployment nginx --replicas=5

# Scale down  
kubectl scale deployment nginx --replicas=2
```

Observe how the controller creates/deletes pods to match the desired state.

### Step 5: Simulate Node Failure

```bash
# Cordon a node (simulate failure)
kubectl get nodes
kubectl cordon <node-name>

# Watch how pods get rescheduled
kubectl get pods -o wide --watch
```

## Controller Best Practices

### 1. Idempotency

Ensure reconciliation can be run multiple times safely:

```go
// Bad: Always creates a new resource
func (r *MyReconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
    // This will fail on second run
    return r.Create(ctx, newResource)
}

// Good: Check if resource exists first
func (r *MyReconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
    existing := &v1.Pod{}
    err := r.Get(ctx, types.NamespacedName{Name: "my-pod"}, existing)
    if errors.IsNotFound(err) {
        // Create only if it doesn't exist
        return r.Create(ctx, newResource)
    }
    return ctrl.Result{}, err
}
```

### 2. Error Handling

Handle errors gracefully and provide meaningful status updates:

```go
func (r *MyReconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
    // Fetch resource
    resource := &myv1.MyResource{}
    if err := r.Get(ctx, req.NamespacedName, resource); err != nil {
        return ctrl.Result{}, client.IgnoreNotFound(err)
    }
    
    // Perform operation
    if err := r.doSomething(ctx, resource); err != nil {
        // Update status with error
        resource.Status.State = "Failed"
        resource.Status.Message = err.Error()
        r.Status().Update(ctx, resource)
        
        // Requeue with backoff
        return ctrl.Result{RequeueAfter: time.Minute * 5}, nil
    }
    
    // Success
    resource.Status.State = "Ready"
    resource.Status.Message = "Operation completed successfully"
    r.Status().Update(ctx, resource)
    
    return ctrl.Result{}, nil
}
```

### 3. Status Management

Always update resource status to reflect current state:

```go
type BackupStatus struct {
    State       string      `json:"state"`
    Message     string      `json:"message,omitempty"`
    LastBackup  metav1.Time `json:"lastBackup,omitempty"`
    NextBackup  metav1.Time `json:"nextBackup,omitempty"`
}
```

## Key Takeaways

1. **Controllers implement the Kubernetes control loop pattern**
2. **Reconciliation should be idempotent and level-based**
3. **Controllers watch for events and take action to achieve desired state**
4. **Operators are custom controllers with domain-specific knowledge**
5. **Status updates are crucial for observability**
6. **Error handling and retries are essential for reliability**

## Cleanup

```bash
kubectl delete namespace controller-demo
```

## Next Steps

Now that you understand controllers, let's learn how to build them using the Operator SDK:
- [03 - Operator SDK Introduction](03-operator-sdk.md)

---

**Navigation:**
- **Previous:** [← Kubernetes Introduction](01-intro-k8s.md)
- **Next:** [Operator SDK →](03-operator-sdk.md)
- **Home:** [Workshop Overview](../README.md)