// Copyright (C) 2024, MinIO, Inc.
//
// This code is free software: you can redistribute it and/or modify
// it under the terms of the GNU Affero General Public License, version 3,
// as published by the Free Software Foundation.
//
// This program is distributed in the hope that it will be useful,
// but WITHOUT ANY WARRANTY; without even the implied warranty of
// MERCHANTABILITY or FITNESS FOR A PARTICULAR PURPOSE. See the
// GNU Affero General Public License for more details.
//
// You should have received a copy of the GNU Affero General Public License, version 3,
// along with this program.  If not, see <http://www.gnu.org/licenses/>

package upstream

import (
	"fmt"

	clusterv3 "github.com/envoyproxy/go-control-plane/envoy/config/cluster/v3"
	miniov2 "github.com/minio/operator/pkg/apis/minio.min.io/v2"
	"github.com/minio/operator/pkg/gateway/builders/cluster"
)

// Upstreams manages the creation of Envoy clusters for MinIO tenants
type Upstreams struct {
	tenant        miniov2.Tenant
	clusterDomain string
}

// NewUpstreams creates a new Upstreams instance
func NewUpstreams(tenant miniov2.Tenant, clusterDomain string) *Upstreams {
	return &Upstreams{
		tenant:        tenant,
		clusterDomain: clusterDomain,
	}
}

// GetClusters returns all clusters for the MinIO tenant
func (u *Upstreams) GetClusters() ([]*clusterv3.Cluster, error) {
	var clusters []*clusterv3.Cluster

	// Create main MinIO cluster for S3 API
	minioCluster, err := u.buildMinIOCluster()
	if err != nil {
		return nil, fmt.Errorf("failed to build MinIO cluster: %w", err)
	}
	clusters = append(clusters, minioCluster)

	// Create MinIO Console cluster for web UI
	consoleCluster, err := u.buildConsoleCluster()
	if err != nil {
		return nil, fmt.Errorf("failed to build Console cluster: %w", err)
	}
	clusters = append(clusters, consoleCluster)

	return clusters, nil
}

// buildMinIOCluster builds the main MinIO cluster
func (u *Upstreams) buildMinIOCluster() (*clusterv3.Cluster, error) {
	// Use headless service for MinIO
	serviceName := fmt.Sprintf("%s-hl", u.tenant.Name)
	hostname := fmt.Sprintf("%s.%s.svc.%s", serviceName, u.tenant.Namespace, u.clusterDomain)

	// MinIO default port
	port := uint32(9000)

	// Build cluster using cluster builder
	cb := cluster.NewClusterBuilder("minio_cluster").
		WithLoadAssignment(hostname, port).
		WithClusterDiscoveryType(clusterv3.Cluster_STRICT_DNS).
		WithLbPolicy(clusterv3.Cluster_ROUND_ROBIN).
		WithDnsLookupFamily(clusterv3.Cluster_V4_ONLY)

	// Enable HTTP/2 if needed
	if u.shouldUseHTTP2() {
		cb = cb.WithHTTP2ProtocolOptions()
	}

	// Enable TLS if configured
	if u.tenant.Spec.RequestAutoCert != nil && *u.tenant.Spec.RequestAutoCert {
		// Use the MinIO tenant's public certificate mounted in Envoy
		// Note: Using public.crt as ca.crt doesn't exist in minio-tls secret
		cb = cb.WithUpstreamTLS(hostname, "/etc/envoy/minio-certs/public.crt")
	}

	return cb.Build()
}

// buildConsoleCluster builds the MinIO Console cluster for web UI
func (u *Upstreams) buildConsoleCluster() (*clusterv3.Cluster, error) {
	// Use console service
	serviceName := fmt.Sprintf("%s-console", u.tenant.Name)
	hostname := fmt.Sprintf("%s.%s.svc.%s", serviceName, u.tenant.Namespace, u.clusterDomain)

	// Console port (HTTPS)
	port := uint32(9443)

	// Build cluster using cluster builder
	cb := cluster.NewClusterBuilder("console_cluster").
		WithLoadAssignment(hostname, port).
		WithClusterDiscoveryType(clusterv3.Cluster_STRICT_DNS).
		WithLbPolicy(clusterv3.Cluster_ROUND_ROBIN).
		WithDnsLookupFamily(clusterv3.Cluster_V4_ONLY)

	// Console uses HTTPS on port 9443
	// Enable TLS without certificate verification (internal service)
	if u.tenant.Spec.RequestAutoCert != nil && *u.tenant.Spec.RequestAutoCert {
		cb = cb.WithHTTP2ProtocolOptions().
			WithUpstreamTLSNoVerify(hostname)
	}

	return cb.Build()
}

// shouldUseHTTP2 determines if HTTP/2 should be used
func (u *Upstreams) shouldUseHTTP2() bool {
	// Enable HTTP/2 if TLS is enabled
	return u.tenant.Spec.RequestAutoCert != nil && *u.tenant.Spec.RequestAutoCert
}

// GetClusterNames returns the names of all clusters
func (u *Upstreams) GetClusterNames() []string {
	// Only MinIO cluster is needed for S3 API routing
	return []string{"minio_cluster"}
}

// Made with Bob
