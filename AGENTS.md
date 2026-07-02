# JobSet Operator - AI Agent Development Guide

> **For AI Agents:** This file contains critical context for understanding and working with the jobset-operator codebase. Read this fully before making changes. Pay special attention to "Never Do" and "Common Agent Mistakes" sections.

OpenShift operator that manages the deployment and lifecycle of the JobSet controller for distributed ML training and HPC workloads.

## Quick Reference

### Essential Commands
```bash
# Build
make build                    # Build operator binary
make lint                     # Run linting

# Test
make test-unit               # Run unit tests
make test-e2e                # Run e2e tests (requires OPERATOR_IMAGE and RELATED_IMAGE_OPERAND_IMAGE)
make test-e2e-operand        # Run operand e2e tests (JobSet controller tests)

# Code Generation
make generate                # Regenerate all code
make verify-codegen          # Verify codegen is up to date

# Deploy
oc apply -f deploy/ --server-side    # Deploy to cluster
oc get jobsetoperator cluster -o yaml # Check status
```

> **Note**: This lists the most commonly used commands. Consult the [Makefile](./Makefile) for additional build, test, and verification targets.

### Additional Documentation
- **[ARCHITECTURE.md](./ARCHITECTURE.md)** - System design, components, reconciliation flow
- **[CONTRIBUTING.md](./CONTRIBUTING.md)** - Development workflow, testing, debugging
- **[README.md](./README.md)** - Quick start guide

## How to Use This File

**Before making changes:**
1. Read "Overview" to understand what this operator does
2. Check "Never Do" and "Common Agent Mistakes" to avoid critical errors
3. Review "Architecture Patterns" for code structure and library-go patterns
4. Check "Quick Reference" for essential commands

**When stuck:**
1. Check "Common Workflows" for your specific task
2. Review "Code Organization" to find relevant code
3. Check "What to Ask First" to see if you need human input
4. Review "Architecture & Design Documentation" links for design context

**When implementing:**
1. Follow "Controller Pattern" for adding/modifying controllers
2. Use "What to Always Do" as a checklist
3. Run commands from "Quick Reference" or Makefile to verify changes
4. Add tests following patterns in "Testing" section

## Architecture & Design Documentation

- **[ARCHITECTURE.md](./ARCHITECTURE.md)** - Comprehensive system architecture documentation covering:
  - System architecture diagrams and data flow
  - Component architecture (operator controllers, operand, cert-manager integration)
  - Deployment topology and resource ownership
  - Reconciliation flow and controller patterns
  - Certificate management lifecycle
  - Design decisions and their rationales

- **[CONTRIBUTING.md](./CONTRIBUTING.md)** - Development workflow, PR process, and code conventions

- **[README.md](./README.md)** - Quick start: building, deploying, and running tests

- **API Documentation**: The `JobSetOperator` CRD is defined in `pkg/apis/openshiftoperator/v1/types_jobsetoperator.go` with kubebuilder markers for validation

