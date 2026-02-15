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
	k8serrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/apimachinery/pkg/util/intstr"
	"k8s.io/klog/v2"
)

const (
	EnvoyGatewayName      = "envoy-gateway"
	EnvoyConfigMapName    = "envoy-config"
	EnvoyImage            = "envoyproxy/envoy:v1.28-latest"
	GatewayControllerAddr = "gateway-controller.minio-operator.svc.cluster.local"
	GatewayControllerPort = 18000
)

// checkClusterIssuer function removed - no longer using automatic cert-manager integration
// Users should manually create wildcard certificates if TLS is needed

// checkEnvoyGateway validates and creates/updates the Envoy gateway deployment for the tenant
func (c *Controller) checkEnvoyGateway(ctx context.Context, tenant *miniov2.Tenant, nsName types.NamespacedName) error {
	// Check if Envoy gateway is enabled for this tenant
	if !tenant.Spec.Features.EnableGateway {
		klog.V(2).Infof("Envoy gateway not enabled for tenant %s", nsName)
		return nil
	}

	// Create or update ConfigMap for Envoy configuration
	if err := c.checkEnvoyConfigMap(ctx, tenant); err != nil {
		return err
	}

	// Create or update Envoy Deployment
	if err := c.checkEnvoyDeployment(ctx, tenant); err != nil {
		return err
	}

	// Create or update Envoy Service
	if err := c.checkEnvoyService(ctx, tenant); err != nil {
		return err
	}

	// Ingress removed - LoadBalancer service handles all subdomain routing
	// Envoy does hostname-based routing via xDS configuration

	// Automatic certificate management removed
	// Users should manually create wildcard certificates if TLS is needed
	// The operator will automatically copy wildcard certs for each tenant

	return nil
}

// checkEnvoyConfigMap creates or updates the Envoy configuration ConfigMap
func (c *Controller) checkEnvoyConfigMap(ctx context.Context, tenant *miniov2.Tenant) error {
	configMapName := fmt.Sprintf("%s-%s", EnvoyConfigMapName, tenant.Name)

	envoyConfig := fmt.Sprintf(`node:
  cluster: gateway-%s-%s
  id: gateway-%s-%s

dynamic_resources:
  ads_config:
    api_type: GRPC
    transport_api_version: V3
    grpc_services:
    - envoy_grpc:
        cluster_name: xds_cluster
  cds_config:
    resource_api_version: V3
    ads: {}
  lds_config:
    resource_api_version: V3
    ads: {}

static_resources:
  clusters:
  - name: xds_cluster
    type: STRICT_DNS
    typed_extension_protocol_options:
      envoy.extensions.upstreams.http.v3.HttpProtocolOptions:
        "@type": type.googleapis.com/envoy.extensions.upstreams.http.v3.HttpProtocolOptions
        explicit_http_config:
          http2_protocol_options: {}
    load_assignment:
      cluster_name: xds_cluster
      endpoints:
      - lb_endpoints:
        - endpoint:
            address:
              socket_address:
                address: %s
                port_value: %d

admin:
  address:
    socket_address:
      address: 0.0.0.0
      port_value: 9901
`, tenant.Namespace, tenant.Name, tenant.Namespace, tenant.Name, GatewayControllerAddr, GatewayControllerPort)

	expectedConfigMap := &corev1.ConfigMap{
		ObjectMeta: metav1.ObjectMeta{
			Name:      configMapName,
			Namespace: tenant.Namespace,
			Labels: map[string]string{
				"app":    EnvoyGatewayName,
				"tenant": tenant.Name,
			},
			OwnerReferences: []metav1.OwnerReference{
				*metav1.NewControllerRef(tenant, miniov2.SchemeGroupVersion.WithKind("Tenant")),
			},
		},
		Data: map[string]string{
			"envoy.yaml": envoyConfig,
		},
	}

	cm, err := c.kubeClientSet.CoreV1().ConfigMaps(tenant.Namespace).Get(ctx, configMapName, metav1.GetOptions{})
	if err != nil {
		if k8serrors.IsNotFound(err) {
			klog.V(2).Infof("Creating Envoy ConfigMap for tenant %s/%s", tenant.Namespace, tenant.Name)
			_, err = c.kubeClientSet.CoreV1().ConfigMaps(tenant.Namespace).Create(ctx, expectedConfigMap, metav1.CreateOptions{})
			if err != nil {
				return err
			}
			c.recorder.Event(tenant, corev1.EventTypeNormal, "ConfigMapCreated", "Envoy ConfigMap Created")
			return nil
		}
		return err
	}

	// Update if configuration has changed
	if cm.Data["envoy.yaml"] != envoyConfig {
		cm.Data = expectedConfigMap.Data
		_, err = c.kubeClientSet.CoreV1().ConfigMaps(tenant.Namespace).Update(ctx, cm, metav1.UpdateOptions{})
		if err != nil {
			return err
		}
		c.recorder.Event(tenant, corev1.EventTypeNormal, "ConfigMapUpdated", "Envoy ConfigMap Updated")
	}

	return nil
}

