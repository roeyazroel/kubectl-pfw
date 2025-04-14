package k8s

import (
	"context"
	"fmt"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// StatefulSet represents a Kubernetes statefulset
type StatefulSet struct {
	Name      string
	Namespace string
}

// GetStatefulSets retrieves all statefulsets in the specified namespace
func (c *Client) GetStatefulSets(ctx context.Context) ([]StatefulSet, error) {
	statefulSetList, err := c.clientset.AppsV1().StatefulSets(c.namespace).List(ctx, metav1.ListOptions{})
	if err != nil {
		return nil, fmt.Errorf("failed to list statefulsets: %w", err)
	}

	statefulSets := make([]StatefulSet, 0, len(statefulSetList.Items))
	for _, ss := range statefulSetList.Items {
		statefulSet := StatefulSet{
			Name:      ss.Name,
			Namespace: ss.Namespace,
		}
		statefulSets = append(statefulSets, statefulSet)
	}

	return statefulSets, nil
}

// GetPodsForStatefulSet returns pods managed by a statefulset
func (c *Client) GetPodsForStatefulSet(ctx context.Context, statefulSetName string) ([]Pod, error) {
	statefulSet, err := c.clientset.AppsV1().StatefulSets(c.namespace).Get(ctx, statefulSetName, metav1.GetOptions{})
	if err != nil {
		return nil, fmt.Errorf("failed to get statefulset %s: %w", statefulSetName, err)
	}

	// Get the selector from the statefulset
	selector := statefulSet.Spec.Selector
	if selector == nil {
		return nil, fmt.Errorf("statefulset %s does not have a selector", statefulSetName)
	}

	// Format the label selector
	labelSelector := metav1.FormatLabelSelector(selector)

	// List pods matching the statefulset's selector
	podList, err := c.clientset.CoreV1().Pods(c.namespace).List(ctx, metav1.ListOptions{
		LabelSelector: labelSelector,
	})
	if err != nil {
		return nil, fmt.Errorf("failed to list pods for statefulset %s: %w", statefulSetName, err)
	}

	if len(podList.Items) == 0 {
		return nil, fmt.Errorf("no pods found for statefulset %s", statefulSetName)
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

// StatefulSetToString returns a string representation of a statefulset
func StatefulSetToString(statefulSet StatefulSet) string {
	return fmt.Sprintf("%s (statefulset)", statefulSet.Name)
}
