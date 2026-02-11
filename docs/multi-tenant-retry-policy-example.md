# Multi-Tenant Retry Policy Example

## Overview

This document demonstrates how the retry policy fix in `route.go` works across multiple MinIO tenants in a shared gateway controller architecture.

## Architecture: Multi-Tenant Setup

```
┌─────────────────────────────────────────────────────────────────┐
│              Gateway Controller (Shared)                         │
│  ┌──────────────────────────────────────────────────────────┐  │
│  │  XDS Server (Port 18000)                                  │  │
│  │  - Watches ALL tenant CRDs                                │  │
│  │  - Generates separate snapshots per tenant                │  │
│  │  - Uses RouteBuilder.WithRetryPolicy() for each tenant   │  │
│  └──────────────────────────────────────────────────────────┘  │
└─────────────────────────────────────────────────────────────────┘
                              │
                              │ xDS Protocol (gRPC)
                              │
        ┌─────────────────────┼─────────────────────┐
        │                     │                     │
        ▼                     ▼                     ▼
┌──────────────┐      ┌──────────────┐      ┌──────────────┐
│ Envoy Gateway│      │ Envoy Gateway│      │ Envoy Gateway│
│  (Tenant A)  │      │  (Tenant B)  │      │  (Tenant C)  │
│              │      │              │      │              │
│ Node ID:     │      │ Node ID:     │      │ Node ID:     │
│ gateway-ns-a │      │ gateway-ns-b │      │ gateway-ns -c│
└──────┬───────┘      └──────┬───────┘      └──────┬───────┘
       │                     │                     │
       ▼                     ▼                     ▼
┌──────────────┐      ┌──────────────┐      ┌──────────────┐
│ MinIO Tenant │      │ MinIO Tenant │      │ MinIO Tenant │
│      A       │      │      B       │      │      C       │
└──────────────┘      └──────────────┘      └──────────────┘
```

## How Retry Policy Works Per Tenant

### 1. Tenant Configuration

Each tenant can have different retry configurations:

```yaml
# Tenant A - Production (Conservative retries)
apiVersion: minio.min.io/v2
kind: Tenant
metadata:
  name: tenant-a
  namespace: production
spec:
  pools:
    - servers: 4
      volumesPerServer: 4
  features:
    enableGateway: true
  # Retry policy applied: 3 retries, 5s timeout

---
# Tenant B - Development (Aggressive retries)
apiVersion: minio.min.io/v2
kind: Tenant
metadata:
  name: tenant-b
  namespace: development
spec:
  pools:
    - servers: 2
      volumesPerServer: 2
  features:
    enableGateway: true
  # Retry policy applied: 5 retries, 5s timeout

---
# Tenant C - Testing (No retries)
apiVersion: minio.min.io/v2
kind: Tenant
metadata:
  name: tenant-c
  namespace: testing
spec:
  pools:
    - servers: 1
      volumesPerServer: 1
  features:
    enableGateway: true
  # Retry policy applied: 0 retries (fail fast)
```

### 2. Gateway Controller Snapshot Generation

The gateway controller generates **separate snapshots** for each tenant:

```go
// In operator/pkg/gateway/snapshot/snapshot.go
func (sb *SnapshotBuilder) Build() (cache.ResourceSnapshot, error) {
    // Build routes with retry policy for THIS tenant
    routes := []*routev3.Route{
        route.NewRouteBuilder().
            WithPrefixMatch("/").
            WithClusterName(fmt.Sprintf("minio_cluster_%s", sb.tenantName)).
            WithTimeout(30 * time.Second).
            WithRetryPolicy(sb.getRetryCount(), "5xx,reset,connect-failure").
            Build(),
    }
    
    // Each tenant gets its own snapshot with its own retry configuration
    snapshot, err := cache.NewSnapshot(
        sb.version,
        map[resource.Type][]types.Resource{
            resource.EndpointType: endpoints,
            resource.ClusterType:  clusters,
            resource.RouteType:    routes,
            resource.ListenerType: listeners,
        },
    )
    
    return snapshot, nil
}
```

