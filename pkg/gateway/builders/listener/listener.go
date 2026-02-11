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

package listener

import (
	"time"

	accesslogv3 "github.com/envoyproxy/go-control-plane/envoy/config/accesslog/v3"
	corev3 "github.com/envoyproxy/go-control-plane/envoy/config/core/v3"
	listenerv3 "github.com/envoyproxy/go-control-plane/envoy/config/listener/v3"
	fileaccesslogv3 "github.com/envoyproxy/go-control-plane/envoy/extensions/access_loggers/file/v3"
	routerv3 "github.com/envoyproxy/go-control-plane/envoy/extensions/filters/http/router/v3"
	hcmv3 "github.com/envoyproxy/go-control-plane/envoy/extensions/filters/network/http_connection_manager/v3"
	tlsv3 "github.com/envoyproxy/go-control-plane/envoy/extensions/transport_sockets/tls/v3"
	"github.com/envoyproxy/go-control-plane/pkg/resource/v3"
	"google.golang.org/protobuf/types/known/anypb"
	"google.golang.org/protobuf/types/known/durationpb"
	"google.golang.org/protobuf/types/known/wrapperspb"
)

// ListenerBuilder is a builder for Envoy Listener objects
type ListenerBuilder struct {
	listener *listenerv3.Listener

	httpConnManager *hcmv3.HttpConnectionManager
	tlsContext      *tlsv3.DownstreamTlsContext
}

// NewListenerBuilder creates a new ListenerBuilder with the given name
func NewListenerBuilder(name string) *ListenerBuilder {
	return &ListenerBuilder{
		listener: &listenerv3.Listener{
			Name: name,
			SocketOptions: []*corev3.SocketOption{
				{
					Description: "Enable TCP keep-alive",
					Level:       1,
					Name:        9,
					Value: &corev3.SocketOption_IntValue{
						IntValue: 1,
					},
					State: corev3.SocketOption_STATE_LISTENING,
				},
				{
					Description: "TCP keep-alive initial idle time",
					Level:       6,
					Name:        4,
					Value: &corev3.SocketOption_IntValue{
						IntValue: 45,
					},
					State: corev3.SocketOption_STATE_LISTENING,
				},
			},
		},
	}
}

// WithAddressAndPort sets the address and port for the listener
func (lb *ListenerBuilder) WithAddressAndPort(address string, port uint32) *ListenerBuilder {
	lb.listener.Address = &corev3.Address{
		Address: &corev3.Address_SocketAddress{
			SocketAddress: &corev3.SocketAddress{
				Address: address,
				PortSpecifier: &corev3.SocketAddress_PortValue{
					PortValue: port,
				},
			},
		},
	}
	return lb
}

// WithHCM adds a HttpConnectionManager to the listener
func (lb *ListenerBuilder) WithHCM(statPrefix string, routeConfigName string, xdsClusterName string) *ListenerBuilder {
	routerConfig, _ := anypb.New(&routerv3.Router{
		DynamicStats: wrapperspb.Bool(false),
	})

	fileAccessLogConfig, _ := anypb.New(&fileaccesslogv3.FileAccessLog{
		Path: "/dev/stdout",
	})

	lb.httpConnManager = &hcmv3.HttpConnectionManager{
		AccessLog: []*accesslogv3.AccessLog{{
			Name: "envoy.access_loggers.file",
			ConfigType: &accesslogv3.AccessLog_TypedConfig{
				TypedConfig: fileAccessLogConfig,
			},
		}},
		CodecType:         hcmv3.HttpConnectionManager_AUTO,
		StatPrefix:        statPrefix,
		NormalizePath:     wrapperspb.Bool(true),
		MergeSlashes:      true,
		GenerateRequestId: wrapperspb.Bool(true),
		UseRemoteAddress:  wrapperspb.Bool(true),
		CommonHttpProtocolOptions: &corev3.HttpProtocolOptions{
			IdleTimeout: durationpb.New(60 * time.Second),
		},
		Http2ProtocolOptions: &corev3.Http2ProtocolOptions{
			InitialStreamWindowSize:     wrapperspb.UInt32(65536),   // 64 KiB
			InitialConnectionWindowSize: wrapperspb.UInt32(1048576), // 1 MiB
		},
		RouteSpecifier: &hcmv3.HttpConnectionManager_Rds{
			Rds: &hcmv3.Rds{
				RouteConfigName: routeConfigName,
				ConfigSource: &corev3.ConfigSource{
					ResourceApiVersion: resource.DefaultAPIVersion,
					ConfigSourceSpecifier: &corev3.ConfigSource_ApiConfigSource{
						ApiConfigSource: &corev3.ApiConfigSource{
							TransportApiVersion:       resource.DefaultAPIVersion,
							ApiType:                   corev3.ApiConfigSource_GRPC,
							SetNodeOnFirstMessageOnly: true,
							GrpcServices: []*corev3.GrpcService{{
								TargetSpecifier: &corev3.GrpcService_EnvoyGrpc_{
									EnvoyGrpc: &corev3.GrpcService_EnvoyGrpc{
										ClusterName: xdsClusterName,
									},
								},
							}},
						},
					},
				},
			},
		},
		HttpFilters: []*hcmv3.HttpFilter{
			{
				Name: "envoy.filters.http.router",
				ConfigType: &hcmv3.HttpFilter_TypedConfig{
					TypedConfig: routerConfig,
				},
			},
		},
	}
	return lb
}

// WithTLSTransportSocket sets the TLS transport socket for the listener
func (lb *ListenerBuilder) WithTLSTransportSocket(certFile string, keyFile string, minVersion tlsv3.TlsParameters_TlsProtocol, maxVersion tlsv3.TlsParameters_TlsProtocol) *ListenerBuilder {
	lb.tlsContext = &tlsv3.DownstreamTlsContext{
		CommonTlsContext: &tlsv3.CommonTlsContext{
			TlsCertificates: []*tlsv3.TlsCertificate{{
				CertificateChain: &corev3.DataSource{
					Specifier: &corev3.DataSource_Filename{
						Filename: certFile,
					},
				},
				PrivateKey: &corev3.DataSource{
					Specifier: &corev3.DataSource_Filename{
						Filename: keyFile,
					},
				},
			}},
			TlsParams: &tlsv3.TlsParameters{
				TlsMinimumProtocolVersion: minVersion,
				TlsMaximumProtocolVersion: maxVersion,
			},
			AlpnProtocols: []string{"h2", "http/1.1"},
		},
	}
	return lb
}

// Build builds the listener object
func (lb *ListenerBuilder) Build() (*listenerv3.Listener, error) {
	var filterChain *listenerv3.FilterChain

	if lb.httpConnManager != nil {
		hcmAny, err := anypb.New(lb.httpConnManager)
		if err != nil {
			return nil, err
		}
		filterChain = &listenerv3.FilterChain{
			Name: lb.listener.Name,
			Filters: []*listenerv3.Filter{{
				Name: "envoy.filters.network.http_connection_manager",
				ConfigType: &listenerv3.Filter_TypedConfig{
					TypedConfig: hcmAny,
				},
			}},
		}
		if lb.tlsContext != nil {
			tlsCtxAny, err := anypb.New(lb.tlsContext)
			if err != nil {
				return nil, err
			}
			filterChain.TransportSocket = &corev3.TransportSocket{
				Name: "envoy.transport_sockets.tls",
				ConfigType: &corev3.TransportSocket_TypedConfig{
					TypedConfig: tlsCtxAny,
				},
			}
		}
	}

	lb.listener.FilterChains = []*listenerv3.FilterChain{filterChain}

	return lb.listener, nil
}

// Made with Bob
