# MinIO Operator - Local Development Guide

This guide explains how to set up and use the local development environment for the MinIO Operator, including both the main operator and the gateway-controller.

## Prerequisites

### Required Tools

1. **Go 1.21+**
   ```bash
   go version
   ```

2. **Docker**
   ```bash
   docker version
   ```

3. **Kubernetes Cluster** (one of):
   - [kind](https://kind.sigs.k8s.io/)
   - [minikube](https://minikube.sigs.k8s.io/)
   - [k3d](https://k3d.io/)
   - Docker Desktop with Kubernetes enabled

4. **kubectl**
   ```bash
   kubectl version
   ```

5. **Helm 3**
   ```bash
   helm version
   ```

6. **Tilt** (for live development)
   ```bash
   # macOS
   brew install tilt-dev/tap/tilt
   
   # Linux
   curl -fsSL https://raw.githubusercontent.com/tilt-dev/tilt/master/scripts/install.sh | bash
   
   # Verify
   tilt version
   ```

7. **Mage** (optional, for build automation)
   ```bash
   go install github.com/magefile/mage@latest
   ```

## Quick Start with Tilt

Tilt provides the fastest development workflow with live reloading.

### 1. Set Environment Variables

```bash
# Optional: Set Docker registry (defaults to docker.io)
export DOCKER_REGISTRY="docker.io"
export DOCKER_USERNAME="your-username"
export DOCKER_PASSWORD="your-password"  # Optional for private registries
export USER=$(whoami)
```

### 2. Start Tilt

```bash
cd operator
tilt up
```

This will:
- ✅ Compile operator and gateway-controller binaries automatically
- ✅ Build Docker images
- ✅ Deploy to your Kubernetes cluster
- ✅ Watch for code changes and rebuild/redeploy automatically
- ✅ Provide a web UI at http://localhost:10350

### 3. View Logs

Open http://localhost:10350 in your browser to see:
- Build status
- Pod logs
- Resource status
- Trigger manual rebuilds

### 4. Make Changes

Edit any Go file and Tilt will automatically:
1. Recompile the binary
2. Rebuild the Docker image
3. Redeploy to Kubernetes
4. Show you the logs

### 5. Stop Tilt

```bash
# Press Ctrl+C in the terminal, then:
tilt down
```

## Using Mage for Build Tasks

Mage provides convenient build targets similar to Make but written in Go.

### Install Mage

```bash
go install github.com/magefile/mage@latest
```

### Available Mage Targets

```bash
# List all available targets
mage -l

# Build both operator and gateway-controller
mage build

# Build operator binary for current platform
mage buildOperatorBinary linux amd64 false

# Build gateway-controller binary
mage buildGatewayBinary linux amd64 false

# Build all platform binaries
mage buildOperatorBinaries
mage buildGatewayBinaries

# Build Docker images
mage buildImages
mage buildOperatorImage
mage buildGatewayImage

# Run tests
mage test
mage testCoverage

# Run linters
mage lint
mage vet
mage fmt

# Regenerate CRDs
mage regenCRD

# Package Helm chart
mage helmPackage
mage helmReindex

# Clean build artifacts
mage clean
```

### Example: Build for Multiple Platforms

```bash
# Build operator for all platforms
mage buildOperatorBinaries

# Binaries will be in dist/:
# - minio-operator-linux-amd64
# - minio-operator-linux-arm64
# - minio-operator-linux-s390x
# - minio-operator-linux-ppc64le
# - minio-operator-darwin-amd64
# - minio-operator-darwin-arm64
```

## Manual Development Workflow

If you prefer not to use Tilt, you can build and deploy manually.

### 1. Build Binaries

```bash
# Using Mage
mage buildOperatorBinary linux amd64 false
mage buildGatewayBinary linux amd64 false

# Or using Make
make binary
make gateway-binary

# Or manually
CGO_ENABLED=0 GOOS=linux GOARCH=amd64 go build -trimpath \
  -ldflags "-s -w -X github.com/minio/operator/pkg.Version=$(git describe --tags)" \
  -o minio-operator ./cmd/operator

CGO_ENABLED=0 GOOS=linux GOARCH=amd64 go build -trimpath \
  -ldflags "-s -w -X github.com/minio/operator/pkg.Version=$(git describe --tags)" \
  -o gateway-controller ./cmd/gateway-controller
```

### 2. Build Docker Images

```bash
# Using Mage
mage buildOperatorImage
mage buildGatewayImage

# Or using Make
make docker
make docker-gateway

# Or manually
docker build -t minio/operator:dev .
docker build -t minio/gateway-controller:dev -f Dockerfile.gateway .
```

### 3. Deploy to Kubernetes

```bash
# Install CRDs
kubectl apply -k resources/base/crds

# Install operator using Helm
helm install minio-operator helm/operator \
  --namespace minio-operator \
  --create-namespace \
  --set operator.image.tag=dev

# Or using kubectl
kubectl apply -k resources/base
```

### 4. Deploy Gateway Controller (Optional)

```bash
kubectl apply -f deploy/gateway-controller.yaml
```

## Development Workflows

### Workflow 1: Tilt (Recommended)

**Best for:** Active development with frequent code changes

```bash
# Start development environment
tilt up

# Make code changes
# Tilt automatically rebuilds and redeploys

# View logs in Tilt UI
open http://localhost:10350

# Stop when done
tilt down
```

### Workflow 2: Mage + Manual Deploy

**Best for:** Testing specific builds or CI/CD simulation

```bash
# Build binaries
mage build

# Build images
mage buildImages

# Deploy
helm upgrade --install minio-operator helm/operator \
  --namespace minio-operator \
  --create-namespace \
  --set operator.image.tag=$(git describe --tags)
```

### Workflow 3: Make (Traditional)

**Best for:** Compatibility with existing workflows

```bash
# Build and deploy operator
make build

# Build gateway controller
make docker-gateway

# Deploy
kubectl apply -k resources/base
```

## Testing

### Unit Tests

```bash
# Using Mage
mage test

# Using Make
make gotest

# Using go directly
go test -race ./...
```

### Integration Tests

```bash
# Run with coverage
mage testCoverage

# View coverage report
go tool cover -html=coverage.out
```

### End-to-End Tests

```bash
# Deploy a test tenant
kubectl apply -f examples/kustomization/tenant-lite/tenant.yaml

# Verify tenant is running
kubectl get tenant -n tenant-lite

# Check operator logs
kubectl logs -n minio-operator -l app=minio-operator -f
```

## Debugging

### Debug Operator Locally

```bash
# Run operator outside cluster
export KUBECONFIG=~/.kube/config
go run cmd/operator/main.go controller

# Or with delve debugger
dlv debug cmd/operator/main.go -- controller
```

### Debug Gateway Controller Locally

```bash
# Run gateway-controller outside cluster
export KUBECONFIG=~/.kube/config
go run cmd/gateway-controller/main.go \
  --kubeconfig=$KUBECONFIG \
  --debug-xds=true \
  --debug-ctrl=true

# Or with delve
dlv debug cmd/gateway-controller/main.go -- \
  --kubeconfig=$KUBECONFIG \
  --debug-xds=true
```

### Debug in Kubernetes

```bash
# Port forward to operator
kubectl port-forward -n minio-operator svc/minio-operator 8080:8080

# Access metrics
curl http://localhost:8080/metrics

# Port forward to gateway-controller
kubectl port-forward -n minio-operator svc/gateway-controller 18000:18000

# Check xDS configuration
kubectl port-forward -n minio-operator <envoy-pod> 9901:9901
curl http://localhost:9901/config_dump | jq
```

## Troubleshooting

### Tilt Issues

**Problem:** Tilt can't connect to Kubernetes
```bash
# Check kubectl context
kubectl config current-context

# Allow Tilt to use this context
tilt up --context=$(kubectl config current-context)
```

**Problem:** Docker build fails
```bash
# Check Docker is running
docker ps

# Clean Docker cache
docker system prune -a
```

### Build Issues

**Problem:** Mage not found
```bash
# Install mage
go install github.com/magefile/mage@latest

# Add to PATH
export PATH=$PATH:$(go env GOPATH)/bin
```

**Problem:** Dependencies not found
```bash
# Download dependencies
go mod download
go mod tidy
```

### Deployment Issues

**Problem:** CRDs not installed
```bash
# Regenerate and install CRDs
mage regenCRD
kubectl apply -k resources/base/crds
```

**Problem:** Image pull errors
```bash
# Check image exists
docker images | grep minio

# Load image into kind cluster
kind load docker-image minio/operator:dev

# Or for minikube
minikube image load minio/operator:dev
```

## Best Practices

1. **Use Tilt for active development** - fastest feedback loop
2. **Use Mage for CI/CD** - reproducible builds
3. **Run tests before committing** - `mage test`
4. **Format code** - `mage fmt`
5. **Run linters** - `mage lint`
6. **Regenerate CRDs after API changes** - `mage regenCRD`
7. **Keep dependencies updated** - `go mod tidy`

## Additional Resources

- [Tilt Documentation](https://docs.tilt.dev/)
- [Mage Documentation](https://magefile.org/)
- [MinIO Operator Documentation](https://min.io/docs/minio/kubernetes/upstream/)
- [Gateway Controller Setup](./gateway-controller-setup.md)
- [Multi-Tenant Retry Policy Example](./multi-tenant-retry-policy-example.md)

## Getting Help

- GitHub Issues: https://github.com/minio/operator/issues
- Slack: https://slack.min.io
- Documentation: https://min.io/docs/minio/kubernetes/upstream/