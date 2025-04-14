package cli

import (
	"context"
	"fmt"

	"roeyazroel/kubectl-pfw/pkg/k8s"
	"roeyazroel/kubectl-pfw/pkg/ui"
)

// getResourcesForMode retrieves the appropriate resources based on the selected mode.
func getResourcesForMode(usePods, useDeployments, useStatefulSets bool, client *k8s.Client, ctx context.Context) ([]ui.Resource, error) {
	var resources []ui.Resource

	if usePods {
		pods, err := client.GetPods(ctx)
		if err != nil {
			return nil, fmt.Errorf("failed to get pods: %w", err)
		}
		resources = make([]ui.Resource, 0, len(pods))
		for _, pod := range pods {
			if len(pod.Ports) > 0 {
				resources = append(resources, ui.NewResourceFromPod(pod))
			}
		}
		if len(resources) == 0 {
			return nil, fmt.Errorf("no pods with exposed ports found in namespace %s", client.GetNamespace())
		}
	} else if useDeployments {
		deployments, err := client.GetDeployments(ctx)
		if err != nil {
			return nil, fmt.Errorf("failed to get deployments: %w", err)
		}
		resources = make([]ui.Resource, 0, len(deployments))
		for _, dep := range deployments {
			resources = append(resources, ui.NewResourceFromDeployment(dep))
		}
		if len(resources) == 0 {
			return nil, fmt.Errorf("no deployments found in namespace %s", client.GetNamespace())
		}
	} else if useStatefulSets {
		statefulSets, err := client.GetStatefulSets(ctx)
		if err != nil {
			return nil, fmt.Errorf("failed to get statefulsets: %w", err)
		}
		resources = make([]ui.Resource, 0, len(statefulSets))
		for _, ss := range statefulSets {
			resources = append(resources, ui.NewResourceFromStatefulSet(ss))
		}
		if len(resources) == 0 {
			return nil, fmt.Errorf("no statefulsets found in namespace %s", client.GetNamespace())
		}
	} else {
		services, err := client.GetServices(ctx)
		if err != nil {
			return nil, fmt.Errorf("failed to get services: %w", err)
		}
		resources = make([]ui.Resource, 0, len(services))
		for _, svc := range services {
			if len(svc.Ports) > 0 {
				resources = append(resources, ui.NewResourceFromService(svc))
			}
		}
		if len(resources) == 0 {
			return nil, fmt.Errorf("no services with ports found in namespace %s", client.GetNamespace())
		}
	}

	return resources, nil
}

// getPromptForMode returns the appropriate prompt based on the selected mode.
func getPromptForMode(usePods, useDeployments, useStatefulSets bool, isConfig bool, namespace string) string {
	var action string
	if isConfig {
		action = "for configuration"
	} else {
		action = "to port-forward"
	}

	if usePods {
		return fmt.Sprintf("Select pods %s in namespace %s:", action, namespace)
	} else if useDeployments {
		return fmt.Sprintf("Select deployments %s in namespace %s:", action, namespace)
	} else if useStatefulSets {
		return fmt.Sprintf("Select statefulsets %s in namespace %s:", action, namespace)
	}
	return fmt.Sprintf("Select services %s in namespace %s:", action, namespace)
}

// processSelectedResources handles common processing for selected resources.
func processSelectedResources(selectedResources []ui.Resource, client *k8s.Client, ctx context.Context) error {
	for i, resource := range selectedResources {
		if resource.Type == ui.DeploymentResource {
			pods, err := client.GetPodsForDeployment(ctx, resource.Name)
			if err != nil || len(pods) == 0 {
				return fmt.Errorf("no pods found for deployment %s", resource.Name)
			}
			selectedResources[i] = ui.NewResourceFromPod(pods[0])
			selectedResources[i].Type = ui.DeploymentResource
			selectedResources[i].Name = resource.Name
			selectedResources[i].Namespace = resource.Namespace
			selectedResources[i].DisplayName = resource.DisplayName
		} else if resource.Type == ui.StatefulSetResource {
			pods, err := client.GetPodsForStatefulSet(ctx, resource.Name)
			if err != nil || len(pods) == 0 {
				return fmt.Errorf("no pods found for statefulset %s", resource.Name)
			}
			selectedResources[i] = ui.NewResourceFromPod(pods[0])
			selectedResources[i].Type = ui.StatefulSetResource
			selectedResources[i].Name = resource.Name
			selectedResources[i].Namespace = resource.Namespace
			selectedResources[i].DisplayName = resource.DisplayName
		}
	}
	return nil
}

// createPortMappings builds port mappings for resources.
func createPortMappings(selectedResources []ui.Resource, resolvedPorts map[string]map[int]int32, client *k8s.Client) (map[string]map[int]int32, error) {
	portMaps := make(map[string]map[int]int32)

	for _, resource := range selectedResources {
		portMap := make(map[int]int32)
		portMaps[resource.Name] = portMap

		for i, portValue := range resource.Ports {
			suggestedPort := portValue
			if resource.Type == ui.ServiceResource {
				if resolvedPortMap, ok := resolvedPorts[resource.Name]; ok {
					if resolvedValue, ok := resolvedPortMap[i]; ok {
						suggestedPort = resolvedValue
					}
				}
			}
			localPort, err := ui.AskForLocalPort(resource, suggestedPort, i)
			if err != nil {
				return nil, fmt.Errorf("error getting local port: %w", err)
			}
			portMap[i] = localPort
		}
	}

	return portMaps, nil
}