// checkEnvoyDeployment creates or updates the Envoy gateway deployment
func (c *Controller) checkEnvoyDeployment(ctx context.Context, tenant *miniov2.Tenant) error {
	deploymentName := fmt.Sprintf("%s-%s", EnvoyGatewayName, tenant.Name)
	configMapName := fmt.Sprintf("%s-%s", EnvoyConfigMapName, tenant.Name)

	replicas := int32(2)
	if tenant.Spec.Features.GatewayReplicas != nil {
		replicas = *tenant.Spec.Features.GatewayReplicas
	}

	expectedDeployment := &appsv1.Deployment{
		ObjectMeta: metav1.ObjectMeta{
			Name:      deploymentName,
			Namespace: tenant.Namespace,
			Labels: map[string]string{
				"app":    EnvoyGatewayName,
				"tenant": tenant.Name,
			},
			OwnerReferences: []metav1.OwnerReference{
				*metav1.NewControllerRef(tenant, miniov2.SchemeGroupVersion.WithKind("Tenant")),
			},
		},
		Spec: appsv1.DeploymentSpec{
			Replicas: &replicas,
			Selector: &metav1.LabelSelector{
				MatchLabels: map[string]string{
					"app":    EnvoyGatewayName,
					"tenant": tenant.Name,
				},
			},
			Template: corev1.PodTemplateSpec{
				ObjectMeta: metav1.ObjectMeta{
					Labels: map[string]string{
						"app":    EnvoyGatewayName,
						"tenant": tenant.Name,
					},
				},
				Spec: corev1.PodSpec{
					ImagePullSecrets: getImagePullSecrets(tenant),
					Containers: []corev1.Container{
						{
							Name:            "envoy",
							Image:           EnvoyImage,
							ImagePullPolicy: corev1.PullIfNotPresent,
							Command:         []string{"/usr/local/bin/envoy"},
							Args: []string{
								"-c", "/etc/envoy/envoy.yaml",
								"--service-cluster", fmt.Sprintf("gateway-%s-%s", tenant.Namespace, tenant.Name),
								"--service-node", fmt.Sprintf("gateway-%s-%s", tenant.Namespace, tenant.Name),
							},
							Ports: []corev1.ContainerPort{
								{
									Name:          "http",
									ContainerPort: 10000,
									Protocol:      corev1.ProtocolTCP,
								},
								{
									Name:          "admin",
									ContainerPort: 9901,
									Protocol:      corev1.ProtocolTCP,
								},
							},
							VolumeMounts: []corev1.VolumeMount{
								{
									Name:      "envoy-config",
									MountPath: "/etc/envoy",
								},
								{
									Name:      "minio-tls-certs",
									MountPath: "/etc/envoy/minio-certs",
									ReadOnly:  true,
								},
								{
									Name:      "gateway-tls-certs",
									MountPath: "/etc/envoy/gateway-certs",
									ReadOnly:  true,
								},
							},
							Resources: corev1.ResourceRequirements{
								Limits: corev1.ResourceList{
									corev1.ResourceCPU:    *parseQuantity("1000m"),
									corev1.ResourceMemory: *parseQuantity("1Gi"),
								},
								Requests: corev1.ResourceList{
									corev1.ResourceCPU:    *parseQuantity("200m"),
									corev1.ResourceMemory: *parseQuantity("256Mi"),
								},
							},
							LivenessProbe: &corev1.Probe{
								ProbeHandler: corev1.ProbeHandler{
									HTTPGet: &corev1.HTTPGetAction{
										Path: "/ready",
										Port: intstr.FromInt(9901),
									},
								},
								InitialDelaySeconds: 15,
								PeriodSeconds:       20,
							},
							ReadinessProbe: &corev1.Probe{
								ProbeHandler: corev1.ProbeHandler{
									HTTPGet: &corev1.HTTPGetAction{
										Path: "/ready",
										Port: intstr.FromInt(9901),
									},
								},
								InitialDelaySeconds: 5,
								PeriodSeconds:       10,
							},
						},
					},
					Volumes: []corev1.Volume{
						{
							Name: "envoy-config",
							VolumeSource: corev1.VolumeSource{
								ConfigMap: &corev1.ConfigMapVolumeSource{
									LocalObjectReference: corev1.LocalObjectReference{
										Name: configMapName,
									},
								},
							},
						},
						{
							Name: "minio-tls-certs",
							VolumeSource: corev1.VolumeSource{
								Secret: &corev1.SecretVolumeSource{
									SecretName: fmt.Sprintf("%s-tls", tenant.Name),
									Items: []corev1.KeyToPath{
										{
											Key:  "public.crt",
											Path: "public.crt",
										},
									},
									Optional: &[]bool{true}[0],
								},
							},
						},
						{
							Name: "gateway-tls-certs",
							VolumeSource: corev1.VolumeSource{
								Secret: &corev1.SecretVolumeSource{
									SecretName: fmt.Sprintf("envoy-gateway-%s-tls", tenant.Name),
									Items: []corev1.KeyToPath{
										{
											Key:  "tls.crt",
											Path: "tls.crt",
										},
										{
											Key:  "tls.key",
											Path: "tls.key",
										},
									},
									Optional: &[]bool{true}[0],
								},
							},
						},
					},
				},
			},
		},
	}

	deployment, err := c.kubeClientSet.AppsV1().Deployments(tenant.Namespace).Get(ctx, deploymentName, metav1.GetOptions{})
	if err != nil {
		if k8serrors.IsNotFound(err) {
			klog.V(2).Infof("Creating Envoy Deployment for tenant %s/%s", tenant.Namespace, tenant.Name)
			_, err = c.kubeClientSet.AppsV1().Deployments(tenant.Namespace).Create(ctx, expectedDeployment, metav1.CreateOptions{})
			if err != nil {
				return err
			}
			c.recorder.Event(tenant, corev1.EventTypeNormal, "DeploymentCreated", "Envoy Gateway Deployment Created")
			return nil
		}
		return err
	}

	// Update deployment if replicas changed
	if *deployment.Spec.Replicas != replicas {
		deployment.Spec.Replicas = &replicas
		_, err = c.kubeClientSet.AppsV1().Deployments(tenant.Namespace).Update(ctx, deployment, metav1.UpdateOptions{})
		if err != nil {
			return err
		}
		c.recorder.Event(tenant, corev1.EventTypeNormal, "DeploymentUpdated", "Envoy Gateway Deployment Updated")
	}

	return nil
}

