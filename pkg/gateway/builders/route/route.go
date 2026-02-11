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

package route

import (
	"time"

	corev3 "github.com/envoyproxy/go-control-plane/envoy/config/core/v3"
	routev3 "github.com/envoyproxy/go-control-plane/envoy/config/route/v3"
	matcherv3 "github.com/envoyproxy/go-control-plane/envoy/type/matcher/v3"
	"google.golang.org/protobuf/types/known/anypb"
	"google.golang.org/protobuf/types/known/durationpb"
	"google.golang.org/protobuf/types/known/wrapperspb"
)

// RouteBuilder is a struct used for building Envoy Route objects
type RouteBuilder struct {
	route *routev3.Route
}

// NewRedirectRouteBuilder creates a new RouteBuilder for a redirect route
func NewRedirectRouteBuilder(hostname string, redirectPath string, responseCode routev3.RedirectAction_RedirectResponseCode) *RouteBuilder {
	return &RouteBuilder{
		route: &routev3.Route{
			TypedPerFilterConfig: make(map[string]*anypb.Any),
			Action: &routev3.Route_Redirect{
				Redirect: &routev3.RedirectAction{
					SchemeRewriteSpecifier: &routev3.RedirectAction_SchemeRedirect{
						SchemeRedirect: "https",
					},
					HostRedirect: hostname,
					PathRewriteSpecifier: &routev3.RedirectAction_PathRedirect{
						PathRedirect: redirectPath,
					},
					ResponseCode: responseCode,
				},
			},
		},
	}
}

// NewRouteBuilder creates a new RouteBuilder for a regular route
func NewRouteBuilder() *RouteBuilder {
	return &RouteBuilder{
		route: &routev3.Route{
			TypedPerFilterConfig: make(map[string]*anypb.Any),
			Action: &routev3.Route_Route{
				Route: &routev3.RouteAction{},
			},
		},
	}
}

// WithPrefixMatch sets the prefix match for the route
func (rb *RouteBuilder) WithPrefixMatch(prefix string) *RouteBuilder {
	rb.route.Match = &routev3.RouteMatch{
		PathSpecifier: &routev3.RouteMatch_Prefix{
			Prefix: prefix,
		},
	}
	return rb
}

// WithRegexMatch sets the regex match for the route
func (rb *RouteBuilder) WithRegexMatch(regex string) *RouteBuilder {
	rb.route.Match = &routev3.RouteMatch{
		PathSpecifier: &routev3.RouteMatch_SafeRegex{
			SafeRegex: &matcherv3.RegexMatcher{
				Regex:      regex,
				EngineType: &matcherv3.RegexMatcher_GoogleRe2{},
			},
		},
	}
	return rb
}

// WithPathMatch sets the exact path match for the route
func (rb *RouteBuilder) WithPathMatch(path string) *RouteBuilder {
	rb.route.Match = &routev3.RouteMatch{
		PathSpecifier: &routev3.RouteMatch_Path{
			Path: path,
		},
	}
	return rb
}

// WithClusterName sets the cluster name for the route
func (rb *RouteBuilder) WithClusterName(clusterName string) *RouteBuilder {
	rb.route.GetRoute().ClusterSpecifier = &routev3.RouteAction_Cluster{
		Cluster: clusterName,
	}
	return rb
}

// WithTimeout sets the timeout for the route
func (rb *RouteBuilder) WithTimeout(timeout time.Duration) *RouteBuilder {
	rb.route.GetRoute().Timeout = durationpb.New(timeout)
	return rb
}

// WithPrefixRewrite sets the prefix rewrite for the route
func (rb *RouteBuilder) WithPrefixRewrite(prefix string) *RouteBuilder {
	rb.route.GetRoute().PrefixRewrite = prefix
	return rb
}

// WithUpgradeWebsocket enables WebSocket upgrade for the route
func (rb *RouteBuilder) WithUpgradeWebsocket() *RouteBuilder {
	rb.route.GetRoute().UpgradeConfigs = []*routev3.RouteAction_UpgradeConfig{{
		UpgradeType: "websocket",
	}}
	return rb
}

// WithHostRewrite sets the host rewrite for the route
func (rb *RouteBuilder) WithHostRewrite(host string) *RouteBuilder {
	rb.route.GetRoute().HostRewriteSpecifier = &routev3.RouteAction_HostRewriteLiteral{
		HostRewriteLiteral: host,
	}
	return rb
}

// WithRequestHeadersToAdd sets the request headers to add for the route
func (rb *RouteBuilder) WithRequestHeadersToAdd(headers map[string]string) *RouteBuilder {
	for k, v := range headers {
		rb.route.RequestHeadersToAdd = append(rb.route.RequestHeadersToAdd, &corev3.HeaderValueOption{
			Header: &corev3.HeaderValue{
				Key:   k,
				Value: v,
			},
			AppendAction: corev3.HeaderValueOption_OVERWRITE_IF_EXISTS_OR_ADD,
		})
	}
	return rb
}

// WithResponseHeadersToAdd sets the response headers to add for the route
func (rb *RouteBuilder) WithResponseHeadersToAdd(headers map[string]string) *RouteBuilder {
	for k, v := range headers {
		rb.route.ResponseHeadersToAdd = append(rb.route.ResponseHeadersToAdd, &corev3.HeaderValueOption{
			Header: &corev3.HeaderValue{
				Key:   k,
				Value: v,
			},
			AppendAction: corev3.HeaderValueOption_OVERWRITE_IF_EXISTS_OR_ADD,
		})
	}
	return rb
}

// WithRequestHeadersToRemove sets the request headers to remove for the route
func (rb *RouteBuilder) WithRequestHeadersToRemove(headers []string) *RouteBuilder {
	rb.route.RequestHeadersToRemove = append(rb.route.RequestHeadersToRemove, headers...)
	return rb
}

// WithResponseHeadersToRemove sets the response headers to remove for the route
func (rb *RouteBuilder) WithResponseHeadersToRemove(headers []string) *RouteBuilder {
	rb.route.ResponseHeadersToRemove = append(rb.route.ResponseHeadersToRemove, headers...)
	return rb
}

// WithRetryPolicy sets the retry policy for the route
func (rb *RouteBuilder) WithRetryPolicy(numRetries uint32, retryOn string) *RouteBuilder {
	rb.route.GetRoute().RetryPolicy = &routev3.RetryPolicy{
		NumRetries:    &wrapperspb.UInt32Value{Value: numRetries},
		RetryOn:       retryOn,
		PerTryTimeout: durationpb.New(5 * time.Second),
	}
	return rb
}

// Build builds the route and returns the Envoy Route object
func (rb *RouteBuilder) Build() (*routev3.Route, error) {
	return rb.route, nil
}

// Made with Bob