### 3. The Fix: Correct Protobuf Serialization

**Before the fix** (BROKEN):
```go
// ❌ This doesn't compile - routev3.RetryPolicy_NumRetries doesn't exist
rb.route.GetRoute().RetryPolicy = &routev3.RetryPolicy{
    NumRetries: &routev3.RetryPolicy_NumRetries{
        Value: numRetries,
    },
    RetryOn: retryOn,
}
```

**After the fix** (WORKING):
```go
// ✅ Correct - uses wrapperspb.UInt32()
rb.route.GetRoute().RetryPolicy = &routev3.RetryPolicy{
    NumRetries:    wrapperspb.UInt32(numRetries),
    RetryOn:       retryOn,
    PerTryTimeout: durationpb.New(5 * time.Second),
}
```

### 4. How Each Tenant Gets Its Configuration

```
Gateway Controller Reconciliation Loop:
    │
    ├─► Tenant A detected
    │   ├─ Build snapshot for "gateway-production-tenant-a"
    │   ├─ Apply retry policy: wrapperspb.UInt32(3)
    │   └─ Update xDS cache with snapshot
    │
    ├─► Tenant B detected
    │   ├─ Build snapshot for "gateway-development-tenant-b"
    │   ├─ Apply retry policy: wrapperspb.UInt32(5)
    │   └─ Update xDS cache with snapshot
    │
    └─► Tenant C detected
        ├─ Build snapshot for "gateway-testing-tenant-c"
        ├─ Apply retry policy: wrapperspb.UInt32(0)
        └─ Update xDS cache with snapshot

Envoy Gateways Connect:
    │
    ├─► Envoy A connects with node ID "gateway-production-tenant-a"
    │   └─ Receives snapshot with 3 retries
    │
    ├─► Envoy B connects with node ID "gateway-development-tenant-b"
    │   └─ Receives snapshot with 5 retries
    │
    └─► Envoy C connects with node ID "gateway-testing-tenant-c"
        └─ Receives snapshot with 0 retries
```

## Practical Example: Testing Multi-Tenant Retries

### Step 1: Deploy Multiple Tenants

```bash
# Deploy Tenant A (Production)
kubectl apply -f - <<EOF
apiVersion: minio.min.io/v2
kind: Tenant
metadata:
  name: tenant-a
  namespace: production
spec:
  pools:
    - servers: 4
      volumesPerServer: 4
      volumeClaimTemplate:
        spec:
          accessModes: [ReadWriteOnce]
          resources:
            requests:
              storage: 10Gi
  features:
    enableGateway: true
EOF

# Deploy Tenant B (Development)
kubectl apply -f - <<EOF
apiVersion: minio.min.io/v2
kind: Tenant
metadata:
  name: tenant-b
  namespace: development
spec:
  pools:
    - servers: 2
      volumesPerServer: 2
      volumeClaimTemplate:
        spec:
          accessModes: [ReadWriteOnce]
          resources:
            requests:
              storage: 5Gi
  features:
    enableGateway: true
EOF
```

### Step 2: Verify Gateway Controller Sees Both Tenants

```bash
# Check gateway controller logs
kubectl logs -n minio-operator -l app=gateway-controller | grep "snapshot updated"

# Expected output:
# snapshot updated for node gateway-production-tenant-a version 1707504000000000
# snapshot updated for node gateway-development-tenant-b version 1707504000000001
```

### Step 3: Verify Each Envoy Has Correct Retry Configuration

