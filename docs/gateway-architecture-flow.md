# Gateway Controller Architecture - Complete Flow

## How It Works: Step-by-Step

### 1. User Creates Tenant with Gateway Enabled

```yaml
apiVersion: minio.min.io/v2
kind: Tenant
metadata:
  name: minio-tenant
  namespace: minio-tenant
spec:
  pools:
    - servers: 4
      volumesPerServer: 4
  features:
    enableGateway: true  # ← This triggers everything
```

### 2. MinIO Operator Reconciliation Loop

When the operator detects a tenant with `enableGateway: true`:

```
operator/pkg/controller/main-controller.go (syncHandler)
    ↓
    Checks: tenant.Spec.Features.EnableGateway == true
    ↓
    Calls: ensureGatewayController(ctx)  ← Deploys gateway controller (once)
    ↓
    Calls: checkEnvoyGateway(ctx, tenant) ← Deploys Envoy for this tenant
```

### 3. Gateway Controller Deployment (Once Per Cluster)

The operator creates in `minio-operator` namespace:

```
operator/pkg/controller/gateway-controller.go
    ↓
    Creates:
    - ServiceAccount: gateway-controller
    - ClusterRole: gateway-controller-role (read tenants, services, endpoints)
    - ClusterRoleBinding: gateway-controller-rolebinding
    - Deployment: gateway-controller (1 replica)
    - Service: gateway-controller (ClusterIP, port 18000)
```

### 4. Envoy Gateway Deployment (Per Tenant)

The operator creates in the tenant's namespace:

```
operator/pkg/controller/envoy-gateway.go
    ↓
    Creates:
    - ConfigMap: envoy-config-<tenant-name>
      └─ Contains: Envoy bootstrap config pointing to gateway-controller:18000
    - Deployment: envoy-gateway-<tenant-name> (2 replicas by default)
    - Service: envoy-gateway-<tenant-name> (LoadBalancer, port 80)
```

### 5. Gateway Controller Watches Tenants

```
operator/cmd/gateway-controller/main.go
    ↓
    Starts XDS Server on port 18000
    ↓
    Starts Controller-Runtime Manager
    ↓
operator/pkg/gateway/reconciler/reconciler.go (XDSReconciler)
    ↓
    Watches: Tenant resources across all namespaces
    ↓
    On Tenant Change:
        1. Fetches tenant details
        2. Builds Envoy configuration snapshot
        3. Updates xDS cache
```

### 6. Snapshot Building Process

```
operator/pkg/gateway/snapshot/snapshot.go
    ↓
    NewSnapshotBuilder(tenant, clusterDomain)
    ↓
    Builds:
    ├─ Clusters (operator/pkg/gateway/builders/upstream/upstream.go)
    │   ├─ minio_cluster → Points to: <tenant-name>-hl.namespace.svc.cluster.local:9000
    │   └─ console_cluster → Points to: <tenant-name>-console.namespace.svc.cluster.local:9090
    │
    ├─ Listeners (operator/pkg/gateway/builders/listener/listener.go)
    │   └─ minio_listener → Port 10000, HTTP connection manager
    │
    └─ Routes (operator/pkg/gateway/builders/route/route.go)
        └─ minio_route → Routes traffic to minio_cluster
```

### 7. Envoy Fetches Configuration

```
Envoy Pod starts
    ↓
    Reads: /etc/envoy/envoy.yaml (from ConfigMap)
    ↓
    Connects to: gateway-controller.minio-operator.svc.cluster.local:18000
    ↓
    Sends: Node ID = "gateway-<namespace>-<tenant-name>"
    ↓
    Gateway Controller responds with xDS snapshot
    ↓
    Envoy applies configuration:
    - Listener on port 10000
    - Routes to MinIO headless service
    - Load balances across MinIO pods
```

### 8. Traffic Flow

```
External Client
    ↓
    HTTP Request to LoadBalancer IP:80
    ↓
[LoadBalancer Service] envoy-gateway-<tenant>:80
    ↓
[Envoy Pod] Port 10000
    ↓
    Envoy applies routing rules from xDS
    ↓
[MinIO Headless Service] <tenant>-hl:9000
    ↓
    DNS resolves to all MinIO pod IPs
    ↓
[MinIO Pods] Round-robin load balancing
    ↓
    MinIO processes S3 request
    ↓
    Response flows back through Envoy to client
```

## Key Components and Their Roles

### MinIO Operator (Main Controller)
- **File**: `operator/pkg/controller/main-controller.go`
- **Role**: Orchestrates deployment of gateway controller and Envoy
- **Triggers**: When tenant has `spec.features.enableGateway: true`

