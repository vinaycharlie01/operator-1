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

package controller

import (
	"context"
	"fmt"
	"os"

	miniov2 "github.com/minio/operator/pkg/apis/minio.min.io/v2"
	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	rbacv1 "k8s.io/api/rbac/v1"
	k8serrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/util/intstr"
	"k8s.io/klog/v2"
)

const (
	GatewayControllerName      = "gateway-controller"
	GatewayControllerNamespace = "minio-operator"
	GatewayControllerImage     = "ghcr.io/vinaycharlie01/gateway-controller:latest"
)

// ensureGatewayController ensures the gateway controller is deployed in the cluster
func (c *Controller) ensureGatewayController(ctx context.Context) error {
	// Check if gateway controller service account exists
	if err := c.ensureGatewayControllerServiceAccount(ctx); err != nil {
		return err
	}

	// Check if gateway controller cluster role exists
	if err := c.ensureGatewayControllerClusterRole(ctx); err != nil {
		return err
	}

	// Check if gateway controller cluster role binding exists
	if err := c.ensureGatewayControllerClusterRoleBinding(ctx); err != nil {
		return err
	}

	// Check if gateway controller deployment exists
	if err := c.ensureGatewayControllerDeployment(ctx); err != nil {
		return err
	}

	// Check if gateway controller service exists
	if err := c.ensureGatewayControllerService(ctx); err != nil {
		return err
	}

	return nil
}

// ensureGatewayControllerServiceAccount creates the service account for gateway controller
func (c *Controller) ensureGatewayControllerServiceAccount(ctx context.Context) error {
	sa := &corev1.ServiceAccount{
		ObjectMeta: metav1.ObjectMeta{
			Name:      GatewayControllerName,
			Namespace: GatewayControllerNamespace,
		},
	}

	_, err := c.kubeClientSet.CoreV1().ServiceAccounts(GatewayControllerNamespace).Get(ctx, GatewayControllerName, metav1.GetOptions{})
	if err != nil {
		if k8serrors.IsNotFound(err) {
			klog.V(2).Infof("Creating gateway controller service account")
			_, err = c.kubeClientSet.CoreV1().ServiceAccounts(GatewayControllerNamespace).Create(ctx, sa, metav1.CreateOptions{})
			return err
		}
		return err
	}
	return nil
}

// ensureGatewayControllerClusterRole creates the cluster role for gateway controller
func (c *Controller) ensureGatewayControllerClusterRole(ctx context.Context) error {
	cr := &rbacv1.ClusterRole{
		ObjectMeta: metav1.ObjectMeta{
			Name: fmt.Sprintf("%s-role", GatewayControllerName),
		},
		Rules: []rbacv1.PolicyRule{
			{
				APIGroups: []string{"minio.min.io"},
				Resources: []string{"tenants"},
				Verbs:     []string{"get", "list", "watch"},
			},
			{
				APIGroups: []string{""},
				Resources: []string{"services", "endpoints"},
				Verbs:     []string{"get", "list", "watch"},
			},
		},
	}

	_, err := c.kubeClientSet.RbacV1().ClusterRoles().Get(ctx, cr.Name, metav1.GetOptions{})
	if err != nil {
		if k8serrors.IsNotFound(err) {
			klog.V(2).Infof("Creating gateway controller cluster role")
			_, err = c.kubeClientSet.RbacV1().ClusterRoles().Create(ctx, cr, metav1.CreateOptions{})
			return err
		}
		return err
	}
	return nil
}

// ensureGatewayControllerClusterRoleBinding creates the cluster role binding for gateway controller
func (c *Controller) ensureGatewayControllerClusterRoleBinding(ctx context.Context) error {
	crb := &rbacv1.ClusterRoleBinding{
		ObjectMeta: metav1.ObjectMeta{
			Name: fmt.Sprintf("%s-rolebinding", GatewayControllerName),
		},
		RoleRef: rbacv1.RoleRef{
			APIGroup: "rbac.authorization.k8s.io",
			Kind:     "ClusterRole",
			Name:     fmt.Sprintf("%s-role", GatewayControllerName),
		},
		Subjects: []rbacv1.Subject{
			{
				Kind:      "ServiceAccount",
				Name:      GatewayControllerName,
				Namespace: GatewayControllerNamespace,
			},
		},
	}

	_, err := c.kubeClientSet.RbacV1().ClusterRoleBindings().Get(ctx, crb.Name, metav1.GetOptions{})
	if err != nil {
		if k8serrors.IsNotFound(err) {
			klog.V(2).Infof("Creating gateway controller cluster role binding")
			_, err = c.kubeClientSet.RbacV1().ClusterRoleBindings().Create(ctx, crb, metav1.CreateOptions{})
			return err
		}
		return err
	}
	return nil
}

