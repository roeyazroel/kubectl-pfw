package config

import (
	"fmt"
	"io"
	"os"
	"roeyazroel/kubectl-pfw/pkg/ui"

	"gopkg.in/yaml.v3"
)

// PortForwardEntry represents a single port forwarding configuration entry
type PortForwardEntry struct {
	// ResourceType can be "service", "pod", "deployment", or "statefulset"
	ResourceType string `yaml:"resourceType"`
	// Name of the resource to forward to
	Name string `yaml:"name"`
	// Optional namespace, uses current context namespace if empty
	Namespace string `yaml:"namespace,omitempty"`
	// Port mappings
	Ports []PortMapping `yaml:"ports"`
}

// PortMapping defines a local-to-remote port mapping
type PortMapping struct {
	// Local port to use. If 0, auto-assign based on remote port
	LocalPort int32 `yaml:"localPort"`
	// Remote port to forward to
	RemotePort int32 `yaml:"remotePort"`
}

// ForwardingConfig defines the structure of a configuration file for port forwarding
type ForwardingConfig struct {
	// Context is the Kubernetes context to use (optional, uses current if empty)
	Context string `yaml:"context,omitempty"`
	// DefaultNamespace is the namespace to use for resources if not specified (optional)
	DefaultNamespace string `yaml:"defaultNamespace,omitempty"`
	// Resources is a list of resources to forward
	Resources []PortForwardEntry `yaml:"resources"`
}

// LoadConfig loads a forwarding configuration from a YAML file
func LoadConfig(filePath string) (*ForwardingConfig, error) {
	file, err := os.Open(filePath)
	if err != nil {
		return nil, fmt.Errorf("failed to open config file: %w", err)
	}
	defer file.Close()

	content, err := io.ReadAll(file)
	if err != nil {
		return nil, fmt.Errorf("failed to read config file: %w", err)
	}

	config := &ForwardingConfig{}
	err = yaml.Unmarshal(content, config)
	if err != nil {
		return nil, fmt.Errorf("failed to parse config file: %w", err)
	}

	// Validate the configuration
	if err := validateConfig(config); err != nil {
		return nil, err
	}

	return config, nil
}

// validateConfig validates a forwarding configuration
func validateConfig(config *ForwardingConfig) error {
	if len(config.Resources) == 0 {
		return fmt.Errorf("no resources specified in config")
	}

	for i, res := range config.Resources {
		if res.ResourceType == "" {
			return fmt.Errorf("resource %d: resourceType is required", i+1)
		}

		// Check if resource type is valid
		switch res.ResourceType {
		case "service", "pod", "deployment", "statefulset":
			// Valid resource type
		default:
			return fmt.Errorf("resource %d: invalid resourceType '%s', must be one of: service, pod, deployment, statefulset", i+1, res.ResourceType)
		}

		if res.Name == "" {
			return fmt.Errorf("resource %d: name is required", i+1)
		}

		if len(res.Ports) == 0 {
			return fmt.Errorf("resource %d: no ports specified", i+1)
		}

		for j, port := range res.Ports {
			if port.RemotePort <= 0 {
				return fmt.Errorf("resource %d, port %d: remotePort must be greater than 0", i+1, j+1)
			}

			if port.LocalPort < 0 {
				return fmt.Errorf("resource %d, port %d: localPort must be at least 0", i+1, j+1)
			}
		}
	}

	return nil
}

// ConvertEntryToResource converts a PortForwardEntry to a ui.Resource
func ConvertEntryToResource(entry PortForwardEntry, defaultNamespace string) (ui.Resource, error) {
	// Determine the namespace to use
	namespace := defaultNamespace
	if entry.Namespace != "" {
		namespace = entry.Namespace
	}

	// Determine the resource type
	var resourceType ui.ResourceType
	switch entry.ResourceType {
	case "service":
		resourceType = ui.ServiceResource
	case "pod":
		resourceType = ui.PodResource
	case "deployment":
		resourceType = ui.DeploymentResource
	case "statefulset":
		resourceType = ui.StatefulSetResource
	default:
		return ui.Resource{}, fmt.Errorf("invalid resource type: %s", entry.ResourceType)
	}

	// Extract ports
	ports := make([]int32, len(entry.Ports))
	for i, p := range entry.Ports {
		ports[i] = p.RemotePort
	}

	// Create a resource object
	return ui.Resource{
		Name:      entry.Name,
		Namespace: namespace,
		Type:      resourceType,
		Ports:     ports,
		// Note: We're not setting PortNames and TargetPortSpecs here
		// Those are typically filled by the k8s client when getting actual resources
		DisplayName: fmt.Sprintf("%s/%s", entry.ResourceType, entry.Name),
	}, nil
}

// CreatePortMapping creates a port mapping map from a PortForwardEntry
func CreatePortMapping(entry PortForwardEntry) map[int]int32 {
	mapping := make(map[int]int32)
	for i, p := range entry.Ports {
		if p.LocalPort > 0 { // Only add explicit mappings
			mapping[i] = p.LocalPort
		}
	}
	return mapping
}
