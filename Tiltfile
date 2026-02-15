trigger_mode(TRIGGER_MODE_MANUAL)
allow_k8s_contexts(k8s_context())
load('ext://namespace', 'namespace_create')
load('ext://secret', 'secret_yaml_docker_registry')
load('ext://local_output', 'local_output')
load('ext://dotenv', 'dotenv')
load('ext://helm_resource', 'helm_resource')

# Load environment variables from .env file
dotenv(fn='.env')

# Ensure dist directory exists and build binaries initially
local('mkdir -p dist')
local('mage buildOperatorBinary')
# local('mage buildGatewayBinary')

# Get version from mage build
image_tag = "dev"

content = local_output('mage buildOperatorBinary')

for line in content.split('\n'):
    if "version" in line and ":" in line:
        image_tag = line.split(':')[1].strip().replace('"', '')
        break

print("Using version tag:", image_tag)

# Compile operator binary using mage
local_resource('operator-compile',
  'mage buildOperatorBinary',
  ignore=[
    'dist',
    'mage_output_file.go',
    'helm',
    'examples',
    'docs',
    'testing'
  ],
  deps=[
    'cmd/operator/',
    'pkg/',
    'resources/',
    'go.mod',
    'go.sum'
  ],
  labels = ['minio-operator-binaries'],
  trigger_mode=TRIGGER_MODE_AUTO
)

# Compile gateway-controller binary using mage
local_resource('gateway-controller-compile',
  'mage buildGatewayBinary',
  ignore=[
    'dist',
    'mage_output_file.go',
    'helm',
    'examples',
    'docs',
    'testing'
  ],
  deps=[
    'cmd/gateway-controller/',
    'pkg/gateway/',
    'go.mod',
    'go.sum'
  ],
  labels = ['minio-operator-binaries'],
  trigger_mode=TRIGGER_MODE_AUTO
)
os.getenv("DOCKER_USERNAME")
os.getenv("DOCKER_REGISTRY")

docker_username = os.getenv("DOCKER_USERNAME")
docker_password = os.getenv("DOCKER_PASSWORD")
docker_registry = os.getenv("DOCKER_REGISTRY")

print('Docker Registry: ' + docker_registry)
print('GitHub User: ' + docker_username)

if not docker_password:
    fail('DOCKER_PASSWORD is required. Set it in .env file or export DOCKER_PASSWORD=<github-token>')


print('Docker Password: ***configured***')

# Build operator Docker image
docker_build(
    docker_registry+'/'+ docker_username + '/minio-operator',
    '.',
    dockerfile='Dockerfile',
    platform='linux/amd64',
    extra_tag=image_tag
)

# # Build gateway-controller Docker image
docker_build(
    docker_registry+'/'+ docker_username  + '/gateway-controller',
    '.',
    dockerfile='Dockerfile.gateway',
    platform='linux/amd64',
    extra_tag=image_tag
)

# Create namespace for development
namespace_create('minio-operator')

k8s_resource(
  new_name = 'namespaces',
  objects = [
    'minio-operator:namespace',
  ],
  labels = ['minio-operator']
)

# Create Docker registry secret if credentials provided
if docker_password:
    k8s_yaml(secret_yaml_docker_registry(
        name='minio-registry',
        username=docker_username,
        password=docker_password,
        server=docker_registry,
        namespace='minio-operator'
    ))
    
    k8s_resource(
      new_name = 'minio-registry',
      objects = [
        'minio-registry:secret',
      ],
      labels = ['minio-operator']
    )

# Deploy MinIO Operator using helm_resource for proper image tracking
helm_resource(
    name='minio-operator',
    chart='helm/operator',
    namespace='minio-operator',
    image_deps=[
        docker_registry+'/'+ docker_username + '/minio-operator',
        docker_registry+'/'+ docker_username + '/gateway-controller',
    ],
    image_keys=[
        ('operator.image.repository', 'operator.image.tag'),
        ('operator.gatewayImage.repository', 'operator.gatewayImage.tag'),
    ],
    flags=[
        '--set=operator.image.pullPolicy=Always',
        '--set=operator.gatewayImage.pullPolicy=Always',
        '--set=operator.replicaCount=1',
        '--set=operator.imagePullSecrets[0].name=minio-registry',
    ],
    resource_deps=['operator-compile', 'gateway-controller-compile'],
)

# Deploy tenant using helm_resource
helm_resource(
    name='tenant',
    chart='helm/tenant',
    namespace='tenant-ns',
    flags=[
        '--create-namespace',
    ],
    resource_deps=['minio-operator'],
)

# Port forwards for operator deployment (managed by helm_resource)
k8s_resource(
    workload='minio-operator',
    port_forwards=[
        port_forward(9443, 9443, name='webhook'),
        port_forward(8080, 8080, name='metrics')
    ],
    labels=['minio-operator'],
)


