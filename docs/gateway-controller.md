# MinIO Gateway Controller

## Overview

The MinIO Gateway Controller is an Envoy xDS (Discovery Service) control plane that dynamically configures Envoy proxies to route traffic to MinIO Tenants. It watches MinIO Tenant resources in Kubernetes and automatically generates Envoy configuration snapshots.

## Architecture

```
┌─────────────────┐
│  MinIO Tenant   │
│   (CRD)         │
└────────┬────────┘
         │ watches
         ▼
┌─────────────────┐      xDS/gRPC      ┌─────────────────┐
│    Gateway      │◄───────────────────┤  Envoy Proxy    │
│   Controller    │                    │   (Gateway)     │
└─────────────────┘                    └────────┬────────┘
         │                                      │
         │ generates snapshots                  │ routes traffic
         ▼                                      ▼
┌─────────────────┐                    ┌─────────────────┐
│  xDS Snapshot   │                    │  MinIO Service  │
│     Cache       │                    │   (Headless)    │
└─────────────────┘                    └─────────────────┘
```

## Components

### 1. Gateway Controller (`cmd/gateway-controller/main.go`)
- Main entry point
- Runs two parallel services:
  - **xDS gRPC Server** (port 18000) - serves Envoy configuration
  - **Controller Manager** - watches Kubernetes Tenant resources
- Uses controller-runtime framework

### 2. XDS Server (`pkg/gateway/xdsserver/server.go`)
- Implements Envoy's xDS protocol (EDS, CDS, RDS, LDS)
- Serves dynamic configuration to Envoy proxies
- gRPC-based communication with keepalive

