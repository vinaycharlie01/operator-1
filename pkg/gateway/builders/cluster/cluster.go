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

package cluster

import (
	"time"

	clusterv3 "github.com/envoyproxy/go-control-plane/envoy/config/cluster/v3"
	corev3 "github.com/envoyproxy/go-control-plane/envoy/config/core/v3"
	endpointv3 "github.com/envoyproxy/go-control-plane/envoy/config/endpoint/v3"
	tlsv3 "github.com/envoyproxy/go-control-plane/envoy/extensions/transport_sockets/tls/v3"
	httpv3 "github.com/envoyproxy/go-control-plane/envoy/extensions/upstreams/http/v3"
	"google.golang.org/protobuf/types/known/anypb"
	"google.golang.org/protobuf/types/known/durationpb"
)

// ClusterBuilder is a builder for Envoy Cluster objects
type ClusterBuilder struct {
	cluster *clusterv3.Cluster

	tlsContext          *tlsv3.UpstreamTlsContext
	httpProtocolOptions *httpv3.HttpProtocolOptions
}

// NewClusterBuilder creates a new ClusterBuilder with the given name
func NewClusterBuilder(name string) *ClusterBuilder {
	return &ClusterBuilder{
		cluster: &clusterv3.Cluster{
			Name:                 name,
			ConnectTimeout:       durationpb.New(5 * time.Second),
			ClusterDiscoveryType: &clusterv3.Cluster_Type{Type: clusterv3.Cluster_STRICT_DNS},
			LbPolicy:             clusterv3.Cluster_ROUND_ROBIN,
			DnsLookupFamily:      clusterv3.Cluster_V4_ONLY,
		},
	}
}

// WithConnectTimeout sets the connect timeout for the cluster
func (cb *ClusterBuilder) WithConnectTimeout(timeout time.Duration) *ClusterBuilder {
	cb.cluster.ConnectTimeout = durationpb.New(timeout)
	return cb
}

// WithClusterDiscoveryType sets the cluster discovery type for the cluster
func (cb *ClusterBuilder) WithClusterDiscoveryType(discoveryType clusterv3.Cluster_DiscoveryType) *ClusterBuilder {
	cb.cluster.ClusterDiscoveryType = &clusterv3.Cluster_Type{
		Type: discoveryType,
	}
	return cb
}

// WithLbPolicy sets the load balancing policy for the cluster
func (cb *ClusterBuilder) WithLbPolicy(policy clusterv3.Cluster_LbPolicy) *ClusterBuilder {
	cb.cluster.LbPolicy = policy
	return cb
}

// WithDnsLookupFamily sets the DNS lookup family for the cluster
func (cb *ClusterBuilder) WithDnsLookupFamily(family clusterv3.Cluster_DnsLookupFamily) *ClusterBuilder {
	cb.cluster.DnsLookupFamily = family
	return cb
}

// WithLoadAssignment sets the load assignment for the cluster
func (cb *ClusterBuilder) WithLoadAssignment(hostname string, port uint32) *ClusterBuilder {
	cb.cluster.LoadAssignment = &endpointv3.ClusterLoadAssignment{
		ClusterName: cb.cluster.Name,
		Endpoints: []*endpointv3.LocalityLbEndpoints{{
			LbEndpoints: []*endpointv3.LbEndpoint{{
				HostIdentifier: &endpointv3.LbEndpoint_Endpoint{
					Endpoint: &endpointv3.Endpoint{
						Address: &corev3.Address{
							Address: &corev3.Address_SocketAddress{
								SocketAddress: &corev3.SocketAddress{
									Address: hostname,
									PortSpecifier: &corev3.SocketAddress_PortValue{
										PortValue: port,
									},
								},
							},
						},
					},
				},
			}},
		}},
	}
	return cb
}

// WithHTTP2ProtocolOptions enables HTTP/2 for the cluster
func (cb *ClusterBuilder) WithHTTP2ProtocolOptions() *ClusterBuilder {
	if cb.httpProtocolOptions == nil {
		cb.httpProtocolOptions = &httpv3.HttpProtocolOptions{}
	}
	cb.httpProtocolOptions.UpstreamProtocolOptions = &httpv3.HttpProtocolOptions_ExplicitHttpConfig_{
		ExplicitHttpConfig: &httpv3.HttpProtocolOptions_ExplicitHttpConfig{
			ProtocolConfig: &httpv3.HttpProtocolOptions_ExplicitHttpConfig_Http2ProtocolOptions{
				Http2ProtocolOptions: &corev3.Http2ProtocolOptions{},
			},
		},
	}
	return cb
}

// WithUpstreamTLS enables TLS for upstream connections
func (cb *ClusterBuilder) WithUpstreamTLS(sni string, certFile string) *ClusterBuilder {
	cb.tlsContext = &tlsv3.UpstreamTlsContext{
		Sni: sni,
		CommonTlsContext: &tlsv3.CommonTlsContext{
			ValidationContextType: &tlsv3.CommonTlsContext_ValidationContext{
				ValidationContext: &tlsv3.CertificateValidationContext{
					TrustedCa: &corev3.DataSource{
						Specifier: &corev3.DataSource_Filename{
							Filename: certFile,
						},
					},
				},
			},
		},
	}
	return cb
}

// WithUpstreamTLSNoVerify enables TLS for upstream connections without certificate verification
func (cb *ClusterBuilder) WithUpstreamTLSNoVerify(sni string) *ClusterBuilder {
	cb.tlsContext = &tlsv3.UpstreamTlsContext{
		Sni: sni,
		CommonTlsContext: &tlsv3.CommonTlsContext{
			ValidationContextType: &tlsv3.CommonTlsContext_ValidationContext{
				ValidationContext: &tlsv3.CertificateValidationContext{
					TrustChainVerification: tlsv3.CertificateValidationContext_ACCEPT_UNTRUSTED,
				},
			},
		},
	}
	return cb
}

// Build returns the built cluster
func (cb *ClusterBuilder) Build() (*clusterv3.Cluster, error) {
	if cb.tlsContext != nil {
		tlsAny, err := anypb.New(cb.tlsContext)
		if err != nil {
			return nil, err
		}

		cb.cluster.TransportSocket = &corev3.TransportSocket{
			Name: "envoy.transport_sockets.tls",
			ConfigType: &corev3.TransportSocket_TypedConfig{
				TypedConfig: tlsAny,
			},
		}
	}

	if cb.httpProtocolOptions != nil {
		httpOptionsAny, err := anypb.New(cb.httpProtocolOptions)
		if err != nil {
			return nil, err
		}

		cb.cluster.TypedExtensionProtocolOptions = map[string]*anypb.Any{
			"envoy.extensions.upstreams.http.v3.HttpProtocolOptions": httpOptionsAny,
		}
	}

	return cb.cluster, nil
}

// Made with Bob
