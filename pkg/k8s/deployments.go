package k8s

import (
	"context"
	"fmt"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// Deployment represents a Kubernetes deployment
type Deployment struct {
	Name      string
	Namespace string
}

// GetDeployments retrieves all deployments in the specified namespace
func (c *Client) GetDeployments(ctx context.Context) ([]Deployment, error) {
	deploymentList, err := c.clientset.AppsV1().Deployments(c.namespace).List(ctx, metav1.ListOptions{})
	if err != nil {
		return nil, fmt.Errorf("failed to list deployments: %w", err)
	}

	deployments := make([]Deployment, 0, len(deploymentList.Items))
	for _, d := range deploymentList.Items {
		deployment := Deployment{
			Name:      d.Name,
			Namespace: d.Namespace,
		}
		deployments = append(deployments, deployment)
	}

	return deployments, nil
}

// GetPodsForDeployment returns pods managed by a deployment
func (c *Client) GetPodsForDeployment(ctx context.Context, deploymentName string) ([]Pod, error) {
	deployment, err := c.clientset.AppsV1().Deployments(c.namespace).Get(ctx, deploymentName, metav1.GetOptions{})
	if err != nil {
		return nil, fmt.Errorf("failed to get deployment %s: %w", deploymentName, err)
	}

	// Get the selector from the deployment
	selector := deployment.Spec.Selector
	if selector == nil {
		return nil, fmt.Errorf("deployment %s does not have a selector", deploymentName)
	}

	// Format the label selector
	labelSelector := metav1.FormatLabelSelector(selector)

	// List pods matching the deployment's selector
	podList, err := c.clientset.CoreV1().Pods(c.namespace).List(ctx, metav1.ListOptions{
		LabelSelector: labelSelector,
	})
	if err != nil {
		return nil, fmt.Errorf("failed to list pods for deployment %s: %w", deploymentName, err)
	}

	if len(podList.Items) == 0 {
		return nil, fmt.Errorf("no pods found for deployment %s", deploymentName)
	}

	// Convert to our Pod type
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

// DeploymentToString returns a string representation of a deployment
func DeploymentToString(deployment Deployment) string {
	return fmt.Sprintf("%s (deployment)", deployment.Name)
}