### 3. Reconciler (`pkg/gateway/reconciler/reconciler.go`)
- Watches MinIO Tenant CRDs
- Builds Envoy snapshots from Tenant specifications
- Updates xDS cache when Tenant resources change
- Read-only mode (doesn't modify Tenant resources)

### 4. Snapshot Builder (`pkg/gateway/snapshot/snapshot.go`)
- Generates Envoy configuration:
  - **Clusters**: MinIO service endpoints
  - **Listeners**: HTTP/HTTPS listeners
  - **Routes**: Path-based routing rules
- Version-controlled snapshots (Unix microseconds)

## Features

- **Dynamic Configuration**: Automatically updates Envoy when Tenants change
- **Multi-Tenant Support**: Routes traffic to multiple MinIO Tenants
- **Load Balancing**: Round-robin across MinIO pools
- **Health Checking**: Integrates with Kubernetes service discovery
- **Namespace Isolation**: Can watch specific namespaces or all namespaces

## Installation

### Prerequisites

1. Kubernetes cluster (1.30.0+)
2. MinIO Operator installed
3. At least one MinIO Tenant deployed

### Deploy Gateway Controller

```bash
# Deploy the gateway controller
kubectl apply -f deploy/gateway-controller.yaml

# Deploy Envoy gateway
kubectl apply -f deploy/envoy-gateway.yaml
```

### Verify Installation

```bash
# Check gateway controller is running
kubectl get pods -n minio-operator -l app=gateway-controller

# Check Envoy gateway is running
kubectl get pods -n minio-operator -l app=envoy-gateway

# Check xDS connection
kubectl logs -n minio-operator -l app=gateway-controller
```

## Configuration

### Environment Variables

| Variable | Description | Default |
|----------|-------------|---------|
| `WATCH_NAMESPACES` | Comma-separated list of namespaces to watch | `""` (all) |
| `POD_NAMESPACE` | Namespace where controller is running | `minio-operator` |

### Command-Line Flags

| Flag | Description | Default |
|------|-------------|---------|
| `--port` | xDS server port | `18000` |
| `--metrics-bind-address` | Metrics endpoint address | `:8383` |
| `--cluster-domain` | Kubernetes cluster domain | `cluster.local` |
| `--debug-xds` | Enable xDS debug logging | `false` |
| `--debug-ctrl` | Enable controller debug logging | `false` |

## Building

### Build Binary

```bash
# Build gateway-controller binary
make gateway-binary

# Build for specific architecture
GOARCH=arm64 make gateway-binary
```

### Build Docker Image

```bash
# Build Docker image
make docker-gateway

# Build with specific version
VERSION=v1.0.0 make docker-gateway
```

### Multi-Architecture Build

```bash
# Build for multiple architectures
docker buildx build --platform linux/amd64,linux/arm64,linux/s390x \
  -t minio/gateway-controller:latest \
  -f Dockerfile.gateway .
```

## Usage

### Access MinIO Through Gateway

Once deployed, access MinIO through the Envoy gateway:

```bash
# Get gateway service external IP
kubectl get svc envoy-gateway -n minio-operator

# Access MinIO via gateway
mc alias set mygw http://<EXTERNAL-IP> <ACCESS-KEY> <SECRET-KEY>
mc ls mygw
```

### Watch Specific Namespaces

To restrict the controller to specific namespaces:

```yaml
env:
- name: WATCH_NAMESPACES
  value: "tenant-ns-1,tenant-ns-2"
```

### Enable Debug Logging

```yaml
args:
- --debug-xds=true
- --debug-ctrl=true
```

## Monitoring

### Metrics

The gateway controller exposes Prometheus metrics on port 8383:

```bash
# Access metrics
kubectl port-forward -n minio-operator svc/gateway-controller 8383:8383
curl http://localhost:8383/metrics
```

### Health Checks

```bash
# Check readiness
curl http://localhost:8383/readyz
```

### Envoy Admin Interface

Envoy exposes an admin interface on port 9901:

```bash
# Port forward to Envoy admin
kubectl port-forward -n minio-operator svc/envoy-gateway 9901:9901

# View configuration
curl http://localhost:9901/config_dump

# View clusters
curl http://localhost:9901/clusters

# View listeners
curl http://localhost:9901/listeners
```

## Troubleshooting

### Gateway Controller Not Starting

```bash
# Check logs
kubectl logs -n minio-operator -l app=gateway-controller

# Check RBAC permissions
kubectl auth can-i get tenants --as=system:serviceaccount:minio-operator:gateway-controller
```

### Envoy Not Receiving Configuration

```bash
# Check xDS connection in Envoy logs
kubectl logs -n minio-operator -l app=envoy-gateway

# Verify gateway-controller service is accessible
kubectl get svc gateway-controller -n minio-operator

# Check xDS server is listening
kubectl exec -n minio-operator <gateway-controller-pod> -- netstat -tlnp | grep 18000
```

### Traffic Not Routing to MinIO

```bash
# Check Envoy clusters
kubectl port-forward -n minio-operator <envoy-pod> 9901:9901
curl http://localhost:9901/clusters | grep minio

# Verify MinIO service endpoints
kubectl get endpoints -n <tenant-namespace> <tenant-name>-hl

# Check snapshot version
kubectl logs -n minio-operator -l app=gateway-controller | grep "snapshot updated"
```

## Development

### Running Locally

```bash
# Set kubeconfig
export KUBECONFIG=~/.kube/config

# Run gateway controller
go run cmd/gateway-controller/main.go \
  --kubeconfig=$KUBECONFIG \
  --debug-xds=true \
  --debug-ctrl=true
```

### Testing

```bash
# Run tests
go test ./pkg/gateway/...

# Run with coverage
go test -cover ./pkg/gateway/...
```

## Advanced Configuration

### Custom Envoy Configuration

Modify `deploy/envoy-gateway.yaml` to customize Envoy behavior:

- Add TLS termination
- Configure access logging
- Add rate limiting
- Implement circuit breakers

### Multi-Cluster Setup

Deploy gateway-controller in each cluster and use external DNS for cross-cluster routing.

### High Availability

Increase gateway-controller replicas (requires leader election):

```yaml
spec:
  replicas: 3
```

## Comparison with Direct Access

| Feature | Direct Access | Gateway Controller |
|---------|--------------|-------------------|
| Load Balancing | Kubernetes Service | Envoy (advanced) |
| Routing | Basic | Path/header-based |
| TLS Termination | At MinIO | At Gateway |
| Observability | Limited | Rich (Envoy metrics) |
| Traffic Management | Basic | Advanced (retries, timeouts) |
| Multi-Tenant | Separate services | Single gateway |

## References

- [Envoy xDS Protocol](https://www.envoyproxy.io/docs/envoy/latest/api-docs/xds_protocol)
- [go-control-plane](https://github.com/envoyproxy/go-control-plane)
- [MinIO Operator](https://github.com/minio/operator)
- [Controller Runtime](https://github.com/kubernetes-sigs/controller-runtime)

## License

GNU Affero General Public License v3.0