// ensureGatewayControllerDeployment creates the deployment for gateway controller
func (c *Controller) ensureGatewayControllerDeployment(ctx context.Context) error {
	replicas := int32(1)

	// Get gateway image from environment variable, fallback to default
	gatewayImage := os.Getenv(miniov2.TenantGatewayImageEnv)
	if gatewayImage == "" {
		gatewayImage = GatewayControllerImage
		klog.V(2).Infof("Using default gateway controller image: %s", gatewayImage)
	} else {
		klog.V(2).Infof("Using gateway controller image from environment: %s", gatewayImage)
	}

	// Get image pull secret from environment variable
	imagePullSecretName := os.Getenv(miniov2.TenantGatewayImagePullSecretEnv)
	var imagePullSecrets []corev1.LocalObjectReference
	if imagePullSecretName != "" {
		imagePullSecrets = []corev1.LocalObjectReference{
			{Name: imagePullSecretName},
		}
		klog.V(2).Infof("Using image pull secret for gateway controller: %s", imagePullSecretName)
	}

	deployment := &appsv1.Deployment{
		ObjectMeta: metav1.ObjectMeta{
			Name:      GatewayControllerName,
			Namespace: GatewayControllerNamespace,
			Labels: map[string]string{
				"app": GatewayControllerName,
			},
		},
		Spec: appsv1.DeploymentSpec{
			Replicas: &replicas,
			Selector: &metav1.LabelSelector{
				MatchLabels: map[string]string{
					"app": GatewayControllerName,
				},
			},
			Template: corev1.PodTemplateSpec{
				ObjectMeta: metav1.ObjectMeta{
					Labels: map[string]string{
						"app": GatewayControllerName,
					},
				},
				Spec: corev1.PodSpec{
					ServiceAccountName: GatewayControllerName,
					ImagePullSecrets:   imagePullSecrets,
					Containers: []corev1.Container{
						{
							Name:            GatewayControllerName,
							Image:           gatewayImage,
							ImagePullPolicy: corev1.PullAlways,
							Args: []string{
								"--port=18000",
								"--metrics-bind-address=:8383",
								"--health-probe-bind-address=:8081",
								"--cluster-domain=cluster.local",
							},
							Env: []corev1.EnvVar{
								{
									Name: "POD_NAMESPACE",
									ValueFrom: &corev1.EnvVarSource{
										FieldRef: &corev1.ObjectFieldSelector{
											FieldPath: "metadata.namespace",
										},
									},
								},
								{
									Name:  "WATCH_NAMESPACES",
									Value: "",
								},
							},
							Ports: []corev1.ContainerPort{
								{
									Name:          "xds",
									ContainerPort: 18000,
									Protocol:      corev1.ProtocolTCP,
								},
								{
									Name:          "metrics",
									ContainerPort: 8383,
									Protocol:      corev1.ProtocolTCP,
								},
							},
							Resources: corev1.ResourceRequirements{
								Limits: corev1.ResourceList{
									corev1.ResourceCPU:    resource.MustParse("500m"),
									corev1.ResourceMemory: resource.MustParse("512Mi"),
								},
								Requests: corev1.ResourceList{
									corev1.ResourceCPU:    resource.MustParse("100m"),
									corev1.ResourceMemory: resource.MustParse("128Mi"),
								},
							},
							LivenessProbe: &corev1.Probe{
								ProbeHandler: corev1.ProbeHandler{
									HTTPGet: &corev1.HTTPGetAction{
										Path: "/healthz",
										Port: intstr.FromInt(8081),
									},
								},
								InitialDelaySeconds: 15,
								PeriodSeconds:       20,
							},
							ReadinessProbe: &corev1.Probe{
								ProbeHandler: corev1.ProbeHandler{
									HTTPGet: &corev1.HTTPGetAction{
										Path: "/readyz",
										Port: intstr.FromInt(8081),
									},
								},
								InitialDelaySeconds: 5,
								PeriodSeconds:       10,
							},
						},
					},
				},
			},
		},
	}

	_, err := c.kubeClientSet.AppsV1().Deployments(GatewayControllerNamespace).Get(ctx, GatewayControllerName, metav1.GetOptions{})
	if err != nil {
		if k8serrors.IsNotFound(err) {
			klog.V(2).Infof("Creating gateway controller deployment")
			_, err = c.kubeClientSet.AppsV1().Deployments(GatewayControllerNamespace).Create(ctx, deployment, metav1.CreateOptions{})
			return err
		}
		return err
	}
	return nil
}

// ensureGatewayControllerService creates the service for gateway controller
func (c *Controller) ensureGatewayControllerService(ctx context.Context) error {
	svc := &corev1.Service{
		ObjectMeta: metav1.ObjectMeta{
			Name:      GatewayControllerName,
			Namespace: GatewayControllerNamespace,
			Labels: map[string]string{
				"app": GatewayControllerName,
			},
		},
		Spec: corev1.ServiceSpec{
			Type: corev1.ServiceTypeClusterIP,
			Ports: []corev1.ServicePort{
				{
					Name:       "xds",
					Port:       18000,
					TargetPort: intstr.FromInt(18000),
					Protocol:   corev1.ProtocolTCP,
				},
				{
					Name:       "metrics",
					Port:       8383,
					TargetPort: intstr.FromInt(8383),
					Protocol:   corev1.ProtocolTCP,
				},
			},
			Selector: map[string]string{
				"app": GatewayControllerName,
			},
		},
	}

	_, err := c.kubeClientSet.CoreV1().Services(GatewayControllerNamespace).Get(ctx, GatewayControllerName, metav1.GetOptions{})
	if err != nil {
		if k8serrors.IsNotFound(err) {
			klog.V(2).Infof("Creating gateway controller service")
			_, err = c.kubeClientSet.CoreV1().Services(GatewayControllerNamespace).Create(ctx, svc, metav1.CreateOptions{})
			return err
		}
		return err
	}
	return nil
}

// Made with Bob