```bash
# Check Tenant A's Envoy configuration
kubectl port-forward -n production svc/envoy-gateway-tenant-a 9901:9901 &
curl -s http://localhost:9901/config_dump | jq '.configs[] | select(.["@type"] | contains("RouteConfiguration")) | .dynamic_route_configs[].route_config.virtual_hosts[].routes[].route.retry_policy'

# Expected output for Tenant A:
# {
#   "retry_on": "5xx,reset,connect-failure",
#   "num_retries": 3,
#   "per_try_timeout": "5s"
# }

# Check Tenant B's Envoy configuration
kubectl port-forward -n development svc/envoy-gateway-tenant-b 9901:9901 &
curl -s http://localhost:9901/config_dump | jq '.configs[] | select(.["@type"] | contains("RouteConfiguration")) | .dynamic_route_configs[].route_config.virtual_hosts[].routes[].route.retry_policy'

# Expected output for Tenant B:
# {
#   "retry_on": "5xx,reset,connect-failure",
#   "num_retries": 5,
#   "per_try_timeout": "5s"
# }
```

### Step 4: Test Retry Behavior

```bash
# Simulate failure on Tenant A
# Stop one MinIO pod to trigger retries
kubectl scale statefulset -n production tenant-a-pool-0 --replicas=3

# Make S3 request through Envoy
GATEWAY_A=$(kubectl get svc -n production envoy-gateway-tenant-a -o jsonpath='{.status.loadBalancer.ingress[0].ip}')
mc alias set tenant-a http://$GATEWAY_A <access-key> <secret-key>
mc ls tenant-a

# Check Envoy stats for retries
curl -s http://localhost:9901/stats | grep retry

# Expected output shows retry attempts:
# cluster.minio_cluster_tenant-a.upstream_rq_retry: 2
# cluster.minio_cluster_tenant-a.upstream_rq_retry_success: 2
```

## Why This Fix Is Critical for Multi-Tenancy

### 1. **Protobuf Compatibility**
- Envoy uses Protocol Buffers for configuration
- `wrapperspb.UInt32()` creates the correct protobuf wrapper type
- Without this, the configuration cannot be serialized to Envoy

### 2. **Per-Tenant Isolation**
- Each tenant's snapshot is independent
- Retry policies are configured per tenant
- No cross-tenant interference

### 3. **Dynamic Updates**
- When tenant configuration changes, only that tenant's snapshot is rebuilt
- Other tenants continue with their existing retry policies
- No service interruption

### 4. **Scalability**
- Single gateway controller handles all tenants
- Each tenant gets optimized retry configuration
- Efficient resource usage

## Verification Checklist

- [ ] Gateway controller compiles successfully
- [ ] Multiple tenants can be deployed simultaneously
- [ ] Each Envoy receives correct retry configuration
- [ ] Retry policies work independently per tenant
- [ ] Configuration updates don't affect other tenants
- [ ] Envoy stats show retry attempts when failures occur

## Troubleshooting Multi-Tenant Retries

### Issue: Retries Not Working for a Tenant

```bash
# 1. Check if snapshot was generated
kubectl logs -n minio-operator -l app=gateway-controller | grep "tenant-name"

# 2. Verify Envoy received configuration
kubectl port-forward -n <namespace> svc/envoy-gateway-<tenant> 9901:9901
curl http://localhost:9901/config_dump | jq '.configs[] | select(.["@type"] | contains("RouteConfiguration"))'

# 3. Check retry stats
curl http://localhost:9901/stats | grep retry
```

### Issue: Different Tenants Have Same Retry Config

```bash
# This indicates snapshot generation issue
# Check gateway controller logic for tenant-specific configuration
kubectl logs -n minio-operator -l app=gateway-controller -f
```

## Summary

The retry policy fix in `route.go` enables:

1. ✅ **Correct protobuf serialization** using `wrapperspb.UInt32()`
2. ✅ **Multi-tenant support** with isolated retry configurations
3. ✅ **Dynamic updates** without affecting other tenants
4. ✅ **Scalable architecture** with shared gateway controller
5. ✅ **Production-ready** retry handling for all tenants

Each tenant gets its own Envoy gateway with its own retry policy, all managed by a single gateway controller that uses the fixed `RouteBuilder.WithRetryPolicy()` method.