package cli

import (
	"context"
	"fmt"
	"os"
	"path/filepath"

	"roeyazroel/kubectl-pfw/pkg/config"
	"roeyazroel/kubectl-pfw/pkg/k8s"
	"roeyazroel/kubectl-pfw/pkg/portforward"
	"roeyazroel/kubectl-pfw/pkg/ui"

	"k8s.io/cli-runtime/pkg/genericclioptions"
)

// GenerateConfigFile handles interactive selection and generates a configuration file.
func GenerateConfigFile(usePods, useDeployments, useStatefulSets bool, outputFile string, client *k8s.Client, streams genericclioptions.IOStreams, ctx context.Context) error {
	// Get resources based on the selected mode
	resources, err := getResourcesForMode(usePods, useDeployments, useStatefulSets, client, ctx)
	if err != nil {
		return err
	}

	// Get the appropriate prompt
	prompt := getPromptForMode(usePods, useDeployments, useStatefulSets, true, client.GetNamespace())

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

	// Generate the configuration
	cfg := config.GenerateConfig(selectedResources, portMaps, resolvedPorts, client.GetNamespace())

	// Create output directory if needed
	outputDir := filepath.Dir(outputFile)
	if outputDir != "" && outputDir != "." {
		err := os.MkdirAll(outputDir, 0755)
		if err != nil {
			return fmt.Errorf("failed to create output directory: %w", err)
		}
	}

	// Write the configuration to file
	err = config.WriteConfig(cfg, outputFile)
	if err != nil {
		return fmt.Errorf("failed to write config file: %w", err)
	}

	fmt.Fprintf(streams.Out, "Configuration file generated at: %s\n", outputFile)
	fmt.Fprintf(streams.Out, "You can use it with: kubectl pfw -f %s\n", outputFile)

	return nil
}

// RunWithConfigFile handles port forwarding based on a configuration file.
func RunWithConfigFile(filePath string, manager *portforward.Manager, client *k8s.Client, ctx context.Context) error {
	cfg, err := config.LoadConfig(filePath)
	if err != nil {
		return fmt.Errorf("failed to load config file: %w", err)
	}

	if cfg.DefaultNamespace != "" {
		client.SetNamespace(cfg.DefaultNamespace)
	}

	for i, entry := range cfg.Resources {
		resource, err := config.ConvertEntryToResource(entry, client.GetNamespace())
		if err != nil {
			return fmt.Errorf("error processing resource %d: %w", i+1, err)
		}
		portMapping := config.CreatePortMapping(entry)
		err = manager.ForwardResource(resource, portMapping)
		if err != nil {
			return fmt.Errorf("error forwarding resource %s: %w", resource.Name, err)
		}
	}

	return nil
}
