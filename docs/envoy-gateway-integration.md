# Envoy Gateway Integration with MinIO Operator

## Overview

The MinIO Operator now supports automatic deployment of Envoy Gateway for load balancing and advanced routing capabilities. When enabled, the operator automatically deploys:

1. **Gateway Controller** - A control plane component that watches MinIO Tenants and generates Envoy configurations
2. **Envoy Gateway** - One or more Envoy proxy instances per tenant that handle incoming traffic

## Architecture

```
┌─────────────────────────────────────────────────────────────┐
│                     Kubernetes Cluster                       │
│                                                              │
│  ┌────────────────────────────────────────────────────┐    │
│  │           minio-operator namespace                  │    │
│  │                                                      │    │
│  │  ┌──────────────────────┐                          │    │
│  │  │ MinIO Operator       │                          │    │
│  │  │ (Main Controller)    │                          │    │
│  │  └──────────┬───────────┘                          │    │
│  │             │                                        │    │
│  │             │ Deploys when tenant.spec.features.    │    │
│  │             │ enableGateway: true                   │    │
│  │             ▼                                        │    │
│  │  ┌──────────────────────┐                          │    │
│  │  │ Gateway Controller   │◄─────────────────┐       │    │
│  │  │ (XDS Server)         │                  │       │    │
│  │  └──────────┬───────────┘                  │       │    │
│  │             │                               │       │    │
│  │             │ Watches Tenants               │       │    │
│  │             │ Generates xDS configs         │       │    │
│  └─────────────┼───────────────────────────────┼───────┘    │
│                │                               │            │
│  ┌─────────────▼───────────────────────────────┼───────┐    │
│  │           tenant namespace                  │       │    │
│  │                                             │       │    │
│  │  ┌──────────────────────┐                  │       │    │
│  │  │ MinIO Tenant         │                  │       │    │
│  │  │ (StatefulSet)        │                  │       │    │
│  │  └──────────────────────┘                  │       │    │
│  │                                             │       │    │
│  │  ┌──────────────────────┐                  │       │    │
│  │  │ Envoy Gateway        │──────────────────┘       │    │
│  │  │ (Deployment)         │  Fetches config via gRPC │    │
│  │  └──────────┬───────────┘                          │    │
│  │             │                                        │    │
│  │             │ Routes traffic to MinIO pods          │    │
│  │             ▼                                        │    │
│  │  ┌──────────────────────┐                          │    │
│  │  │ MinIO Service        │                          │    │
│  │  │ (LoadBalancer)       │                          │    │
│  │  └──────────────────────┘                          │    │
│  └─────────────────────────────────────────────────────┘    │
│                                                              │
└─────────────────────────────────────────────────────────────┘
```

## Features

- **Automatic Deployment**: Gateway controller and Envoy proxies are automatically deployed when enabled
- **Dynamic Configuration**: Envoy configuration is dynamically generated based on tenant specifications
- **Load Balancing**: Intelligent load balancing across MinIO pods
- **High Availability**: Multiple Envoy replicas for redundancy
- **Per-Tenant Isolation**: Each tenant gets its own Envoy gateway deployment

## Enabling Envoy Gateway

### Step 1: Deploy the MinIO Operator

First, ensure the MinIO Operator is deployed in your cluster:

```bash
kubectl apply -f https://github.com/minio/operator/releases/latest/download/operator.yaml
```

### Step 2: Create a Tenant with Gateway Enabled

Create a tenant YAML file with the gateway feature enabled:

```yaml
apiVersion: minio.min.io/v2
kind: Tenant
metadata:
  name: minio-tenant
  namespace: minio-tenant
spec:
  image: quay.io/minio/minio:latest
  
  pools:
    - servers: 4
      name: pool-0
      volumesPerServer: 4
      volumeClaimTemplate:
        spec:
          storageClassName: standard
          accessModes:
            - ReadWriteOnce
          resources:
            requests:
              storage: 10Gi
  
  configuration:
    name: minio-env-configuration
  
  features:
    # Enable Envoy Gateway
    enableGateway: true
    # Optional: Set number of Envoy replicas (default: 2)
    gatewayReplicas: 2
```

### Step 3: Apply the Configuration

```bash
kubectl create namespace minio-tenant
kubectl apply -f tenant-with-gateway.yaml
```

## What Gets Deployed

When you enable the gateway feature, the operator automatically creates:

### In the `minio-operator` namespace:

1. **Gateway Controller Deployment**
   - Service Account: `gateway-controller`
   - ClusterRole: `gateway-controller-role`
   - ClusterRoleBinding: `gateway-controller-rolebinding`
   - Deployment: `gateway-controller` (1 replica)
   - Service: `gateway-controller` (ClusterIP on port 18000)

### In the tenant namespace:

1. **Envoy Gateway Resources**
   - ConfigMap: `envoy-config-<tenant-name>` (Envoy bootstrap configuration)
   - Deployment: `envoy-gateway-<tenant-name>` (configurable replicas, default 2)
   - Service: `envoy-gateway-<tenant-name>` (LoadBalancer on port 80)

## Configuration Options

### Tenant Spec Features

```yaml
spec:
  features:
    # Enable/disable gateway (default: false)
    enableGateway: true
    
    # Number of Envoy proxy replicas (default: 2)
    gatewayReplicas: 3
```

### Gateway Controller Configuration

The gateway controller is deployed with the following default settings:

