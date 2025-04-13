package main

import (
	"context"
	"fmt"
	"os"
	"os/signal"
	"syscall"
	"time"

	"roeyazroel/kubectl-pfw/pkg/config"
	"roeyazroel/kubectl-pfw/pkg/k8s"
	"roeyazroel/kubectl-pfw/pkg/portforward"
	"roeyazroel/kubectl-pfw/pkg/ui"

	"github.com/spf13/cobra"
	"k8s.io/cli-runtime/pkg/genericclioptions"
)

var (
	// version is set during build using -ldflags
	version = "dev"

	pfwExample = `
	# Port forward multiple services in the current namespace
	%[1]s pfw

	# Port forward multiple services in a specific namespace
	%[1]s pfw -n mynamespace

	# Port forward multiple pods in the current namespace
	%[1]s pfw --pods

	# Port forward using a configuration file
	%[1]s pfw -f config.yaml
`
)

// main sets up the command structure using Cobra and executes the root command.
// It defines flags for Kubernetes configuration and the --pods option.
func main() {
	flags := genericclioptions.NewConfigFlags(true)
	streams := genericclioptions.IOStreams{
		In:     os.Stdin,
		Out:    os.Stdout,
		ErrOut: os.Stderr,
	}

	root := &cobra.Command{
		Use:          "kubectl-pfw",
		Short:        "Port forward multiple services or pods",
		Example:      fmt.Sprintf(pfwExample, "kubectl"),
		SilenceUsage: true,
		RunE: func(cmd *cobra.Command, args []string) error {
			// Check for version flag
			showVersion, _ := cmd.Flags().GetBool("version")
			if showVersion {
				fmt.Printf("kubectl-pfw version %s\n", version)
				return nil
			}
			return run(flags, streams, cmd)
		},
	}

	flags.AddFlags(root.Flags())

	usePods := false
	configFile := ""
	root.Flags().BoolVar(&usePods, "pods", false, "Select pods instead of services")
	root.Flags().StringVarP(&configFile, "file", "f", "", "Configuration file for port forwarding")
	root.Flags().BoolP("version", "v", false, "Show version information")

	if err := root.Execute(); err != nil {
		os.Exit(1)
	}
}

// run contains the core logic: fetching resources, prompting user selection,
// calculating port mappings (including remapping and conflict resolution),
// and starting the port forwarding manager.
func run(flags *genericclioptions.ConfigFlags, streams genericclioptions.IOStreams, cmd *cobra.Command) error {
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

	configFile, err := cmd.Flags().GetString("file")
	if err != nil {
		return fmt.Errorf("failed to get --file flag: %w", err)
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
		err := runWithConfigFile(configFile, manager, client, ctx)
		if err != nil {
			return err
		}
	} else {
		// Otherwise, use interactive selection
		err := runInteractive(usePods, manager, client, streams, ctx)
		if err != nil {
			return err
		}
	}

	fmt.Fprintln(streams.Out, "Port forwarding started. Press Ctrl+C to stop.")
	manager.WaitForCompletion()

	return nil
}

// runWithConfigFile handles port forwarding based on a configuration file
func runWithConfigFile(filePath string, manager *portforward.Manager, client *k8s.Client, ctx context.Context) error {
	cfg, err := config.LoadConfig(filePath)
	if err != nil {
		return fmt.Errorf("failed to load config file: %w", err)
	}

	// Set namespace from config if specified
	if cfg.DefaultNamespace != "" {
		client.SetNamespace(cfg.DefaultNamespace)
	}

	// Process each resource in the config
	for i, entry := range cfg.Resources {
		resource, err := config.ConvertEntryToResource(entry, client.GetNamespace())
		if err != nil {
			return fmt.Errorf("error processing resource %d: %w", i+1, err)
		}

		// Create port mapping from entry
		portMapping := config.CreatePortMapping(entry)

		// Start port forwarding
		err = manager.ForwardResource(resource, portMapping)
		if err != nil {
			return fmt.Errorf("error forwarding resource %s: %w", resource.Name, err)
		}
	}

	return nil
}

// runInteractive handles interactive selection of resources and port forwarding
func runInteractive(usePods bool, manager *portforward.Manager, client *k8s.Client, streams genericclioptions.IOStreams, ctx context.Context) error {
	var resources []ui.Resource
	if usePods {
		// Get pods
		pods, err := client.GetPods(ctx)
		if err != nil {
			return fmt.Errorf("failed to get pods: %w", err)
		}

		// Convert pods to resources
		resources = make([]ui.Resource, 0, len(pods))
		for _, pod := range pods {
			// Only add pods with ports
			if len(pod.Ports) > 0 {
				resources = append(resources, ui.NewResourceFromPod(pod))
			}
		}

		if len(resources) == 0 {
			return fmt.Errorf("no pods with exposed ports found in namespace %s", client.GetNamespace())
		}
	} else {
		// Get services
		services, err := client.GetServices(ctx)
		if err != nil {
			return fmt.Errorf("failed to get services: %w", err)
		}

		// Convert services to resources
		resources = make([]ui.Resource, 0, len(services))
		for _, svc := range services {
			// Only add services with ports
			if len(svc.Ports) > 0 {
				resources = append(resources, ui.NewResourceFromService(svc))
			}
		}

		if len(resources) == 0 {
			return fmt.Errorf("no services with ports found in namespace %s", client.GetNamespace())
		}
	}

	// Display multi-select UI
	var prompt string
	if usePods {
		prompt = fmt.Sprintf("Select pods to port-forward in namespace %s:", client.GetNamespace())
	} else {
		prompt = fmt.Sprintf("Select services to port-forward in namespace %s:", client.GetNamespace())
	}

	selectedResources, err := ui.SelectResources(resources, prompt)
	if err != nil {
		return err
	}

	// Ask for local port for each resource port
	portMaps := make(map[string]map[int]int32)

	for _, resource := range selectedResources {
		portMap := make(map[int]int32)
		portMaps[resource.Name] = portMap

		for i, portValue := range resource.Ports {
			// Ask the user for the local port
			var portName string
			if i < len(resource.PortNames) && resource.PortNames[i] != "" {
				portName = resource.PortNames[i]
			}

			// Display port info
			var promptMsg string
			if portName != "" {
				promptMsg = fmt.Sprintf("Local port for %s/%s (remote port %d)", resource.Name, portName, portValue)
			} else {
				promptMsg = fmt.Sprintf("Local port for %s (remote port %d)", resource.Name, portValue)
			}

			// For ephemeral port allocation, suggest 0
			fmt.Fprintf(streams.Out, "%s [%d, or 0 for auto]: ", promptMsg, portValue)
			var localPort int32
			_, err := fmt.Fscanln(streams.In, &localPort)
			if err != nil {
				// Default to the same as remote port if input fails
				localPort = portValue
			}

			portMap[i] = localPort
		}
	}

	// Start port forwarding for each selected resource
	for _, resource := range selectedResources {
		portMap := portMaps[resource.Name]
		err := manager.ForwardResource(resource, portMap)
		if err != nil {
			return fmt.Errorf("error starting port forward for %s: %w", resource.Name, err)
		}
	}

	return nil
}
