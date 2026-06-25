# JobSet Operator - Architecture

This document explains the architecture, design decisions, and operational model of the JobSet Operator.

## Table of Contents
- [Overview](#overview)
- [System Architecture](#system-architecture)
- [Component Architecture](#component-architecture)
- [Deployment Topology](#deployment-topology)
- [Resource Management](#resource-management)
- [Reconciliation Flow](#reconciliation-flow)
- [Certificate Management](#certificate-management)
- [Monitoring and Observability](#monitoring-and-observability)
- [Design Decisions](#design-decisions)
- [Failure Modes and Recovery](#failure-modes-and-recovery)

## Overview

### What is JobSet?

**JobSet** is a Kubernetes-native API for managing groups of Jobs as a coordinated unit. It's designed for distributed ML training and HPC (High-Performance Computing) workloads where multiple jobs with different roles need to work together.

**Key Features**:
- **Multi-Job Coordination**: Manages multiple Jobs (e.g., leader-worker, parameter server-worker)
- **Stable Networking**: Uses IndexedJobs and headless services for stable pod hostnames
- **Intelligent Failure Recovery**: Recreates only affected Jobs instead of restarting everything
- **Scheduler-Friendly**: Reduces pressure on the Kubernetes scheduler during recovery at scale
- **Lifecycle Management**: Coordinates creation, networking, and teardown of related Jobs

**Example Use Cases**:
- Distributed machine learning training (parameter server + workers)
- MapReduce-style workloads (mapper + reducer jobs)
- Leader-follower patterns in HPC
- Multi-stage batch processing pipelines

### What is the JobSet Operator?

The **JobSet Operator** is an OpenShift operator that:
1. Deploys the JobSet controller (operand) into OpenShift clusters
2. Manages the lifecycle of the JobSet controller
3. Configures webhooks, RBAC, and monitoring for JobSet
4. Provides an OpenShift-native API (`JobSetOperator` CR) for configuration

**Operator vs. Operand**:
- **Operator** (this project): Manages the deployment of the JobSet controller
- **Operand**: The JobSet controller itself (from [openshift/kubernetes-sigs-jobset](https://github.com/openshift/kubernetes-sigs-jobset))

## System Architecture

### High-Level Architecture

```
┌─────────────────────────────────────────────────────────────────┐
│                     OpenShift Cluster                            │
│                                                                   │
│  ┌────────────────────────────────────────────────────────────┐ │
│  │  openshift-jobset-operator namespace                       │ │
│  │                                                              │ │
│  │  ┌──────────────────┐         ┌────────────────────────┐   │ │
│  │  │ JobSet Operator  │────────▶│ JobSet Controller      │   │ │
│  │  │   (this repo)    │ manages │   (operand)            │   │ │
│  │  └──────────────────┘         └────────────────────────┘   │ │
│  │         │                               │                   │ │
│  │         │ watches                       │ watches           │ │
│  │         ▼                               ▼                   │ │
│  │  ┌──────────────────┐         ┌────────────────────────┐   │ │
│  │  │ JobSetOperator CR│         │  JobSet CRs            │   │ │
│  │  │  (name: cluster) │         │  (user workloads)      │   │ │
│  │  └──────────────────┘         └────────────────────────┘   │ │
│  │                                         │                   │ │
│  └─────────────────────────────────────────┼───────────────────┘ │
│                                            │                     │
│  ┌─────────────────────────────────────────▼───────────────────┐ │
│  │  User Namespaces                                             │ │
│  │                                                               │ │
│  │  ┌──────────┐  ┌──────────┐  ┌──────────┐  ┌──────────┐    │ │
│  │  │  Job 1   │  │  Job 2   │  │  Job 3   │  │  Job N   │    │ │
│  │  │ (leader) │  │ (worker) │  │ (worker) │  │ (worker) │    │ │
│  │  └──────────┘  └──────────┘  └──────────┘  └──────────┘    │ │
│  │       │              │              │              │         │ │
│  │       └──────────────┴──────────────┴──────────────┘         │ │
│  │              Coordinated by JobSet                           │ │
│  └───────────────────────────────────────────────────────────────┘ │
│                                                                   │
│  ┌───────────────────────────────────────────────────────────┐   │
│  │  cert-manager (prerequisite)                              │   │
│  │  - Issues certificates for webhooks and metrics           │   │
│  └───────────────────────────────────────────────────────────┘   │
│                                                                   │
│  ┌───────────────────────────────────────────────────────────┐   │
│  │  openshift-monitoring namespace                           │   │
│  │  - Prometheus scrapes JobSet controller metrics           │   │
│  └───────────────────────────────────────────────────────────┘   │
└─────────────────────────────────────────────────────────────────┘
```

### Data Flow

```
User creates/updates JobSetOperator CR
         │
         ▼
┌─────────────────────┐
│  Operator Informer  │ (watches JobSetOperator CR)
└─────────────────────┘
         │
         ▼
┌─────────────────────┐
│  Reconcile Loop     │
└─────────────────────┘
         │
         ├──▶ ManagementState check
         │    └──▶ If not "Managed", skip reconciliation
         │
         ├──▶ Check cert-manager installed
         │    └──▶ If not, set Degraded condition
         │
         ├──▶ Reconcile resources in order:
         │    1. Webhook Service
         │    2. cert-manager Issuer
         │    3. cert-manager Certificates
         │    4. Wait for certificate secrets ready
         │    5. Webhooks (Validating/Mutating)
         │    6. JobSet CRD
         │    7. Prometheus RBAC (Role/RoleBinding)
         │    8. ServiceMonitor
         │    9. ConfigMap (controller config)
         │    10. Deployment (JobSet controller)
         │
         └──▶ Update status conditions
              ├──▶ Available (based on deployment readiness)
              ├──▶ Degraded (based on errors)
              └──▶ Progressing (based on generation)
```

## Component Architecture

### Operator Components

#### 1. Main Entrypoint
**File**: `cmd/jobset-operator/main.go`

Simple wrapper that calls the operator command setup.

#### 2. Operator Command
**File**: `pkg/cmd/operator/cmd.go`

Sets up the Cobra command and controller context:
- Configures kubeconfig and namespace
- Sets up signal handling
- Calls `RunOperator` from `pkg/operator/starter.go`

#### 3. Operator Starter
**File**: `pkg/operator/starter.go`

Initializes and starts controllers:

```go
func RunOperator(ctx context.Context, cc *controllercmd.ControllerContext) error {
    // 1. Create Kubernetes clients
    kubeClient, dynamicClient, apiextensionsClient, discoveryClient
    
    // 2. Create operator-specific client and informers
    operatorConfigClient, operatorConfigInformers
    
    // 3. Create JobSetOperatorClient wrapper
    jobSetOperatorClient
    
    // 4. Start StaticResourceController
    //    - Manages RBAC and Services that don't change
    
    // 5. Start TargetConfigReconciler
    //    - Manages dynamic resources and deployment
    
    // 6. Start LogLevelController
    //    - Syncs logLevel from CR to deployment
    
    // 7. Start all informers and controllers
}
```

**Controllers Started**:
1. **StaticResourceController**: Manages static RBAC and service resources
2. **TargetConfigReconciler**: Manages operand deployment and dependencies
3. **LogLevelController**: Syncs log level configuration

#### 4. Static Resource Controller
**Purpose**: Manage resources that are static and don't depend on cluster state.

**Resources Managed** (from `bindata/assets/jobset-controller-generated/`):
- ServiceAccount: `jobset-controller-manager`
- ClusterRoles: `jobset-manager-role`, `jobset-metrics-reader`, `jobset-proxy-role`
- ClusterRoleBindings: Link service account to cluster roles
- Roles: `jobset-leader-election-role`, `jobset-manager-secrets-role`
- RoleBindings: Link service account to namespaced roles
- Service: `jobset-controller-manager-metrics-service`

**Pattern**:
```go
staticresourcecontroller.NewStaticResourceController(
    name,
    assetFunc,      // Loads and transforms assets
    assetPaths,     // List of YAML files to manage
    clientHolder,
    operatorClient,
    eventRecorder,
)
```

**Transformation Logic**:
1. Load asset from bindata
2. Set owner reference to JobSetOperator CR
3. Set namespace to operator namespace
4. For RoleBindings: Update subject namespaces

#### 5. Target Config Reconciler
**Purpose**: Manage dynamic resources and the operand deployment.

**Key Responsibilities**:
1. Check cert-manager is installed
2. Create cert-manager resources (Issuer, Certificates)
3. Wait for certificates to be ready
4. Configure webhooks with CA bundles
5. Manage the JobSet CRD
6. Set up Prometheus monitoring
7. Deploy the JobSet controller

**Reconciliation Phases**:

```go
func (t *TargetConfigReconciler) sync(ctx, syncCtx) error {
    // Phase 1: Pre-checks
    // - Check management state
    // - Update availability condition based on deployment
    
    // Phase 2: Dependency checks
    // - Verify cert-manager is installed
    // - Get server version for CRD fixes
    
    // Phase 3: Certificate setup
    // - Create webhook service (needed for cert DNS names)
    // - Create cert-manager Issuer (self-signed)
    // - Create cert-manager Certificates
    // - Wait for secrets to be populated
    
    // Phase 4: Webhook configuration
    // - Apply ValidatingWebhookConfiguration
    // - Apply MutatingWebhookConfiguration
    // - Apply JobSet CRD (with webhook config)
    
    // Phase 5: Monitoring setup
    // - Create Prometheus Role
    // - Create Prometheus RoleBinding
    // - Create ServiceMonitor
    
    // Phase 6: Controller deployment
    // - Create ConfigMap (controller config)
    // - Create/update Deployment
    //   - Inject operand image
    //   - Configure log level
    //   - Add annotations to trigger rollout on config changes
    
    // Phase 7: Status update
    // - Set deployment generation in status
    // - Update readyReplicas count
    // - Clear degraded condition (if no errors)
}
```

### Operand (JobSet Controller)

The JobSet controller is a separate component from [openshift/kubernetes-sigs-jobset](https://github.com/openshift/kubernetes-sigs-jobset).

**Responsibilities**:
1. Watch JobSet CRs
2. Create child Jobs based on JobSet spec
3. Create headless Services for DNS resolution
4. Manage failure recovery (recreate only failed Jobs)
5. Handle JobSet lifecycle (suspend, resume, delete)

**Architecture** (simplified):
```
JobSet Controller
    │
    ├──▶ Reconcile Loop
    │    ├──▶ Create/update child Jobs
    │    ├──▶ Create headless Service
    │    ├──▶ Monitor Job status
    │    └──▶ Handle failures
    │
    ├──▶ Webhooks
    │    ├──▶ ValidatingWebhook (validate JobSet spec)
    │    └──▶ MutatingWebhook (set defaults)
    │
    └──▶ Metrics
         └──▶ Expose Prometheus metrics
```

## Deployment Topology

### Namespace Layout

```
openshift-jobset-operator/
├── Operator Resources
│   ├── Deployment: jobset-operator
│   ├── ServiceAccount: jobset-operator
│   └── ConfigMaps, Secrets (operator config)
│
├── Operand Resources
│   ├── Deployment: jobset-controller-manager
│   ├── ServiceAccount: jobset-controller-manager
│   ├── Service: jobset-webhook-service
│   ├── Service: jobset-controller-manager-metrics-service
│   └── Secrets: webhook-server-cert, metrics-server-cert
│
└── Monitoring Resources
    ├── Role: jobset-prometheus-k8s
    ├── RoleBinding: jobset-prometheus-k8s
    └── ServiceMonitor: jobset-controller-manager-metrics-monitor

Cluster-scoped Resources
├── CRDs
│   ├── jobsetoperators.operator.openshift.io
│   └── jobsets.jobset.x-k8s.io
│
├── ClusterRoles
│   ├── jobset-operator (operator RBAC)
│   └── jobset-manager-role (operand RBAC)
│
├── ClusterRoleBindings
│   ├── jobset-operator
│   └── jobset-manager-rolebinding
│
└── WebhookConfigurations
    ├── jobset-validating-webhook-configuration
    └── jobset-mutating-webhook-configuration

cert-manager Resources (in operator namespace)
├── Issuer: jobset-selfsigned-issuer
├── Certificate: jobset-serving-cert
└── Certificate: jobset-metrics-cert
```

### Singleton Pattern

The `JobSetOperator` CR uses a **singleton pattern**:

```yaml
apiVersion: operator.openshift.io/v1
kind: JobSetOperator
metadata:
  name: cluster  # MUST be named "cluster"
spec:
  managementState: Managed
  logLevel: Normal
```

**Enforcement**:
```go
// +kubebuilder:validation:XValidation:rule="self.metadata.name == 'cluster'",message="JobSetOperator is a singleton, .metadata.name must be 'cluster'"
```

**Why Singleton?**
- OpenShift convention for cluster-scoped operators
- Prevents confusion about which CR controls the operand
- Simplifies status reporting (single source of truth)

### Resource Ownership

**Owner References**:
All resources created by the operator have an owner reference pointing to the `JobSetOperator` CR:

```yaml
ownerReferences:
- apiVersion: operator.openshift.io/v1
  kind: JobSetOperator
  name: cluster
  uid: <uid-of-jobsetoperator-cr>
```

**Benefits**:
- **Cascading deletion**: Deleting the JobSetOperator CR deletes all managed resources
- **Clear ownership**: Easy to see which operator owns a resource
- **Garbage collection**: Kubernetes automatically cleans up orphaned resources

**Exception**: ClusterRoles and ClusterRoleBindings don't have owner references (cluster-scoped resources can't be owned by namespaced resources). These are managed directly by the operator.

## Resource Management

### Asset Management

**Embedded Assets**: All YAML manifests are embedded in the operator binary using `bindata`.

**Location**: `bindata/assets/`

**Structure**:
```
bindata/assets/
├── jobset-controller-generated/  # From upstream JobSet repo
│   ├── apps_v1_deployment_jobset-controller-manager.yaml
│   ├── apiextensions.k8s.io_v1_customresourcedefinition_jobsets.jobset.x-k8s.io.yaml
│   ├── admissionregistration.k8s.io_v1_validatingwebhookconfiguration_*.yaml
│   ├── admissionregistration.k8s.io_v1_mutatingwebhookconfiguration_*.yaml
│   ├── cert-manager.io_v1_issuer_*.yaml
│   ├── cert-manager.io_v1_certificate_*.yaml
│   ├── rbac.authorization.k8s.io_v1_*.yaml
│   ├── v1_service_*.yaml
│   └── monitoring.coreos.com_v1_servicemonitor_*.yaml
│
├── jobset-controller-config/  # Controller configuration
│   └── config.yaml  # Controller manager config
│
└── jobset-controller/  # ConfigMap template
    └── config.yaml
```

**Update Process**:
```bash
# Update operand version
echo "v0.11.0" > operand-git-ref

# Regenerate manifests from upstream
hack/update-jobset-controller-manifests.sh

# This script:
# 1. Clones openshift/kubernetes-sigs-jobset at the version in operand-git-ref
# 2. Runs `make manifests` in the cloned repo
# 3. Copies manifests to bindata/assets/jobset-controller-generated/
```

### Image Management

**Operator Image**:
- Built from `Dockerfile.ci`
- Specified in `deploy/12_deployment.yaml`
- Example: `registry.ci.openshift.org/ocp/4.20:jobset-operator`

**Operand Image**:
- Built from OpenShift fork `openshift/kubernetes-sigs-jobset`
- Passed to operator via `OPERAND_IMAGE` environment variable
- Injected into operand deployment by the reconciler

**Image Substitution**:
```go
// In target_config_reconciler.go:manageDeployment()
images := map[string]string{
    "${CONTROLLER_IMAGE}:latest": t.targetImagePullSpec,
}
for i := range required.Spec.Template.Spec.Containers {
    for pat, img := range images {
        if required.Spec.Template.Spec.Containers[i].Image == pat {
            required.Spec.Template.Spec.Containers[i].Image = img
        }
    }
}
```

## Reconciliation Flow

### Informer-Based Reconciliation

The operator uses Kubernetes **informers** (watch caches) to trigger reconciliation:

```
Kubernetes API Server
    │
    │ (watch)
    ▼
Informer Cache
    │
    │ (event: add/update/delete)
    ▼
Work Queue
    │
    │ (dequeue)
    ▼
Reconcile Function
```

**Informers Registered**:
1. `JobSetOperator` CR informer (triggers on operator config changes)
2. `Deployment` informer (triggers on operand deployment changes)
3. `ConfigMap` informer (triggers on config changes)
4. `Secret` informer (triggers on certificate changes)

**Resync Interval**: 5 minutes (even if no changes)

### Reconciliation Loop

```go
func (t *TargetConfigReconciler) sync(ctx, syncCtx) error {
    // Step 1: Get current operator state
    spec, status, generation, err := t.jobSetOperatorClient.GetOperatorState()
    
    // Step 2: Check management state
    if spec.ManagementState != operatorv1.Managed {
        return nil  // Don't reconcile
    }
    
    // Step 3: Update availability condition
    deployment, _ := t.deploymentsLister.Deployments(ns).Get(operandName)
    v1helpers.UpdateStatus(ctx, client, UpdateConditionFn(
        constructAvailableCondition(deployment)))
    
    // Step 4: Check prerequisites
    if !isCertManagerInstalled() {
        setDegradedCondition("MissingDependency", 
            "cert-manager must be installed")
        return err
    }
    
    // Step 5: Reconcile resources
    manageWebhookService()
    manageCertManagerIssuer()
    manageCertManagerCertificate()
    checkSecretReady()  // Wait for certs
    manageValidatingWebhook()
    manageMutatingWebhook()
    manageCRD()
    managePrometheusResources()
    manageConfigMap()
    manageDeployment()
    
    // Step 6: Update status
    v1helpers.UpdateStatus(ctx, client,
        func(status *operatorv1.OperatorStatus) error {
            resourcemerge.SetDeploymentGeneration(&status.Generations, deployment)
            status.ReadyReplicas = deployment.Status.AvailableReplicas
            return nil
        },
        UpdateConditionFn(available),
        UpdateConditionFn(notDegraded))
    
    return err
}
```

### Idempotency

All resource apply operations are **idempotent**:

```go
// Apply functions return (resource, changed, error)
service, changed, err := resourceapply.ApplyService(ctx, client, recorder, required)

// Changed = true only if resource was created or modified
// Changed = false if resource already exists and matches
```

**Benefits**:
- Safe to run reconciliation repeatedly
- No duplicate resources created
- Automatically corrects drift

### Error Handling

**Retry Strategy**: Exponential backoff (built into controller runtime)

```
Error occurs
    │
    ▼
Return error from sync()
    │
    ▼
Controller requeues
    │
    ├──▶ Wait: 5s → 10s → 20s → 40s → max 5min
    │
    ▼
Retry sync()
```

**Status Condition on Error**:
```go
if err != nil {
    v1helpers.UpdateStatus(ctx, client, UpdateConditionFn(
        operatorv1.OperatorCondition{
            Type:    operatorv1.OperatorStatusTypeDegraded,
            Status:  operatorv1.ConditionTrue,
            Reason:  "ReconcileError",
            Message: err.Error(),
        }))
    return err  // Triggers requeue
}
```

## Certificate Management

### TLS Certificate Architecture

```
┌────────────────────────────────────────────────────────────┐
│ cert-manager (prerequisite)                                 │
│                                                              │
│  ┌──────────────┐                                           │
│  │ Issuer       │ (self-signed)                             │
│  │ jobset-      │                                           │
│  │ selfsigned-  │                                           │
│  │ issuer       │                                           │
│  └──────┬───────┘                                           │
│         │                                                    │
│         ├──────────────────┬─────────────────────┐          │
│         │                  │                     │          │
│  ┌──────▼────────┐  ┌──────▼────────┐           │          │
│  │ Certificate   │  │ Certificate   │           │          │
│  │ jobset-       │  │ jobset-       │           │          │
│  │ serving-cert  │  │ metrics-cert  │           │          │
│  └──────┬────────┘  └──────┬────────┘           │          │
│         │                  │                     │          │
│         │ creates          │ creates             │          │
│         ▼                  ▼                     │          │
│  ┌────────────────┐  ┌────────────────┐         │          │
│  │ Secret         │  │ Secret         │         │          │
│  │ webhook-       │  │ metrics-       │         │          │
│  │ server-cert    │  │ server-cert    │         │          │
│  └────────────────┘  └────────────────┘         │          │
└──────────────────────────────────────────────────┼──────────┘
                                                   │
         ┌─────────────────────────────────────────┘
         │
         │ injects CA bundle
         ▼
┌────────────────────────────────────────────────────────────┐
│ Resources with CA bundle                                    │
│                                                              │
│  ┌──────────────────────────────────────┐                  │
│  │ ValidatingWebhookConfiguration       │                  │
│  │   metadata.annotations:               │                  │
│  │     cert-manager.io/inject-ca-from:   │                  │
│  │       openshift-jobset-operator/      │                  │
│  │       jobset-serving-cert             │                  │
│  │   webhooks[].clientConfig.caBundle:   │                  │
│  │     <injected by cert-manager>        │                  │
│  └──────────────────────────────────────┘                  │
│                                                              │
│  ┌──────────────────────────────────────┐                  │
│  │ MutatingWebhookConfiguration         │                  │
│  │   (same structure)                    │                  │
│  └──────────────────────────────────────┘                  │
│                                                              │
│  ┌──────────────────────────────────────┐                  │
│  │ CRD: jobsets.jobset.x-k8s.io         │                  │
│  │   spec.conversion.webhook.            │                  │
│  │     clientConfig.caBundle:            │                  │
│  │       <injected by cert-manager>      │                  │
│  └──────────────────────────────────────┘                  │
└────────────────────────────────────────────────────────────┘
```

### Certificate Lifecycle

**1. Operator Creates Issuer**:
```yaml
apiVersion: cert-manager.io/v1
kind: Issuer
metadata:
  name: jobset-selfsigned-issuer
  namespace: openshift-jobset-operator
spec:
  selfSigned: {}
```

**2. Operator Creates Certificate**:
```yaml
apiVersion: cert-manager.io/v1
kind: Certificate
metadata:
  name: jobset-serving-cert
  namespace: openshift-jobset-operator
spec:
  dnsNames:
  - jobset-webhook-service.openshift-jobset-operator.svc
  - jobset-webhook-service.openshift-jobset-operator.svc.cluster.local
  issuerRef:
    kind: Issuer
    name: jobset-selfsigned-issuer
  secretName: webhook-server-cert
```

**3. cert-manager Creates Secret**:
```yaml
apiVersion: v1
kind: Secret
metadata:
  name: webhook-server-cert
  namespace: openshift-jobset-operator
type: kubernetes.io/tls
data:
  ca.crt: <base64-encoded-ca>
  tls.crt: <base64-encoded-cert>
  tls.key: <base64-encoded-key>
```

**4. Operator Waits for Secret**:
```go
secret, _, err := t.checkSecretReady(WebhookCertificateSecretName)
if err != nil {
    return err  // Requeue
}
if len(secret.Data["tls.crt"]) == 0 || len(secret.Data["tls.key"]) == 0 {
    return fmt.Errorf("secret not initialized")
}
```

**5. Operator Annotates Webhooks**:
```go
annotations[CertManagerInjectCaAnnotation] = 
    "openshift-jobset-operator/jobset-serving-cert"
```

**6. cert-manager Injects CA**:
cert-manager watches for the annotation and populates `caBundle` fields.

### Certificate Rotation

**Automatic Rotation**:
- cert-manager automatically renews certificates before expiry (default: 2/3 of lifetime)
- Self-signed certs have 1 year validity by default
- New secret triggers deployment rollout (via annotation on deployment)

**Force Rotation**:
```bash
# Delete certificate to force regeneration
oc delete certificate jobset-serving-cert -n openshift-jobset-operator

# Delete secret to force regeneration
oc delete secret webhook-server-cert -n openshift-jobset-operator

# cert-manager will recreate automatically
```

## Monitoring and Observability

### Metrics

**JobSet Controller Metrics**:
- Exposed on port 8443 (HTTPS with mTLS)
- Path: `/metrics`
- Format: Prometheus

**ServiceMonitor**:
```yaml
apiVersion: monitoring.coreos.com/v1
kind: ServiceMonitor
metadata:
  name: jobset-controller-manager-metrics-monitor
  namespace: openshift-jobset-operator
spec:
  endpoints:
  - port: https
    scheme: https
    tlsConfig:
      serverName: jobset-controller-manager-metrics-service.openshift-jobset-operator.svc
      certFile: /etc/prometheus/secrets/metrics-client-certs/tls.crt
      keyFile: /etc/prometheus/secrets/metrics-client-certs/tls.key
  selector:
    matchLabels:
      control-plane: controller-manager
```

**Prometheus Integration**:
1. Role in operator namespace grants `get` on pods
2. RoleBinding gives Prometheus service account this role
3. Prometheus mounts client certs from `openshift-monitoring` namespace
4. Metrics scraped via mTLS

### Status Conditions

**Three Standard Conditions**:

```yaml
status:
  conditions:
  - type: Available
    status: "True"
    reason: AsExpected
    message: All operand pods are running
    lastTransitionTime: "2026-06-23T10:00:00Z"
    
  - type: Degraded
    status: "False"
    reason: AsExpected
    lastTransitionTime: "2026-06-23T10:00:00Z"
    
  - type: Progressing
    status: "False"
    reason: AsExpected
    lastTransitionTime: "2026-06-23T10:00:00Z"
```

**Condition Logic**:

**Available**:
- `True`: Deployment exists and all replicas are available
- `False`: Deployment missing or not all replicas ready

**Degraded**:
- `True`: Error during reconciliation (cert-manager missing, secret not ready, etc.)
- `False`: No errors

**Progressing**:
- `True`: Deployment generation changed (rollout in progress)
- `False`: Observed generation matches desired generation

### Logging

**Operator Logs**:
```bash
oc logs -n openshift-jobset-operator -l app=jobset-operator -f
```

**Operand Logs**:
```bash
oc logs -n openshift-jobset-operator -l control-plane=controller-manager -f
```

**Log Levels**:
Controlled via `JobSetOperator.spec.logLevel`:
- `Normal` → operand uses `--zap-log-level=info`
- `Debug`/`Trace`/`TraceAll` → operand uses `--zap-log-level=debug`

**Note**: The `logLevel` field affects the operand, not the operator itself.

## Design Decisions

### Why OpenShift library-go?

**Decision**: Use OpenShift's library-go instead of operator-sdk or kubebuilder.

**Rationale**:
- **Consistency**: Matches other OpenShift operators (descheduler, scheduler, etc.)
- **Maturity**: Battle-tested patterns used across OpenShift
- **Maintenance**: Maintained by OpenShift team
- **Features**: Built-in status management, resource apply, event recording

**Trade-offs**:
- ✅ Strong conventions and best practices
- ✅ Excellent resource management helpers
- ❌ Steeper learning curve than operator-sdk
- ❌ Less community documentation (OpenShift-specific)

### Why cert-manager?

**Decision**: Use cert-manager for certificate management instead of custom cert generation.

**Rationale**:
- **Standard**: Industry-standard certificate management for Kubernetes
- **Automatic Rotation**: Certs rotate automatically before expiry
- **CA Injection**: Automatically injects CA bundles into webhooks/CRDs
- **Operand Requirement**: JobSet controller already requires cert-manager

**Trade-offs**:
- ✅ Production-grade certificate management
- ✅ No custom cert logic to maintain
- ✅ Automatic rotation reduces operational burden
- ❌ Additional prerequisite for users to install
- ❌ Dependency on external component

**Alternative Considered**: service-ca-operator (OpenShift-specific)
- Rejected because upstream JobSet uses cert-manager
- Would create divergence from upstream

### Why Singleton JobSetOperator CR?

**Decision**: Enforce single JobSetOperator CR named "cluster".

**Rationale**:
- **Simplicity**: One source of truth for configuration
- **OpenShift Convention**: Matches other OpenShift operators
- **Clear Ownership**: No ambiguity about which CR controls the operand
- **Status Clarity**: Single status object to check

**Enforcement**:
```go
// +kubebuilder:validation:XValidation:rule="self.metadata.name == 'cluster'"
```

**Trade-offs**:
- ✅ Simple mental model
- ✅ Prevents configuration conflicts
- ❌ Less flexible than multi-instance pattern
- ❌ Can't run multiple operands in different modes

### Why Embed Assets?

**Decision**: Embed YAML manifests in the operator binary using bindata.

**Rationale**:
- **Self-Contained**: Operator binary contains everything needed
- **Version Consistency**: Assets match operator version
- **No External Dependencies**: No need to fetch manifests at runtime
- **Airgapped Support**: Works in disconnected environments

**Trade-offs**:
- ✅ Simple deployment (single binary)
- ✅ Version consistency guaranteed
- ✅ Works in airgapped environments
- ❌ Binary size increases
- ❌ Asset changes require rebuild

### Why Separate Operator and Operand?

**Decision**: Operator deploys a separate operand (JobSet controller) instead of running it in-process.

**Rationale**:
- **Upstream Alignment**: JobSet is an upstream Kubernetes project
- **Update Independence**: Operand can be updated without operator changes
- **Resource Isolation**: Separate resource limits and lifecycle
- **Failure Isolation**: Operand crash doesn't kill operator

**Trade-offs**:
- ✅ Clean separation of concerns
- ✅ Can track upstream releases easily
- ✅ Failure isolation
- ❌ More moving parts
- ❌ More resources consumed (2 deployments)

## Failure Modes and Recovery

### Operator Failures

#### Operator Pod Crashes

**Symptom**: Operator pod is CrashLooping

**Detection**:
```bash
oc get pods -n openshift-jobset-operator -l app=jobset-operator
```

**Recovery**:
- Deployment controller recreates pod automatically
- Reconciliation resumes when new pod starts
- No manual intervention needed

**Mitigation**:
- Operator is stateless (uses informer caches)
- All state is in Kubernetes API
- Restart is safe and fast

#### Reconciliation Errors

**Symptom**: `Degraded: True` condition

**Detection**:
```bash
oc get jobsetoperator cluster -o jsonpath='{.status.conditions[?(@.type=="Degraded")]}'
```

**Common Causes**:
1. cert-manager not installed
2. Certificates not ready
3. Webhook service unreachable
4. Image pull errors

**Recovery**:
- Operator retries with exponential backoff
- Fix underlying issue (install cert-manager, fix image, etc.)
- Reconciliation succeeds on next retry

### Operand Failures

#### Operand Pod Crashes

**Symptom**: JobSet controller pod is CrashLooping

**Detection**:
```bash
oc get pods -n openshift-jobset-operator -l control-plane=controller-manager
```

**Recovery**:
- Deployment controller recreates pod automatically
- Operator monitors deployment status
- Sets `Available: False` until pod is ready

**Mitigation**:
- Operand uses leader election (safe to restart)
- JobSet reconciliation resumes from last known state

#### Webhook Failures

**Symptom**: JobSet creation fails with webhook error

**Example Error**:
```
Internal error occurred: failed calling webhook "mjobset.kb.io": 
Post "https://jobset-webhook-service.openshift-jobset-operator.svc:443/mutate-jobset-x-k8s-io-v1alpha2-jobset?timeout=10s": 
service "jobset-webhook-service" not found
```

**Detection**:
```bash
# Test webhook
cat <<EOF | oc apply --dry-run=server -f -
apiVersion: jobset.x-k8s.io/v1alpha2
kind: JobSet
metadata:
  name: test
  namespace: default
spec: {}
EOF
```

**Recovery**:
1. Check webhook service exists:
   ```bash
   oc get svc -n openshift-jobset-operator jobset-webhook-service
   ```
2. Check webhook endpoint (operand pod):
   ```bash
   oc get endpoints -n openshift-jobset-operator jobset-webhook-service
   ```
3. Check certificate:
   ```bash
   oc get secret -n openshift-jobset-operator webhook-server-cert
   ```
4. If missing, delete and let operator recreate:
   ```bash
   oc delete svc,secret,certificate -n openshift-jobset-operator -l <selector>
   ```

### Certificate Failures

#### Certificates Not Ready

**Symptom**: Operator logs show "secret not initialized"

**Detection**:
```bash
oc get certificate -n openshift-jobset-operator
# Status should be "True" for Ready condition
```

**Recovery**:
1. Check cert-manager is installed:
   ```bash
   oc get pods -n cert-manager
   ```
2. Check cert-manager logs:
   ```bash
   oc logs -n cert-manager -l app=cert-manager
   ```
3. Delete certificate to force recreation:
   ```bash
   oc delete certificate jobset-serving-cert -n openshift-jobset-operator
   ```

#### CA Bundle Not Injected

**Symptom**: Webhook calls fail with TLS errors

**Detection**:
```bash
oc get validatingwebhookconfiguration jobset-validating-webhook-configuration -o yaml | grep caBundle
# Should see base64-encoded CA
```

**Recovery**:
1. Check cert-manager webhook is running:
   ```bash
   oc get pods -n cert-manager -l app=webhook
   ```
2. Check annotation is present:
   ```bash
   oc get validatingwebhookconfiguration jobset-validating-webhook-configuration -o yaml | grep cert-manager.io/inject-ca-from
   ```
3. Delete webhook config to force recreation:
   ```bash
   oc delete validatingwebhookconfiguration jobset-validating-webhook-configuration
   ```

### Recovery Best Practices

1. **Check Status First**:
   ```bash
   oc get jobsetoperator cluster -o yaml
   ```

2. **Review Logs**:
   ```bash
   oc logs -n openshift-jobset-operator -l app=jobset-operator --tail=100
   ```

3. **Check Events**:
   ```bash
   oc get events -n openshift-jobset-operator --sort-by='.lastTimestamp'
   ```

4. **Let Operator Recover**: Most issues self-heal via reconciliation

5. **Delete and Recreate**: If stuck, delete the affected resource (operator recreates it)

6. **Last Resort**: Delete operator pod to force fresh reconciliation:
   ```bash
   oc delete pod -n openshift-jobset-operator -l app=jobset-operator
   ```

## Upgrade and Migration

### Operator Upgrades

**Controlled by**: OpenShift OLM (OperatorHub)

**Upgrade Process**:
1. New operator version deployed
2. Old operator pod terminates
3. New operator pod starts
4. Reconciliation runs with new code
5. Resources updated if manifests changed

**Zero-Downtime**: Operand continues running during operator upgrade

### Operand Upgrades

**Triggered by**: Updating `OPERAND_IMAGE` env var

**Note**: `operand-git-ref` is only needed when creating a new release (to regenerate manifests via `hack/update-jobset-controller-manifests.sh`), not during the deployment of a new release.

**Upgrade Process**:
1. Operator detects image change
2. Updates deployment with new image
3. Kubernetes performs rolling update:
   - New pod starts
   - Old pod terminates after new pod is ready
4. Minimal downtime (only during webhook switchover)

**Rollback**:
```bash
# Rollback deployment
oc rollout undo deployment/jobset-controller-manager -n openshift-jobset-operator
```

## Additional Resources

- [JobSet Documentation](https://jobset.sigs.k8s.io/)
- [Kubernetes JobSet Blog Post](https://kubernetes.io/blog/2025/03/23/introducing-jobset/)
- [OpenShift library-go](https://github.com/openshift/library-go)
- [cert-manager Documentation](https://cert-manager.io/docs/)
- [OpenShift Operator Lifecycle Manager](https://docs.openshift.com/container-platform/latest/operators/understanding/olm/olm-understanding-olm.html)

---

**Document Version**: 1.0  
**Last Updated**: 2026-06-23  
**Maintained By**: JobSet Operator Team
