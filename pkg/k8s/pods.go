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

// PortMetadata contains additional information about a container port
type PortMetadata struct {
	ContainerName   string
	IsInitContainer bool
}

// PodPort represents a container port in a Kubernetes pod
type PodPort struct {
	Name            string
	ContainerPort   int32
	Protocol        string
	ContainerName   string
	IsInitContainer bool
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

// PodToString returns a string representation of a pod
func PodToString(pod Pod) string {
	if len(pod.Ports) == 0 {
		return fmt.Sprintf("%s (no ports)", pod.Name)
	}

	// Count init container ports and regular container ports
	var initContainerPorts, regularContainerPorts int
	for _, port := range pod.Ports {
		if port.IsInitContainer {
			initContainerPorts++
		} else {
			regularContainerPorts++
		}
	}

	// If there's only one port
	if len(pod.Ports) == 1 {
		port := pod.Ports[0]
		containerType := ""
		if port.IsInitContainer {
			containerType = "init:"
		}
		if port.Name != "" {
			return fmt.Sprintf("%s (%s%s:%d/%s)", pod.Name, containerType, port.Name, port.ContainerPort, port.Protocol)
		}
		return fmt.Sprintf("%s (%s%d/%s)", pod.Name, containerType, port.ContainerPort, port.Protocol)
	}

	// If there are multiple ports but they're all of the same type
	if initContainerPorts == 0 {
		return fmt.Sprintf("%s (%d ports)", pod.Name, regularContainerPorts)
	} else if regularContainerPorts == 0 {
		return fmt.Sprintf("%s (%d init ports)", pod.Name, initContainerPorts)
	}

	// If there are both init and regular container ports
	return fmt.Sprintf("%s (%d regular, %d init ports)", pod.Name, regularContainerPorts, initContainerPorts)
}