- **XDS Port**: 18000 (gRPC)
- **Metrics Port**: 8383 (HTTP)
- **Cluster Domain**: cluster.local
- **Watch Namespaces**: All namespaces (can be restricted via WATCH_NAMESPACES env var)

### Envoy Gateway Configuration

Each Envoy gateway deployment includes:

- **HTTP Port**: 10000 (mapped to LoadBalancer port 80)
- **Admin Port**: 9901 (for health checks and metrics)
- **Resource Limits**: 1 CPU, 1Gi memory
- **Resource Requests**: 200m CPU, 256Mi memory

## Accessing MinIO Through Envoy

Once deployed, you can access MinIO through the Envoy gateway LoadBalancer:

```bash
# Get the LoadBalancer external IP
kubectl get svc -n minio-tenant envoy-gateway-minio-tenant

# Access MinIO via the LoadBalancer
mc alias set myminio http://<EXTERNAL-IP> <ACCESS-KEY> <SECRET-KEY>
```

## Monitoring

### Gateway Controller Metrics

The gateway controller exposes Prometheus metrics on port 8383:

```bash
kubectl port-forward -n minio-operator svc/gateway-controller 8383:8383
curl http://localhost:8383/metrics
```

### Envoy Metrics

Each Envoy gateway exposes admin interface and metrics on port 9901:

```bash
kubectl port-forward -n minio-tenant svc/envoy-gateway-minio-tenant 9901:9901
curl http://localhost:9901/stats/prometheus
```

## Troubleshooting

### Check Gateway Controller Status

```bash
# Check if gateway controller is running
kubectl get pods -n minio-operator -l app=gateway-controller

# View gateway controller logs
kubectl logs -n minio-operator -l app=gateway-controller
```

### Check Envoy Gateway Status

```bash
# Check if Envoy gateway is running
kubectl get pods -n minio-tenant -l app=envoy-gateway

# View Envoy gateway logs
kubectl logs -n minio-tenant -l app=envoy-gateway,tenant=minio-tenant

# Check Envoy configuration
kubectl port-forward -n minio-tenant svc/envoy-gateway-minio-tenant 9901:9901
curl http://localhost:9901/config_dump
```

### Common Issues

#### Gateway Controller Not Deployed

If the gateway controller is not automatically deployed:

1. Check operator logs:
   ```bash
   kubectl logs -n minio-operator -l app=minio-operator
   ```

2. Verify RBAC permissions for the operator

#### Envoy Gateway Not Receiving Configuration

1. Check if gateway controller can reach the Kubernetes API:
   ```bash
   kubectl logs -n minio-operator -l app=gateway-controller
   ```

2. Verify the tenant has `enableGateway: true` in its spec

3. Check Envoy logs for connection errors:
   ```bash
   kubectl logs -n minio-tenant -l app=envoy-gateway
   ```

## Disabling Envoy Gateway

To disable the Envoy gateway for a tenant:

1. Edit the tenant:
   ```bash
   kubectl edit tenant -n minio-tenant minio-tenant
   ```

2. Set `enableGateway: false` or remove the field:
   ```yaml
   spec:
     features:
       enableGateway: false
   ```

3. The operator will automatically remove the Envoy gateway resources

**Note**: The gateway controller will remain deployed as it may be used by other tenants.

## Advanced Configuration

### Custom Envoy Image

To use a custom Envoy image, modify the `EnvoyImage` constant in `operator/pkg/controller/envoy-gateway.go`:

```go
const EnvoyImage = "your-registry/envoy:custom-tag"
```

### Custom Gateway Controller Image

To use a custom gateway controller image, modify the `GatewayControllerImage` constant in `operator/pkg/controller/gateway-controller.go`:

```go
const GatewayControllerImage = "your-registry/gateway-controller:custom-tag"
```

### Restricting Gateway Controller to Specific Namespaces

Set the `WATCH_NAMESPACES` environment variable in the gateway controller deployment:

```yaml
env:
- name: WATCH_NAMESPACES
  value: "namespace1,namespace2"
```

## Security Considerations

1. **RBAC**: The gateway controller requires read access to Tenants, Services, and Endpoints
2. **Network Policies**: Consider implementing network policies to restrict traffic flow
3. **TLS**: Configure TLS certificates for secure communication
4. **Service Accounts**: Each component runs with its own service account with minimal permissions

## Performance Tuning

### Envoy Gateway Replicas

Adjust the number of Envoy replicas based on your traffic:

```yaml
spec:
  features:
    enableGateway: true
    gatewayReplicas: 5  # Increase for higher traffic
```

### Resource Limits

Modify resource limits in `operator/pkg/controller/envoy-gateway.go` for your workload:

```go
Resources: corev1.ResourceRequirements{
    Limits: corev1.ResourceList{
        corev1.ResourceCPU:    resource.MustParse("2000m"),
        corev1.ResourceMemory: resource.MustParse("2Gi"),
    },
    Requests: corev1.ResourceList{
        corev1.ResourceCPU:    resource.MustParse("500m"),
        corev1.ResourceMemory: resource.MustParse("512Mi"),
    },
}
```

## References

- [Envoy Proxy Documentation](https://www.envoyproxy.io/docs)
- [MinIO Operator Documentation](https://min.io/docs/minio/kubernetes/upstream/)
- [Kubernetes Service Documentation](https://kubernetes.io/docs/concepts/services-networking/service/)