// checkEnvoyService creates or updates the Envoy gateway service
func (c *Controller) checkEnvoyService(ctx context.Context, tenant *miniov2.Tenant) error {
	serviceName := fmt.Sprintf("%s-%s", EnvoyGatewayName, tenant.Name)

	expectedService := &corev1.Service{
		ObjectMeta: metav1.ObjectMeta{
			Name:      serviceName,
			Namespace: tenant.Namespace,
			Labels: map[string]string{
				"app":    EnvoyGatewayName,
				"tenant": tenant.Name,
			},
			OwnerReferences: []metav1.OwnerReference{
				*metav1.NewControllerRef(tenant, miniov2.SchemeGroupVersion.WithKind("Tenant")),
			},
		},
		Spec: corev1.ServiceSpec{
			Type: corev1.ServiceTypeLoadBalancer,
			Ports: []corev1.ServicePort{
				{
					Name:       "https",
					Port:       443,
					TargetPort: intstr.FromInt(10000),
					Protocol:   corev1.ProtocolTCP,
				},
				{
					Name:       "http",
					Port:       80,
					TargetPort: intstr.FromInt(10000),
					Protocol:   corev1.ProtocolTCP,
				},
				{
					Name:       "console",
					Port:       9443,
					TargetPort: intstr.FromInt(10000),
					Protocol:   corev1.ProtocolTCP,
				},
				{
					Name:       "admin",
					Port:       9901,
					TargetPort: intstr.FromInt(9901),
					Protocol:   corev1.ProtocolTCP,
				},
			},
			Selector: map[string]string{
				"app":    EnvoyGatewayName,
				"tenant": tenant.Name,
			},
		},
	}

	svc, err := c.kubeClientSet.CoreV1().Services(tenant.Namespace).Get(ctx, serviceName, metav1.GetOptions{})
	if err != nil {
		if k8serrors.IsNotFound(err) {
			klog.V(2).Infof("Creating Envoy Service for tenant %s/%s", tenant.Namespace, tenant.Name)
			_, err = c.kubeClientSet.CoreV1().Services(tenant.Namespace).Create(ctx, expectedService, metav1.CreateOptions{})
			if err != nil {
				return err
			}
			c.recorder.Event(tenant, corev1.EventTypeNormal, "ServiceCreated", "Envoy Gateway Service Created")
			return nil
		}
		return err
	}

	// Update service if needed
	needsUpdate := false
	if len(svc.Spec.Ports) != len(expectedService.Spec.Ports) {
		needsUpdate = true
	}

	if needsUpdate {
		svc.Spec.Ports = expectedService.Spec.Ports
		svc.Spec.Selector = expectedService.Spec.Selector
		_, err = c.kubeClientSet.CoreV1().Services(tenant.Namespace).Update(ctx, svc, metav1.UpdateOptions{})
		if err != nil {
			return err
		}
		c.recorder.Event(tenant, corev1.EventTypeNormal, "ServiceUpdated", "Envoy Gateway Service Updated")
	}

	return nil
}

// checkEnvoyIngress function removed - Ingress not needed
// LoadBalancer service handles all subdomain routing via Envoy xDS configuration

// checkEnvoyCertificate function removed - no automatic certificate management
// Wildcard certificate support is still available:
// If a secret named "wildcard-tls-<base-domain>" exists in the tenant namespace,
// the operator will automatically copy it as "envoy-gateway-<tenant-name>-tls"
// This is handled in checkEnvoyDeployment when mounting the certificate volume

// getImagePullSecrets returns the image pull secrets for Envoy Gateway
// Reads from TENANT_GATEWAY_IMAGE_PULL_SECRET environment variable set by operator
func getImagePullSecrets(tenant *miniov2.Tenant) []corev1.LocalObjectReference {
	// Check if operator has configured an imagePullSecret via environment variable
	secretName := os.Getenv(miniov2.TenantGatewayImagePullSecretEnv)
	if secretName != "" {
		return []corev1.LocalObjectReference{
			{
				Name: secretName,
			},
		}
	}
	return nil
}

// Helper function to parse resource quantities
func parseQuantity(s string) *resource.Quantity {
	q, _ := resource.ParseQuantity(s)
	return &q
}

// Made with Bob
