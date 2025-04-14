package cli

import (
	"context"
	"fmt"

	"roeyazroel/kubectl-pfw/pkg/config"
	"roeyazroel/kubectl-pfw/pkg/k8s"
	"roeyazroel/kubectl-pfw/pkg/portforward"
	"roeyazroel/kubectl-pfw/pkg/ui"

	"k8s.io/cli-runtime/pkg/genericclioptions"
)

// RunInteractive handles interactive selection of resources and port forwarding.
func RunInteractive(usePods, useDeployments, useStatefulSets bool, manager *portforward.Manager, client *k8s.Client, streams genericclioptions.IOStreams, ctx context.Context) error {
	// Get resources based on the selected mode
	resources, err := getResourcesForMode(usePods, useDeployments, useStatefulSets, client, ctx)
	if err != nil {
		return err
	}

	// Get the appropriate prompt
	prompt := getPromptForMode(usePods, useDeployments, useStatefulSets, false, client.GetNamespace())

	// Select resources
	selectedResources, err := ui.SelectResources(resources, prompt)
	if err != nil {
		return err
	}

	// Process the selected resources
	err = processSelectedResources(selectedResources, client, ctx)
	if err != nil {
		return err
	}

	// Resolve target ports
	resolvedPorts, err := config.ResolveTargetPorts(ctx, selectedResources, client)
	if err != nil {
		return fmt.Errorf("failed to resolve target ports: %w", err)
	}

	// Create port mappings
	portMaps, err := createPortMappings(selectedResources, resolvedPorts, client)
	if err != nil {
		return err
	}

	// Start port forwarding for each resource
	for _, resource := range selectedResources {
		portMap := portMaps[resource.Name]
		err := manager.ForwardResource(resource, portMap)
		if err != nil {
			return fmt.Errorf("error starting port forward for %s: %w", resource.Name, err)
		}
	}

	return nil
}
