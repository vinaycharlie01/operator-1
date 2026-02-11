# Gateway Controller Setup Guide

## Required Dependencies

To use the gateway-controller, you need to add the following dependencies to your `go.mod` file:

### Add Dependencies

Run these commands in the `operator` directory:

```bash
# Add Envoy go-control-plane dependencies
go get github.com/envoyproxy/go-control-plane@v0.12.0

# Add required gRPC dependencies
go get google.golang.org/grpc@v1.60.0
go get google.golang.org/protobuf@v1.31.0

# Add zap logger dependencies
go get go.uber.org/zap@v1.26.0

# Add pflag for CLI flags
go get github.com/spf13/pflag@v1.0.5

# Add go-logr for logging interface
go get github.com/go-logr/logr@v1.4.1

# Add rate limiting
go get golang.org/x/time@v0.5.0

# Tidy up dependencies
go mod tidy
```

### Expected go.mod Additions

After running the above commands, your `go.mod` should include:

```go
require (
    github.com/envoyproxy/go-control-plane v0.12.0
    github.com/go-logr/logr v1.4.1
    github.com/spf13/pflag v1.0.5
    go.uber.org/zap v1.26.0
    golang.org/x/time v0.5.0
    google.golang.org/grpc v1.60.0
    google.golang.org/protobuf v1.31.0
    // ... existing dependencies
)
```

## Build Steps

### 1. Install Dependencies

```bash
cd operator
go mod download
```

### 2. Build the Binary

```bash
# Build for Linux
make gateway-binary

# Or build manually
CGO_ENABLED=0 GOOS=linux go build -trimpath \
  -ldflags "-s -w -X github.com/minio/operator/pkg.Version=$(git describe --tags)" \
  -o gateway-controller ./cmd/gateway-controller
```

### 3. Build Docker Image

```bash
# Build the Docker image
make docker-gateway

# Or build manually
docker build -f Dockerfile.gateway -t minio/gateway-controller:latest .
```

### 4. Deploy to Kubernetes

```bash
# Deploy gateway controller
kubectl apply -f deploy/gateway-controller.yaml

# Deploy Envoy gateway
kubectl apply -f deploy/envoy-gateway.yaml
```

## Verification

### Check Gateway Controller

```bash
# Check if pod is running
kubectl get pods -n minio-operator -l app=gateway-controller

# Check logs
kubectl logs -n minio-operator -l app=gateway-controller

# Expected output should show:
# - "Starting MinIO Gateway Controller"
# - "XDS server listening" on port 18000
# - "Creating manager"
# - "Starting manager"
```

### Check Envoy Gateway

```bash
# Check if Envoy pods are running
kubectl get pods -n minio-operator -l app=envoy-gateway

# Check Envoy logs for xDS connection
kubectl logs -n minio-operator -l app=envoy-gateway

# Expected output should show:
# - "ads: gRPC config stream connected"
# - "cds: add 1 cluster(s)"
# - "lds: add 1 listener(s)"
```

### Test xDS Connection

```bash
# Port forward to Envoy admin interface
kubectl port-forward -n minio-operator svc/envoy-gateway 9901:9901

# Check clusters (should show minio_cluster)
curl http://localhost:9901/clusters

# Check listeners (should show minio_listener)
curl http://localhost:9901/listeners

# Check configuration dump
curl http://localhost:9901/config_dump | jq
```

## Troubleshooting

### Issue: Missing Dependencies

**Error:**
```
could not import github.com/envoyproxy/go-control-plane/pkg/cache/v3
```

**Solution:**
```bash
cd operator
go get github.com/envoyproxy/go-control-plane@v0.12.0
go mod tidy
```

### Issue: Build Fails

**Error:**
```
undefined: cache.NewSnapshot
```

**Solution:**
Ensure all dependencies are properly installed:
```bash
go mod download
go mod verify
```

### Issue: xDS Server Not Starting

**Error in logs:**
```
error creating xds server listener: address already in use
```

**Solution:**
Check if port 18000 is already in use:
```bash
kubectl get svc -A | grep 18000
```

### Issue: Envoy Not Connecting

**Error in Envoy logs:**
```
gRPC config stream closed: 14, connection error
```

**Solution:**
1. Verify gateway-controller service exists:
```bash
kubectl get svc gateway-controller -n minio-operator
```

2. Check if gateway-controller is running:
```bash
kubectl get pods -n minio-operator -l app=gateway-controller
```

3. Verify network connectivity:
```bash
kubectl exec -n minio-operator <envoy-pod> -- \
  nc -zv gateway-controller.minio-operator.svc.cluster.local 18000
```

## Development Workflow

### Local Development

1. **Run gateway-controller locally:**
```bash
export KUBECONFIG=~/.kube/config
go run cmd/gateway-controller/main.go \
  --kubeconfig=$KUBECONFIG \
  --debug-xds=true \
  --debug-ctrl=true
```

2. **Deploy only Envoy to cluster:**
```bash
kubectl apply -f deploy/envoy-gateway.yaml
```

3. **Update Envoy config to point to local controller:**
```yaml
# In deploy/envoy-gateway.yaml, change:
address: host.docker.internal  # For Docker Desktop
# or
address: <your-local-ip>       # For other setups
port_value: 18000
```

### Testing Changes

1. **Make code changes**

2. **Rebuild binary:**
```bash
make gateway-binary
```

3. **Rebuild image:**
```bash
make docker-gateway
```

4. **Update deployment:**
```bash
kubectl rollout restart deployment/gateway-controller -n minio-operator
```

5. **Watch logs:**
```bash
kubectl logs -f -n minio-operator -l app=gateway-controller
```

## Next Steps

After successful setup:

1. **Deploy a MinIO Tenant:**
```bash
kubectl apply -f examples/kustomization/tenant-lite/tenant.yaml
```

2. **Verify snapshot generation:**
```bash
kubectl logs -n minio-operator -l app=gateway-controller | grep "snapshot updated"
```

3. **Access MinIO through gateway:**
```bash
# Get gateway external IP
GATEWAY_IP=$(kubectl get svc envoy-gateway -n minio-operator -o jsonpath='{.status.loadBalancer.ingress[0].ip}')

# Configure mc client
mc alias set mygw http://$GATEWAY_IP <access-key> <secret-key>

# Test access
mc ls mygw
```

## Additional Resources

- [Gateway Controller Documentation](./gateway-controller.md)
- [Envoy xDS Protocol](https://www.envoyproxy.io/docs/envoy/latest/api-docs/xds_protocol)
- [MinIO Operator Documentation](https://min.io/docs/minio/kubernetes/upstream/)