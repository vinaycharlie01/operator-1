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

package xdsserver

import (
	"context"
	"fmt"
	"net"
	"time"

	"google.golang.org/grpc"
	"google.golang.org/grpc/keepalive"

	clusterservice "github.com/envoyproxy/go-control-plane/envoy/service/cluster/v3"
	endpointservice "github.com/envoyproxy/go-control-plane/envoy/service/endpoint/v3"
	listenerservice "github.com/envoyproxy/go-control-plane/envoy/service/listener/v3"
	routeservice "github.com/envoyproxy/go-control-plane/envoy/service/route/v3"
	"github.com/envoyproxy/go-control-plane/pkg/cache/v3"
	"github.com/envoyproxy/go-control-plane/pkg/server/v3"
	"github.com/envoyproxy/go-control-plane/pkg/test/v3"
	"github.com/go-logr/logr"
)

const (
	grpcKeepaliveTime        = 30 * time.Second
	grpcKeepaliveTimeout     = 5 * time.Second
	grpcKeepaliveMinTime     = 30 * time.Second
	grpcMaxConcurrentStreams = 1000000
)

// Server represents the xDS server for Envoy configuration
type Server struct {
	xdsServer server.Server
	logger    logr.Logger

	grpcServer *grpc.Server
}

// NewServer creates a new xDS server instance
func NewServer(ctx context.Context, cache cache.Cache, l logr.Logger, cb *test.Callbacks) *Server {
	return &Server{
		xdsServer: server.NewServer(ctx, cache, cb),
		logger:    l,
	}
}

// registerServer registers this XDS and gRPC server with EDS, CDS, RDS and LDS discovery services
func (s *Server) registerServer() {
	endpointservice.RegisterEndpointDiscoveryServiceServer(s.grpcServer, s.xdsServer)
	clusterservice.RegisterClusterDiscoveryServiceServer(s.grpcServer, s.xdsServer)
	routeservice.RegisterRouteDiscoveryServiceServer(s.grpcServer, s.xdsServer)
	listenerservice.RegisterListenerDiscoveryServiceServer(s.grpcServer, s.xdsServer)
}

// Run starts the XDS server
func (s *Server) Run(port uint) error {
	grpcOptions := []grpc.ServerOption{
		grpc.MaxConcurrentStreams(grpcMaxConcurrentStreams),
		grpc.KeepaliveParams(keepalive.ServerParameters{
			Time:    grpcKeepaliveTime,
			Timeout: grpcKeepaliveTimeout,
		}),
		grpc.KeepaliveEnforcementPolicy(keepalive.EnforcementPolicy{
			MinTime:             grpcKeepaliveMinTime,
			PermitWithoutStream: true,
		}),
	}
	s.grpcServer = grpc.NewServer(grpcOptions...)

	lis, err := net.Listen("tcp", fmt.Sprintf(":%d", port))
	if err != nil {
		return fmt.Errorf("error creating xds server listener: %w", err)
	}

	s.registerServer()

	s.logger.Info("XDS server listening", "port", port)
	if err := s.grpcServer.Serve(lis); err != nil {
		return fmt.Errorf("error while serving xds server: %w", err)
	}
	return nil
}

// GracefulStop gracefully stops the server
func (s *Server) GracefulStop() {
	s.logger.Info("gracefully shutting down server")
	s.grpcServer.GracefulStop()
}

// Made with Bob
