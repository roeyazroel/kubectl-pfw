package k8s

import (
	"context"
	"fmt"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// Pod represents a Kubernetes pod with container port information
type Pod struct {
	Name      string
	Namespace string
	Ports     []PodPort
}

// PodPort represents a container port in a Kubernetes pod
type PodPort struct {
	Name          string
	ContainerPort int32
	Protocol      string
	ContainerName string
}

// GetPods retrieves all pods in the specified namespace
func (c *Client) GetPods(ctx context.Context) ([]Pod, error) {
	podList, err := c.clientset.CoreV1().Pods(c.namespace).List(ctx, metav1.ListOptions{})
	if err != nil {
		return nil, fmt.Errorf("failed to list pods: %w", err)
	}

	pods := make([]Pod, 0, len(podList.Items))
	for _, p := range podList.Items {
		pod := Pod{
			Name:      p.Name,
			Namespace: p.Namespace,
			Ports:     []PodPort{},
		}

		for _, container := range p.Spec.Containers {
			for _, port := range container.Ports {
				podPort := PodPort{
					Name:          port.Name,
					ContainerPort: port.ContainerPort,
					Protocol:      string(port.Protocol),
					ContainerName: container.Name,
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

// PodToString returns a string representation of a pod
func PodToString(pod Pod) string {
	if len(pod.Ports) == 0 {
		return fmt.Sprintf("%s (no ports)", pod.Name)
	}

	if len(pod.Ports) == 1 {
		port := pod.Ports[0]
		if port.Name != "" {
			return fmt.Sprintf("%s (%s:%d/%s)", pod.Name, port.Name, port.ContainerPort, port.Protocol)
		}
		return fmt.Sprintf("%s (%d/%s)", pod.Name, port.ContainerPort, port.Protocol)
	}

	return fmt.Sprintf("%s (%d ports)", pod.Name, len(pod.Ports))
}
