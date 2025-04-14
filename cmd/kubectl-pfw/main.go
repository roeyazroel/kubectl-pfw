package main

import (
	"fmt"
	"os"

	"roeyazroel/kubectl-pfw/pkg/cli"

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

	# Generate a configuration file from interactive selection
	%[1]s pfw --generate-config --output my-config.yaml

	# Generate a configuration file for pods
	%[1]s pfw --pods --generate-config
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
			return cli.Run(flags, streams, cmd)
		},
	}

	flags.AddFlags(root.Flags())

	usePods := false
	useDeployments := false
	useStatefulSets := false
	configFile := ""
	generateConfig := false
	outputFile := "kubectl-pfw-config.yaml"

	root.Flags().BoolVar(&usePods, "pods", false, "Select pods instead of services")
	root.Flags().BoolVar(&useDeployments, "deployments", false, "Select deployments instead of services")
	root.Flags().BoolVar(&useStatefulSets, "statefulsets", false, "Select statefulsets instead of services")
	root.Flags().StringVarP(&configFile, "file", "f", "", "Configuration file for port forwarding")
	root.Flags().BoolP("version", "v", false, "Show version information")
	root.Flags().BoolVarP(&generateConfig, "generate-config", "g", false, "Generate configuration file from interactive selection")
	root.Flags().StringVarP(&outputFile, "output", "o", outputFile, "Output file for generated configuration")

	if err := root.Execute(); err != nil {
		os.Exit(1)
	}
}