### Gateway Controller (XDS Server)
- **File**: `operator/cmd/gateway-controller/main.go`
- **Role**: Watches tenants and generates Envoy configurations
- **Protocol**: gRPC (xDS v3)
- **Port**: 18000
- **Service Type**: ClusterIP (internal only)

### Envoy Gateway
- **Deployment**: Per tenant
- **Role**: Load balancer and reverse proxy
- **Ports**: 
  - 10000 (HTTP traffic to MinIO)
  - 9901 (Admin/metrics)
- **Service Type**: LoadBalancer (external access)

### MinIO Tenant Services
- **Headless Service**: `<tenant>-hl:9000` (for pod-to-pod)
- **ClusterIP Service**: `<tenant>:9000` (for internal access)
- **Console Service**: `<tenant>-console:9090` (if enabled)

## Configuration Discovery (xDS Protocol)

The gateway controller uses Envoy's xDS (Discovery Service) protocol:

1. **LDS** (Listener Discovery Service)
   - Defines listeners (ports Envoy listens on)
   - Port 10000 for HTTP traffic

2. **RDS** (Route Discovery Service)
   - Defines routing rules
   - Routes S3 API calls to MinIO cluster

3. **CDS** (Cluster Discovery Service)
   - Defines upstream clusters
   - MinIO headless service endpoints

4. **EDS** (Endpoint Discovery Service)
   - Defines actual pod endpoints
   - Dynamically updated as pods scale

## Service Discovery

```
Gateway Controller queries Kubernetes API
    ↓
    Discovers: MinIO tenant services
    ↓
    Builds: Envoy cluster configuration
    ↓
    Points to: <tenant>-hl.namespace.svc.cluster.local:9000
    ↓
    Kubernetes DNS resolves to all MinIO pod IPs
    ↓
    Envoy load balances across all pods
```

## Why This Architecture?

### Separation of Concerns
- **Gateway Controller**: Configuration management (control plane)
- **Envoy**: Traffic handling (data plane)
- **MinIO Operator**: Resource orchestration

### Dynamic Configuration
- No Envoy restarts needed when MinIO pods scale
- Configuration updates via xDS protocol
- Real-time endpoint discovery

### Multi-Tenancy
- Each tenant gets isolated Envoy deployment
- Shared gateway controller reduces overhead
- Per-tenant load balancing and routing

### High Availability
- Multiple Envoy replicas per tenant
- Automatic failover
- Health checking via Kubernetes probes

## Verification Commands

### Check Gateway Controller
```bash
# Is it running?
kubectl get pods -n minio-operator -l app=gateway-controller

# View logs
kubectl logs -n minio-operator -l app=gateway-controller

# Check xDS server
kubectl port-forward -n minio-operator svc/gateway-controller 18000:18000
```

### Check Envoy Gateway
```bash
# Is it running?
kubectl get pods -n minio-tenant -l app=envoy-gateway

# View configuration
kubectl port-forward -n minio-tenant svc/envoy-gateway-minio-tenant 9901:9901
curl http://localhost:9901/config_dump | jq

# Check clusters
curl http://localhost:9901/clusters

# Check listeners
curl http://localhost:9901/listeners
```

### Check MinIO Services
```bash
# Headless service (used by Envoy)
kubectl get svc -n minio-tenant minio-tenant-hl

# Get pod IPs
kubectl get pods -n minio-tenant -o wide
```

### Test Traffic Flow
```bash
# Get LoadBalancer IP
GATEWAY_IP=$(kubectl get svc -n minio-tenant envoy-gateway-minio-tenant -o jsonpath='{.status.loadBalancer.ingress[0].ip}')

# Test S3 API through Envoy
mc alias set myminio http://$GATEWAY_IP <access-key> <secret-key>
mc ls myminio
```

## Troubleshooting

### Envoy Not Getting Configuration
1. Check gateway controller logs for errors
2. Verify Envoy can reach gateway-controller:18000
3. Check node ID matches: `gateway-<namespace>-<tenant-name>`

### Traffic Not Reaching MinIO
1. Verify MinIO headless service exists
2. Check DNS resolution: `nslookup <tenant>-hl.namespace.svc.cluster.local`
3. Verify MinIO pods are ready
4. Check Envoy cluster health: `curl localhost:9901/clusters`

### LoadBalancer Not Getting External IP
1. Check cloud provider load balancer support
2. Verify service type is LoadBalancer
3. Check for pending events: `kubectl describe svc envoy-gateway-<tenant>`