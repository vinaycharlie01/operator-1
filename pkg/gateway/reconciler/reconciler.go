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

package reconciler

import (
	"context"
	"fmt"

	cachev3 "github.com/envoyproxy/go-control-plane/pkg/cache/v3"
	miniov2 "github.com/minio/operator/pkg/apis/minio.min.io/v2"
	"github.com/minio/operator/pkg/gateway/snapshot"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/client-go/rest"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/log"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"
)

const WatchNamespaceEnvVar = "WATCH_NAMESPACES"

// XDSReconciler reconciles MinIO Tenant objects and updates xDS snapshots
type XDSReconciler struct {
	client.Client
	SnapshotCache      cachev3.SnapshotCache
	InCluster          bool
	Config             *rest.Config
	Scheme             *runtime.Scheme
	XDSServerNamespace string
	ClusterDomain      string
}

// Reconcile is part of the main kubernetes reconciliation loop which aims to
// move the current state of the cluster closer to the desired state.
func (r *XDSReconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	logger := log.FromContext(ctx)

	logger.Info("fetching tenant", "namespace", req.Namespace, "name", req.Name)
	tenant := &miniov2.Tenant{}
	if err := r.Get(ctx, req.NamespacedName, tenant); err != nil {
		if apierrors.IsNotFound(err) {
			logger.Info("tenant not found, may have been deleted")
			return reconcile.Result{}, nil
		}
		return reconcile.Result{}, err
	}

	logger.Info("tenant found", "location", fmt.Sprintf("%s/%s", tenant.Namespace, tenant.Name), "pools", len(tenant.Spec.Pools))

	// Don't process tenants that are being deleted
	if !tenant.DeletionTimestamp.IsZero() {
		logger.Info("tenant is being deleted, skipping reconciliation")
		return reconcile.Result{}, nil
	}

	// Build snapshot cache using tenant
	nodeID := fmt.Sprintf("gateway-%s-%s", tenant.Namespace, tenant.Name)
	if err := r.updateSnapshotCache(ctx, nodeID, *tenant); err != nil {
		logger.Error(err, "failed to update snapshot cache")
		return reconcile.Result{}, err
	}

	return reconcile.Result{}, nil
}

// updateSnapshotCache updates the XDS snapshot cache with a new snapshot built from the passed Tenant.
func (r *XDSReconciler) updateSnapshotCache(ctx context.Context, nodeID string, tenant miniov2.Tenant) error {
	logger := log.FromContext(ctx)

	logger.Info("building snapshot for tenant")

	sb := snapshot.NewSnapshotBuilder(tenant, r.ClusterDomain)
	snap, err := sb.Build()
	if err != nil {
		return fmt.Errorf("failed to build snapshot: %w", err)
	}
	logger.Info("built snapshot")

	if err := snap.Consistent(); err != nil {
		return fmt.Errorf("snapshot inconsistent: %w", err)
	}
	logger.Info("snapshot consistent")

	if err := r.SnapshotCache.SetSnapshot(ctx, nodeID, snap); err != nil {
		return fmt.Errorf("failed to set snapshot: %w", err)
	}
	logger.Info("snapshot updated", "nodeID", nodeID, "version", sb.SnapshotVersion)

	return nil
}

// Made with Bob

// SetupWithManager sets up the controller with the Manager.
func (r *XDSReconciler) SetupWithManager(mgr ctrl.Manager) error {
	return ctrl.NewControllerManagedBy(mgr).
		For(&miniov2.Tenant{}).
		Complete(r)
}
