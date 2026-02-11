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

package snapshot

import (
	"fmt"
	"strconv"
	"time"

	clusterv3 "github.com/envoyproxy/go-control-plane/envoy/config/cluster/v3"
	listenerv3 "github.com/envoyproxy/go-control-plane/envoy/config/listener/v3"
	routev3 "github.com/envoyproxy/go-control-plane/envoy/config/route/v3"
	tlsv3 "github.com/envoyproxy/go-control-plane/envoy/extensions/transport_sockets/tls/v3"
	"github.com/envoyproxy/go-control-plane/pkg/cache/types"
	cachev3 "github.com/envoyproxy/go-control-plane/pkg/cache/v3"
	resourcev3 "github.com/envoyproxy/go-control-plane/pkg/resource/v3"
	miniov2 "github.com/minio/operator/pkg/apis/minio.min.io/v2"
	"github.com/minio/operator/pkg/gateway/builders/listener"
	"github.com/minio/operator/pkg/gateway/builders/route"
	"github.com/minio/operator/pkg/gateway/builders/upstream"
)

const (
	// ListenerName is the name of the main listener
	ListenerName = "minio_listener"
	// RouteName is the name of the route configuration
	RouteName = "minio_route"
	// XDSClusterName is the name used for xDS configuration
	XDSClusterName = "xds_cluster"
)

// SnapshotBuilder builds Envoy xDS snapshots from MinIO Tenant resources
type SnapshotBuilder struct {
	SnapshotVersion string
	tenant          miniov2.Tenant
	clusterDomain   string
	clusters        []*clusterv3.Cluster
	listeners       []*listenerv3.Listener
	routeConfigs    []*routev3.RouteConfiguration
	resources       map[string][]types.Resource
}

// NewSnapshotBuilder creates a new snapshot builder
func NewSnapshotBuilder(tenant miniov2.Tenant, clusterDomain string) *SnapshotBuilder {
	return &SnapshotBuilder{
		tenant:        tenant,
		clusterDomain: clusterDomain,
	}
}

// Build builds the xDS snapshot
func (sb *SnapshotBuilder) Build() (*cachev3.Snapshot, error) {
	if err := sb.buildResources(); err != nil {
		return nil, err
	}

	// use current Unix time in microseconds as snapshot version
	sb.SnapshotVersion = strconv.FormatInt(time.Now().UnixMicro(), 10)

	return cachev3.NewSnapshot(sb.SnapshotVersion, sb.resources)
}

func (sb *SnapshotBuilder) buildResources() error {
	if err := sb.buildClusters(); err != nil {
		return err
	}
	if err := sb.buildListeners(); err != nil {
		return err
	}
	if err := sb.buildRouteConfigs(); err != nil {
		return err
	}

	resources := map[string][]types.Resource{
		resourcev3.ClusterType:  convertToResources(sb.clusters),
		resourcev3.RouteType:    convertToResources(sb.routeConfigs),
		resourcev3.ListenerType: convertToResources(sb.listeners),
	}

	sb.resources = resources
	return nil
}

func (sb *SnapshotBuilder) buildClusters() error {
	// Use upstream builder to create clusters
	upstreams := upstream.NewUpstreams(sb.tenant, sb.clusterDomain)
	clusters, err := upstreams.GetClusters()
	if err != nil {
		return fmt.Errorf("failed to build clusters: %w", err)
	}

	sb.clusters = clusters
	return nil
}

func (sb *SnapshotBuilder) buildListeners() error {
	// Create main HTTP listener
	lb := listener.NewListenerBuilder(ListenerName).
		WithAddressAndPort("0.0.0.0", 10000).
		WithHCM("minio_ingress", RouteName, XDSClusterName)

	// Add TLS if configured
	if sb.tenant.Spec.RequestAutoCert != nil && *sb.tenant.Spec.RequestAutoCert {
		lb = lb.WithTLSTransportSocket(
			"/etc/envoy/certs/tls.crt",
			"/etc/envoy/certs/tls.key",
			tlsv3.TlsParameters_TLSv1_2,
			tlsv3.TlsParameters_TLSv1_3,
		)
	}

	l, err := lb.Build()
	if err != nil {
		return fmt.Errorf("failed to build listener: %w", err)
	}

	sb.listeners = []*listenerv3.Listener{l}
	return nil
}

func (sb *SnapshotBuilder) buildRouteConfigs() error {
	// Build main route for MinIO
	minioRoute, err := route.NewRouteBuilder().
		WithPrefixMatch("/").
		WithClusterName("minio_cluster").
		WithTimeout(300 * time.Second).
		Build()
	if err != nil {
		return fmt.Errorf("failed to build MinIO route: %w", err)
	}

	// Create virtual host
	virtualHost := &routev3.VirtualHost{
		Name:    "minio_service",
		Domains: []string{"*"},
		Routes:  []*routev3.Route{minioRoute},
	}

	// Create route configuration
	routeConfig := &routev3.RouteConfiguration{
		Name:         RouteName,
		VirtualHosts: []*routev3.VirtualHost{virtualHost},
	}

	sb.routeConfigs = []*routev3.RouteConfiguration{routeConfig}
	return nil
}

func convertToResources[T types.Resource](items []T) []types.Resource {
	resources := make([]types.Resource, len(items))
	for i, item := range items {
		resources[i] = item
	}
	return resources
}

// Made with Bob
