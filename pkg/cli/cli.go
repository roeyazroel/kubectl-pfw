package cli

import (
	"context"
	"fmt"
	"os"
	"os/signal"
	"syscall"
	"time"

	"roeyazroel/kubectl-pfw/pkg/k8s"
	"roeyazroel/kubectl-pfw/pkg/portforward"

	"github.com/spf13/cobra"
	"k8s.io/cli-runtime/pkg/genericclioptions"
)

// Run contains the core logic: fetching resources, prompting user selection,
// calculating port mappings (including remapping and conflict resolution),
// and starting the port forwarding manager.
func Run(flags *genericclioptions.ConfigFlags, streams genericclioptions.IOStreams, cmd *cobra.Command) error {
	ctx, cancel := context.WithCancel(cmd.Context())
	defer cancel()

	// Create Kubernetes client
	client, err := k8s.NewClient(flags)
	if err != nil {
		return fmt.Errorf("failed to create Kubernetes client: %w", err)
	}

	usePods, err := cmd.Flags().GetBool("pods")
	if err != nil {
		return fmt.Errorf("failed to get --pods flag: %w", err)
	}
	useDeployments, err := cmd.Flags().GetBool("deployments")
	if err != nil {
		return fmt.Errorf("failed to get --deployments flag: %w", err)
	}
	useStatefulSets, err := cmd.Flags().GetBool("statefulsets")
	if err != nil {
		return fmt.Errorf("failed to get --statefulsets flag: %w", err)
	}

	configFile, err := cmd.Flags().GetString("file")
	if err != nil {
		return fmt.Errorf("failed to get --file flag: %w", err)
	}

	generateConfig, err := cmd.Flags().GetBool("generate-config")
	if err != nil {
		return fmt.Errorf("failed to get --generate-config flag: %w", err)
	}

	outputFile, err := cmd.Flags().GetString("output")
	if err != nil {
		return fmt.Errorf("failed to get --output flag: %w", err)
	}

	// If both configFile and generateConfig are specified, show an error
	if configFile != "" && generateConfig {
		return fmt.Errorf("cannot use both --file and --generate-config flags together")
	}

	// Ensure only one of --pods, --deployments, or --statefulsets is set
	selectedModes := 0
	if usePods {
		selectedModes++
	}
	if useDeployments {
		selectedModes++
	}
	if useStatefulSets {
		selectedModes++
	}
	if selectedModes > 1 {
		return fmt.Errorf("only one of --pods, --deployments, or --statefulsets can be used at a time")
	}

	// Start port forwarding manager
	manager := portforward.NewManager(client.GetConfig(), client.GetClientset(), client, streams, ctx)

	// Set up signal handler with access to the cancel function
	signals := make(chan os.Signal, 1)
	signal.Notify(signals, syscall.SIGINT, syscall.SIGTERM)
	go func() {
		<-signals
		fmt.Fprintln(streams.Out, "\nShutting down port forwarding...")
		manager.Stop()
		cancel() // Cancel the context

		// Wait briefly for clean shutdown
		time.Sleep(500 * time.Millisecond)
		os.Exit(0)
	}()

	// If a config file is specified, use it
	if configFile != "" {
		err := RunWithConfigFile(configFile, manager, client, ctx)
		if err != nil {
			return err
		}
	} else {
		// If generate config is specified, run interactive selection and generate config
		if generateConfig {
			err := GenerateConfigFile(usePods, useDeployments, useStatefulSets, outputFile, client, streams, ctx)
			if err != nil {
				return err
			}
			return nil // Exit after generating config
		}

		// Otherwise, use interactive selection for port forwarding
		err := RunInteractive(usePods, useDeployments, useStatefulSets, manager, client, streams, ctx)
		if err != nil {
			return err
		}
	}

	fmt.Fprintln(streams.Out, "Port forwarding started. Press Ctrl+C to stop.")
	manager.WaitForCompletion()

	return nil
}
