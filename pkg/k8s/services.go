package k8s

import (
	"context"
	"fmt"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/util/intstr"
)

// Service represents a Kubernetes service with port information
type Service struct {
	Name      string
	Namespace string
	Ports     []ServicePort
}

// ServicePort represents a port in a Kubernetes service
type ServicePort struct {
	Name           string
	Port           int32
	Protocol       string
	TargetPortSpec *intstr.IntOrString // Original spec (numeric or named)
}

// GetServices retrieves all services in the specified namespace
func (c *Client) GetServices(ctx context.Context) ([]Service, error) {
	serviceList, err := c.clientset.CoreV1().Services(c.namespace).List(ctx, metav1.ListOptions{})
	if err != nil {
		return nil, fmt.Errorf("failed to list services: %w", err)
	}

	services := make([]Service, 0, len(serviceList.Items))
	for _, svc := range serviceList.Items {
		service := Service{
			Name:      svc.Name,
			Namespace: svc.Namespace,
			Ports:     make([]ServicePort, 0, len(svc.Spec.Ports)),
		}

		for _, port := range svc.Spec.Ports {
			// Directly use IntVal. It will be 0 if TargetPort is named or not specified.
			// The actual resolution (if needed) is handled by the port-forwarding mechanism
			// when connecting to the selected pod using the service port.

			servicePort := ServicePort{
				Name:           port.Name,
				Port:           port.Port,
				Protocol:       string(port.Protocol),
				TargetPortSpec: &port.TargetPort,
			}
			service.Ports = append(service.Ports, servicePort)
		}

		services = append(services, service)
	}

	return services, nil
}

// GetPodsForService returns pods matching a service's selector
func (c *Client) GetPodsForService(ctx context.Context, serviceName string) ([]Pod, error) {
	service, err := c.clientset.CoreV1().Services(c.namespace).Get(ctx, serviceName, metav1.GetOptions{})
	if err != nil {
		return nil, fmt.Errorf("failed to get service %s: %w", serviceName, err)
	}

	// If service has no selector, it cannot be backed by pods
	if len(service.Spec.Selector) == 0 {
		return nil, fmt.Errorf("service %s does not have a selector", serviceName)
	}

	// Build label selector string from the service's selector
	var labelSelectors []string
	for key, value := range service.Spec.Selector {
		labelSelectors = append(labelSelectors, fmt.Sprintf("%s=%s", key, value))
	}
	labelSelector := metav1.FormatLabelSelector(&metav1.LabelSelector{
		MatchLabels: service.Spec.Selector,
	})

	// List pods matching the service's selector
	podList, err := c.clientset.CoreV1().Pods(c.namespace).List(ctx, metav1.ListOptions{
		LabelSelector: labelSelector,
	})
	if err != nil {
		return nil, fmt.Errorf("failed to list pods for service %s: %w", serviceName, err)
	}

	if len(podList.Items) == 0 {
		return nil, fmt.Errorf("no pods found for service %s", serviceName)
	}

	// Convert to our Pod type
	pods := make([]Pod, 0, len(podList.Items))
	for _, p := range podList.Items {
		pod := Pod{
			Name:      p.Name,
			Namespace: p.Namespace,
			Ports:     []PodPort{},
		}

		// Add ports from init containers
		for _, container := range p.Spec.InitContainers {
			for _, port := range container.Ports {
				podPort := PodPort{
					Name:            port.Name,
					ContainerPort:   port.ContainerPort,
					Protocol:        string(port.Protocol),
					ContainerName:   container.Name,
					IsInitContainer: true,
				}
				pod.Ports = append(pod.Ports, podPort)
			}
		}

		// Add ports from regular containers
		for _, container := range p.Spec.Containers {
			for _, port := range container.Ports {
				podPort := PodPort{
					Name:            port.Name,
					ContainerPort:   port.ContainerPort,
					Protocol:        string(port.Protocol),
					ContainerName:   container.Name,
					IsInitContainer: false,
				}
				pod.Ports = append(pod.Ports, podPort)
			}
		}

		// Only add pods with ports
		if len(pod.Ports) > 0 {
			pods = append(pods, pod)
		}
	}

	return pods, nil
}

// ServiceToString returns a string representation of a service
func ServiceToString(service Service) string {
	if len(service.Ports) == 0 {
		return fmt.Sprintf("%s (no ports)", service.Name)
	}

	if len(service.Ports) == 1 {
		port := service.Ports[0]
		if port.Name != "" {
			return fmt.Sprintf("%s (%s:%d/%s)", service.Name, port.Name, port.Port, port.Protocol)
		}
		return fmt.Sprintf("%s (%d/%s)", service.Name, port.Port, port.Protocol)
	}

	return fmt.Sprintf("%s (%d ports)", service.Name, len(service.Ports))
}