- **Operand Repository**: [openshift/kubernetes-sigs-jobset](https://github.com/openshift/kubernetes-sigs-jobset) - OpenShift fork of the upstream JobSet controller

- **Enhancement Proposals**: See [OpenShift enhancements repo](https://github.com/openshift/enhancements) for architectural decisions affecting JobSet integration into OpenShift

- **Developer Guide**: [OpenShift Operator Development](https://github.com/openshift/enhancements/blob/master/dev-guide/operators.md) covers build/test/deploy patterns shared across OpenShift operators

## Table of Contents
- [Overview](#overview)
- [Architecture Patterns](#architecture-patterns)
- [Code Organization](#code-organization)
- [Common Workflows](#common-workflows)
- [Development Guidelines](#development-guidelines)
- [What to Always Do](#what-to-always-do)
- [What to Ask First](#what-to-ask-first)
- [What to Never Do](#what-to-never-do)

## Overview

The JobSet Operator is an OpenShift operator that deploys and manages the [JobSet controller](https://github.com/openshift/kubernetes-sigs-jobset) in OpenShift clusters. JobSet is a Kubernetes-native API for managing groups of Jobs as a unit, designed for distributed ML training and HPC workloads.

**Purpose**: Enable multi-job workloads with coordinated lifecycle management, networking, and fault recovery.

**Key Capabilities**:
- Manages multiple Jobs as a single coordinated unit
- Provides stable pod hostnames via IndexedJobs and headless services
- Intelligent failure recovery (recreates affected Jobs, not everything)
- Reduces scheduler pressure during recovery at scale
- Supports multi-role workloads (leader-worker, parameter server-worker patterns)

## Architecture Patterns

### OpenShift library-go Pattern

This operator follows the **OpenShift library-go operator pattern**:

```go
// Standard imports for library-go operators
import (
    "github.com/openshift/library-go/pkg/controller/factory"
    "github.com/openshift/library-go/pkg/operator/events"
    "github.com/openshift/library-go/pkg/operator/resource/resourceapply"
    "github.com/openshift/library-go/pkg/operator/resource/resourceread"
    "github.com/openshift/library-go/pkg/operator/v1helpers"
)
```

**Key Characteristics**:
1. **Factory-based Controllers**: Use `factory.New()` pattern from library-go
2. **Informer-driven**: React to cluster changes via Kubernetes informers
3. **Event Recording**: Use `events.Recorder` for all state changes
4. **Singleton Operator CR**: The `JobSetOperator` CR must be named `cluster`
5. **Status Conditions**: Track `Available`, `Degraded`, `Progressing` conditions

### Controller Structure

The operator has two main controllers:

#### 1. StaticResourceController
**Location**: `pkg/operator/starter.go:75-142`

Manages static resources that don't change based on cluster state:
- ServiceAccounts
- ClusterRoles/ClusterRoleBindings
- Roles/RoleBindings  
- Services

**Pattern**:
```go
staticResourceController := staticresourcecontroller.NewStaticResourceController(
    "JobSetOperatorStaticResources",
    func(name string) ([]byte, error) {
        bytes, err := bindata.Asset(name)
        // Apply transformations (namespace, ownerRefs)
        return out, nil
    },
    []string{"assets/path/to/resource.yaml", ...},
    clientHolder,
    operatorClient,
    eventRecorder,
)
```

#### 2. TargetConfigReconciler
**Location**: `pkg/operator/target_config_reconciler.go`

Manages dynamic resources and the operand deployment:
- cert-manager resources (Issuer, Certificates)
- Webhook configurations (ValidatingWebhook, MutatingWebhook)
- CRDs (with version-specific fixes)
- Prometheus monitoring (ServiceMonitor, Role, RoleBinding)
- ConfigMap and Deployment

**Reconciliation Pattern**:
```go
func (t *TargetConfigReconciler) sync(ctx context.Context, syncCtx factory.SyncContext) error {
    // 1. Check management state
    if spec.ManagementState != operatorv1.Managed {
        return nil
    }
    
    // 2. Update Available condition
    v1helpers.UpdateStatus(ctx, t.jobSetOperatorClient, 
        v1helpers.UpdateConditionFn(condition))
    
    // 3. Manage resources in dependency order
    _, _, err = t.manageWebhookService(ctx, ownerReference)
    _, _, err = t.manageCertManagerIssuer(ctx, ownerReference)
    // ... more resources
    
    // 4. Update final status
    return err
}
```

### Resource Management Pattern

**Always use library-go resource apply functions**:

```go
// For core Kubernetes resources
resourceapply.ApplyService(ctx, kubeClient.CoreV1(), eventRecorder, service)
resourceapply.ApplyDeployment(ctx, kubeClient.AppsV1(), eventRecorder, deployment, expectedGeneration)
resourceapply.ApplyConfigMap(ctx, kubeClient.CoreV1(), eventRecorder, configMap)

// For CRDs
resourceapply.ApplyCustomResourceDefinitionV1(ctx, apiextensionsClient, eventRecorder, crd)

// For webhooks
resourceapply.ApplyValidatingWebhookConfigurationImproved(ctx, kubeClient, eventRecorder, webhook, cache)

// For unstructured (cert-manager resources)
resourceapply.ApplyUnstructuredResourceImproved(ctx, dynamicClient, eventRecorder, obj, cache, gvr, nil, nil)
```

**Benefits**: Automatic diffing, event recording, and idempotent operations.

## Code Organization

```
jobset-operator/
├── cmd/jobset-operator/         # Entrypoint
│   └── main.go                  # Calls pkg/cmd/operator/cmd.go
├── pkg/
│   ├── apis/openshiftoperator/v1/   # API types
│   │   └── types_jobsetoperator.go  # JobSetOperator CRD definition
│   ├── cmd/operator/            # CLI command setup
│   │   └── cmd.go               # Cobra command configuration
│   ├── operator/                # Core operator logic
│   │   ├── starter.go           # Controller initialization
│   │   ├── target_config_reconciler.go  # Main reconciliation logic
│   │   ├── resource.go          # Resource helper functions
│   │   └── operatorclient/      # Typed client wrappers
│   ├── generated/               # Generated code (don't edit manually)
│   │   ├── clientset/           # Kubernetes client
│   │   ├── informers/           # Informers for cache
│   │   └── applyconfiguration/ # Server-side apply configs
│   └── version/                 # Version info
├── bindata/                     # Embedded YAML assets
│   └── assets/
│       ├── jobset-controller-generated/  # Upstream JobSet manifests
│       ├── jobset-controller-config/     # Controller config
│       └── jobset-controller/            # ConfigMap template
├── deploy/                      # Deployment manifests
├── test/e2e/                    # End-to-end tests
└── hack/                        # Build and code generation scripts
```

### Important Files

#### API Definition
**File**: `pkg/apis/openshiftoperator/v1/types_jobsetoperator.go`

```go
// JobSetOperator is a singleton - MUST be named "cluster"
// +kubebuilder:validation:XValidation:rule="self.metadata.name == 'cluster'",message="JobSetOperator is a singleton, .metadata.name must be 'cluster'"
type JobSetOperator struct {
    metav1.TypeMeta   `json:",inline"`
    metav1.ObjectMeta `json:"metadata,omitempty"`
    Spec   JobSetOperatorSpec   `json:"spec"`
    Status JobSetOperatorStatus `json:"status"`
}

type JobSetOperatorSpec struct {
    operatorv1.OperatorSpec `json:",inline"`
    // Inherits: ManagementState, LogLevel, OperatorLogLevel
}
```

**Management States**:
- `Managed` (default): Operator reconciles resources
- `Unmanaged`: Operator doesn't reconcile, allows manual changes
- `Removed`: Operator doesn't reconcile (kept for compatibility)

#### Operand Deployment
**File**: `pkg/operator/target_config_reconciler.go:449-491`

The deployment is managed from embedded assets at:
`bindata/assets/jobset-controller-generated/apps_v1_deployment_jobset-controller-manager.yaml`

**Image Substitution**:
```go
images := map[string]string{
    "${CONTROLLER_IMAGE}:latest": t.targetImagePullSpec,
}
```

The `OPERAND_IMAGE` env var is passed at operator startup and injected into the deployment.

**Log Level Mapping**:
```go
switch jobSetOperator.Spec.LogLevel {
case operatorv1.Normal:
    logLevel = "info"
case operatorv1.Debug, operatorv1.Trace, operatorv1.TraceAll:
    logLevel = "debug"
default:
    logLevel = "info"
}
```

## Common Workflows

### Updating Operand Version

1. **Update operand-git-ref** file:
   ```bash
   echo "v0.11.0" > operand-git-ref
   ```

2. **Regenerate manifests**:
   ```bash
   hack/update-jobset-controller-manifests.sh
   ```
   This script:
   - Clones the operand repo at the specified ref
   - Extracts manifests via `make manifests`
   - Copies them to `bindata/assets/jobset-controller-generated/`

3. **Test locally** (see CONTRIBUTING.md)

### Adding a New Managed Resource

1. **Add asset to bindata** (if new upstream resource):
   ```bash
   # Assets are synced from upstream via:
   hack/update-jobset-controller-manifests.sh
   ```

2. **For static resources** (ServiceAccounts, ClusterRoles, RoleBindings, etc. that don't depend on runtime state): Add the asset path to the `[]string` array (3rd parameter) in the `staticresourcecontroller.NewStaticResourceController()` call in `pkg/operator/starter.go` (within the `RunOperator` function). For **dynamic resources** (that depend on runtime state like certs, webhooks), create a manager function in `target_config_reconciler.go`:
   ```go
   func (t *TargetConfigReconciler) manageMyResource(ctx context.Context, ownerReference metav1.OwnerReference) (*corev1.MyResource, bool, error) {
       required := resourceread.ReadMyResourceV1OrDie(
           bindata.MustAsset("assets/path/to/resource.yaml"))
       required.Namespace = t.operatorNamespace
       required.OwnerReferences = []metav1.OwnerReference{ownerReference}
       
       // Apply transformations
       
       return resourceapply.ApplyMyResource(ctx, t.kubeClient, t.eventRecorder, required)
   }
   ```

3. **Call in sync()** in dependency order:
   ```go
   func (t *TargetConfigReconciler) sync(ctx context.Context, syncCtx factory.SyncContext) error {
       // ... existing code
       
       _, _, err = t.manageMyResource(ctx, ownerReference)
       if err != nil {
           return err
       }
       
       // ... rest of sync
   }
   ```

4. **Add informer if needed** in `NewTargetConfigReconciler`:
   ```go
   return factory.New().WithInformers(
       // ... existing informers
       kubeInformersForNamespaces.InformersFor(operatorNamespace).Core().V1().MyResources().Informer(),
   )
   ```

### Modifying the Operator API

> **Important**: Before modifying the operator API, ensure there is a corresponding enhancement proposal in [openshift/enhancements](https://github.com/openshift/enhancements) or open one. API changes require design review and approval.

1. **Edit the API type**: `pkg/apis/openshiftoperator/v1/types_jobsetoperator.go`

2. **Regenerate CRD**:
   ```bash
   make regen-crd
   ```
   
   This generates:
   - `manifests/jobset-operator.crd.yaml`
   - `deploy/00_jobset-operator.crd.yaml`

3. **Regenerate clients**:
   ```bash
   make generate-clients
   ```
   
   Updates:
   - `pkg/generated/clientset/`
   - `pkg/generated/informers/`
   - `pkg/generated/applyconfiguration/`

4. **Update reconciler logic** to handle new fields

5. **Verify**:
   ```bash
   make verify-codegen
   ```

### Handling cert-manager Resources

The operator uses **cert-manager** for TLS certificates:

**Resources Created**:
1. **Issuer**: `jobset-selfsigned-issuer` (self-signed)
2. **Certificates**:
   - `jobset-serving-cert` → secret `webhook-server-cert`
   - `jobset-metrics-cert` → secret `metrics-server-cert`

**Pattern for Unstructured Resources**:
```go
gvr := schema.GroupVersionResource{
    Group:    "cert-manager.io",
    Version:  "v1",
    Resource: "certificates",
}

// Use ReadGenericWithUnstructured unless a type-specific function exists in resourceread
cert, err := resourceread.ReadGenericWithUnstructured(
    bindata.MustAsset("assets/..."))
certAsUnstructured, ok := cert.(*unstructured.Unstructured)

// Set namespace and ownerRefs
certAsUnstructured.SetNamespace(t.operatorNamespace)
certAsUnstructured.SetOwnerReferences([]metav1.OwnerReference{ownerReference})

return resourceapply.ApplyUnstructuredResourceImproved(
    ctx, t.dynamicClient, t.eventRecorder, certAsUnstructured, 
    t.resourceCache, gvr, nil, nil)
```

**Wait for Secrets**:
```go
secret, _, err := t.checkSecretReady(WebhookCertificateSecretName)
if err != nil {
    return err  // Requeue until cert-manager creates the secret
}
```

**Inject CA into Webhooks/CRDs**:
```go
func injectCertManagerCA(obj metav1.Object, namespace string) error {
    annotations := obj.GetAnnotations()
    injectAnnotation := annotations[CertManagerInjectCaAnnotation]
    injectAnnotation = strings.Replace(injectAnnotation, 
        "$(CERTIFICATE_NAMESPACE)", namespace, 1)
    annotations[CertManagerInjectCaAnnotation] = injectAnnotation
    obj.SetAnnotations(annotations)
    return nil
}
```

## Development Guidelines

### What to Always Do

#### 1. Use library-go Patterns
```go
// ✅ GOOD: Use library-go resource apply
resourceapply.ApplyDeployment(ctx, kubeClient, eventRecorder, deployment, expectedGeneration)

// ❌ BAD: Direct client calls
_, err := kubeClient.AppsV1().Deployments(ns).Update(ctx, deployment, metav1.UpdateOptions{})
```

#### 2. Record Events
```go
// ✅ GOOD: Events are automatically recorded by resourceapply functions
resourceapply.ApplyService(ctx, kubeClient.CoreV1(), t.eventRecorder, service)

// For custom logic, record explicitly:
t.eventRecorder.Event("ServiceCreated", "Created webhook service")
```

#### 3. Set Owner References
```go
ownerReference := metav1.OwnerReference{
    APIVersion: "operator.openshift.io/v1",
    Kind:       "JobSetOperator",
    Name:       jobSetOperator.Name,
    UID:        jobSetOperator.UID,
}
resource.OwnerReferences = []metav1.OwnerReference{ownerReference}
```

**Why**: Ensures cascading deletion when the operator CR is deleted.

#### 4. Update Status Conditions
```go
_, _, err = v1helpers.UpdateStatus(ctx, t.jobSetOperatorClient, 
    v1helpers.UpdateConditionFn(operatorv1.OperatorCondition{
        Type:    operatorv1.OperatorStatusTypeAvailable,
        Status:  operatorv1.ConditionTrue,
        Reason:  "AsExpected",
        Message: "All operand pods are running",
    }))
```

**Required Conditions**:
- `Available`: Operand is running and ready
- `Degraded`: Operator encountered an error
- `Progressing`: Rollout in progress

#### 5. Check Management State
```go
spec, _, _, err := t.jobSetOperatorClient.GetOperatorState()
if spec.ManagementState != operatorv1.Managed {
    return nil  // Don't reconcile
}
```

#### 6. Handle Dependencies
Check for cert-manager before creating resources:
```go
found, err := isResourceRegistered(t.discoveryClient, schema.GroupVersionKind{
    Group:   "cert-manager.io",
    Version: "v1",
    Kind:    "Issuer",
})
if !found {
    return fmt.Errorf("please make sure that cert-manager is installed")
}
```

#### 7. Namespace Injection
```go
// For namespaced resources
resource.Namespace = t.operatorNamespace

// For role bindings
for i := range roleBinding.Subjects {
    roleBinding.Subjects[i].Namespace = t.operatorNamespace
}
```

#### 8. Version-Specific Fixes
```go
func customCRDfixes(crd *apiextensionsv1.CustomResourceDefinition, serverVersion *version.Version) {
    if serverVersion.Major() == 1 && serverVersion.Minor() <= 31 {
        // Apply workaround for k8s 1.31 and earlier
    }
}
```

### What to Ask First

#### 1. Before Adding New API Fields
**Ask**: "Should this be in the operator spec or a separate CR?"
- Operator spec: Configuration affecting the operator itself (log level, management state)
- Operand configuration: Usually passed via ConfigMap or separate CR

#### 2. Before Changing Resource Apply Order
**Ask**: "What are the dependencies?"

Current order in `sync()`:
1. WebhookService (needed for cert DNS names)
2. cert-manager Issuer
3. cert-manager Certificates
4. Wait for Secrets (certs must be ready)
5. Webhooks (need CA from certs)
6. CRD (needs CA from certs)
7. Prometheus resources
8. ConfigMap
9. Deployment (needs all above)

#### 3. Before Modifying Upstream Assets
**Ask**: "Should this be upstreamed to kubernetes-sigs/jobset?"
- Asset files in `bindata/assets/jobset-controller-generated/` come from upstream and **must not be manually edited**
- Any manual modifications will be overwritten by `hack/update-jobset-controller-manifests.sh`
- Transformations should happen in the reconciler code, not in asset files

#### 4. Before Changing the Operator Namespace
**Ask**: "Is this change compatible with existing clusters?"
- Default namespace: `openshift-jobset-operator`
- Changing requires migration strategy
- See `starter.go:37-40` for namespace fallback logic

#### 5. Before Adding New Dependencies
**Ask**: "Is this in library-go or do we need to vendor?"
- Prefer library-go patterns over custom implementations
- Check `go.mod` for existing dependencies
- Large dependencies should be justified

### What to Never Do

#### 1. Never Edit Generated Code
Files with `// Code generated` headers are auto-generated:
```
pkg/generated/
pkg/apis/*/zz_generated.*.go
bindata/assets.go
bindata/assets/jobset-controller-generated/
```

**Instead**: Run the appropriate generator:
```bash
make generate-clients  # For clientset/informers
make regen-crd         # For CRD
make generate          # For all codegen
hack/update-jobset-controller-manifests.sh  # For upstream assets
```

#### 2. Never Skip Owner References
```go
// ❌ BAD: Resource won't be cleaned up
resource.OwnerReferences = nil

// ✅ GOOD: Always set owner reference
resource.OwnerReferences = []metav1.OwnerReference{ownerReference}
```

#### 3. Never Use Hard-Coded Namespaces
```go
// ❌ BAD: Hard-coded namespace
namespace := "openshift-jobset-operator"

// ✅ GOOD: Use the configured namespace
namespace := t.operatorNamespace
```

**Exception**: `openshift-monitoring` for Prometheus integration (external namespace).

#### 4. Never Block on External Services
```go
// ❌ BAD: Blocking wait
for !isCertReady() {
    time.Sleep(1 * time.Second)
}

// ✅ GOOD: Return error and let informer re-trigger
secret, _, err := t.checkSecretReady(secretName)
if err != nil {
    return err  // Controller will retry
}
```

#### 5. Never Modify Operand Configuration Directly
```go
// ❌ BAD: Directly editing JobSet controller config
configMap.Data["config.yaml"] = customConfig

// ✅ GOOD: Use the upstream config and let users customize via JobSetOperator spec
// Then transform in the reconciler if needed
```

The operand (JobSet controller) configuration comes from upstream and should only be modified if:
- Required for OpenShift integration
- Bug fix until upstream release
- Security requirement

#### 6. Never Ignore Error Returns
```go
// ❌ BAD: Ignoring errors
_, _, _ = t.manageWebhookService(ctx, ownerReference)

// ✅ GOOD: Propagate errors
_, _, err := t.manageWebhookService(ctx, ownerReference)
if err != nil {
    return err
}
```

#### 7. Never Use Non-Idempotent Operations
```go
// ❌ BAD: Create without checking existence
_, err := kubeClient.CoreV1().Services(ns).Create(ctx, service, metav1.CreateOptions{})

// ✅ GOOD: Use Apply (idempotent)
resourceapply.ApplyService(ctx, kubeClient.CoreV1(), eventRecorder, service)
```

#### 8. Never Skip Status Updates

**Degraded Condition**: Automatically handled by library-go's `WithSyncDegradedOnError()` pattern:
```go
// Controller setup (in NewTargetConfigReconciler)
return factory.New().
    WithInformers(...).
    WithSyncDegradedOnError(jobSetOperatorClient).  // Automatically sets Degraded=True on error
    WithSync(t.sync).
    ToController("TargetConfigController", eventRecorder)

// In sync function - just return errors
func (t *TargetConfigReconciler) sync(ctx context.Context, syncCtx factory.SyncContext) error {
    if err != nil {
        return err  // Degraded=True set automatically
    }
}
```

**Available Condition**: Must be explicitly updated:
```go
// ✅ GOOD: Always update Available condition
_, _, err := v1helpers.UpdateStatus(ctx, t.jobSetOperatorClient, 
    v1helpers.UpdateConditionFn(constructAvailableCondition(err, deployment)))
```

**For custom Degraded reasons**: Explicitly set before returning:
```go
// Only needed when you want a specific Reason/Message
_, _, err = v1helpers.UpdateStatus(ctx, client, v1helpers.UpdateConditionFn(
    operatorv1.OperatorCondition{
        Type:    operatorv1.OperatorStatusTypeDegraded,
        Status:  operatorv1.ConditionTrue,
        Reason:  "MissingDependency",
        Message: "cert-manager must be installed",
    }))
return err
```

#### 9. Never Create Multiple JobSetOperator CRs
The API enforces a singleton pattern:
```go
// +kubebuilder:validation:XValidation:rule="self.metadata.name == 'cluster'",message="JobSetOperator is a singleton, .metadata.name must be 'cluster'"
```

Only one `JobSetOperator` CR named `cluster` should exist.

#### 10. Never Assume cert-manager is Installed
```go
// ✅ GOOD: Always check first
found, err := isResourceRegistered(t.discoveryClient, ...)
if !found {
    return fmt.Errorf("cert-manager must be installed")
}
```

cert-manager is a prerequisite but not bundled with the operator.

## Common Agent Mistakes to Avoid

These are specific mistakes AI agents frequently make in this codebase:

- **Don't suggest `client.Get()` or `client.List()` in sync functions** - Use informers and listers instead. Direct API calls in controller sync loops cause performance issues and are against OpenShift library-go patterns. Always use the cached listers from informers.

- **Don't propose adding controllers without mentioning `pkg/operator/starter.go`** - All controllers must be registered in the starter's `RunOperator()` function, or they won't run. Simply creating a controller file isn't enough.

- **Don't generate code changes without verifying** - Always run `make verify`, `make verify-codegen`, and `make verify-controller-manifests` before suggesting code is complete. CI will fail if verification doesn't pass locally first.

- **Don't recommend modifying YAML in `bindata/assets/jobset-controller-generated/` without explaining the regeneration process** - These files come from upstream and are overwritten by `hack/update-jobset-controller-manifests.sh`. Explain that changes should be made in the reconciler logic or upstream, not directly in these assets.

- **Don't propose changes to RBAC without updating `deploy/` manifests** - Operator permissions must be declared in deploy manifest files. Code changes alone won't grant new permissions.

- **Don't use table-driven tests without the `t.Run()` pattern** - All tests should use subtests for better failure isolation and reporting:
  ```go
  for _, tt := range tests {
      t.Run(tt.name, func(t *testing.T) {
          // Test implementation
      })
  }
  ```

- **Don't assume single namespace** - This operator manages resources across multiple namespaces (`openshift-jobset-operator` for operator, same namespace for operand). Be explicit about which namespace resources belong to.

- **Don't suggest using `resourcemerge.SetDeploymentGeneration()` without updating status** - Generation tracking must be paired with status updates:
  ```go
  v1helpers.UpdateStatus(ctx, client, func(status *operatorv1.OperatorStatus) error {
      resourcemerge.SetDeploymentGeneration(&status.Generations, deployment)
      return nil
  })
  ```

- **Don't recommend cert-manager changes without security review** - Certificate management is security-critical. Flag any cert-manager modifications for human security review.

- **Don't ignore the singleton pattern** - The `JobSetOperator` CR must be named "cluster". Don't suggest creating CRs with other names or supporting multiple instances.

## Tech Stack

- **Go 1.25+** - Check `go.mod` for the exact version required
- **Kubernetes client-go** - Standard Kubernetes client library
- **OpenShift library-go** - Controller factory, resourceapply, v1helpers, operators
- **OpenShift api** - `operator.openshift.io/v1` for JobSetOperator CRD
- **cert-manager.io/v1** - Certificate management (runtime dependency, not vendored)
- **k8s.io/apiextensions-apiserver** - CRD management
- **k8s.io/klog/v2** - Structured logging
- **github.com/onsi/ginkgo/v2** - BDD-style testing framework
- **github.com/onsi/gomega** - Matcher library for tests

**Key Patterns**:
- **library-go factory pattern** - All controllers use `factory.New()`
- **Informer-based** - No direct API calls in sync loops
- **Declarative apply** - Use `resourceapply.*` functions
- **Operator lifecycle** - Follows OpenShift operator conventions

## Namespaces

The JobSet Operator manages resources in a single primary namespace:

- **`openshift-jobset-operator`** - Operator and operand both run here
  - Operator deployment: `jobset-operator`
  - Operand deployment: `jobset-controller-manager`
  - Certificates, secrets, services, RBAC resources
  - JobSetOperator CR (cluster-scoped, but operator watches from this namespace)

**Cluster-Scoped Resources**:
- `JobSetOperator` CR (name must be "cluster")
- `jobsets.jobset.x-k8s.io` CRD
- ClusterRoles and ClusterRoleBindings for operator and operand
- ValidatingWebhookConfiguration and MutatingWebhookConfiguration

**Cross-Namespace Access**:
- **`openshift-monitoring`** - Prometheus scrapes metrics from operator namespace
  - Role and RoleBinding grant Prometheus ServiceAccount access
  - ServiceMonitor configures scraping with mTLS

**Important**: Unlike multi-namespace operators (e.g., authentication-operator), JobSet keeps everything in one namespace for simplicity. The operand manages JobSets cluster-wide from this single namespace.

## Security Notes

- **cert-manager Dependency** - This operator requires cert-manager for TLS certificates. The operator checks for cert-manager at startup and enters Degraded state if missing.
- **Webhook Security** - ValidatingWebhook and MutatingWebhook use TLS with certificates from cert-manager. CA bundles are automatically injected via annotations.
- **Metrics Security** - Metrics endpoint uses mTLS with certificates from cert-manager. Prometheus must present valid client certificates.
- **RBAC Principle of Least Privilege** - Operator has minimal permissions needed. Operand has permissions to manage Jobs, Services, and related resources cluster-wide.
- **No Secrets in Logs** - Operator does not log certificate contents or sensitive configuration.
- **Owner References for Cleanup** - All created resources have owner references to ensure proper cascading deletion and prevent orphaned resources.

**Security-Critical Code**:
- `pkg/operator/target_config_reconciler.go` - Certificate and webhook management
- `deploy/*.yaml` - RBAC definitions
- `bindata/assets/jobset-controller-generated/*webhook*` - Webhook configurations

**Never**:
- Skip certificate validation
- Log sensitive data (certs, tokens, secrets)
- Grant more permissions than needed
- Disable webhook verification

## Testing

### Unit Tests
Location: `pkg/` (co-located with code)

**Pattern**: Not extensively used in this operator (relies on e2e).

### E2E Tests
Location: `test/e2e/e2e_test.go`

**Test Scenarios**:
1. **Condition Verification**: Checks Available/Degraded conditions
2. **Pod Recovery**: Deletes operand pod, verifies recreation
3. **Management State Unmanaged**: Allows manual scaling
4. **Management State Removed**: Allows manual scaling (compatibility test)

**Running**:
```bash
export OPERATOR_IMAGE=quay.io/user/jobset-operator:tag
export RELATED_IMAGE_OPERAND_IMAGE=quay.io/user/jobset:tag
make test-e2e           # Operator e2e tests
make test-e2e-operand   # Operand e2e tests (JobSet controller)
```

### Operand Tests
The operand (JobSet controller) has its own test suite in the upstream repo.

**Running Operand Tests**:
```bash
make test-e2e-operand
```

This runs the upstream JobSet test suite against the deployed controller.

## Constants Reference

```go
// Namespaces
operatorNamespace = "openshift-jobset-operator"
OpenshiftMonitoringNamespace = "openshift-monitoring"

// Secrets
WebhookCertificateSecretName = "webhook-server-cert"
MetricsCertificateSecretName = "metrics-server-cert"
WebhookCertificateName = "jobset-serving-cert"

// Labels
operandLabel = "control-plane=controller-manager"

// Names
OperandName = "jobset-controller-manager"
OperatorConfigName = "cluster"

// Annotations
CertManagerInjectCaAnnotation = "cert-manager.io/inject-ca-from"
```

## Version Information

**Current Versions** (from README.md):
- Operator 1.0.0: JobSet 0.11.0, OCP 4.18-4.20, k8s 1.35, Go 1.25

**Tracking**:
- `operand-git-ref` file: Upstream JobSet version/commit
- `OS_GIT_VERSION`: Set by OpenShift ART build system

## Additional Resources

- [JobSet Official Docs](https://jobset.sigs.k8s.io/)
- [Kubernetes JobSet Blog](https://kubernetes.io/blog/2025/03/23/introducing-jobset/)
- [OpenShift library-go](https://github.com/openshift/library-go)
- [cert-manager](https://cert-manager.io/)

## Questions?

- **Slack**: #forum-ocp-workloads (OpenShift internal)
- **Component**: jobset-operator (see OWNERS file)
- **Approvers**: control-plane-approvers

---

**Document Version**: 1.0  
**Last Updated**: 2026-06-23  
**Maintenance**: Update this document when making significant architectural changes, adding new workflows, or when patterns change. Include documentation updates in the same PR as code changes.
