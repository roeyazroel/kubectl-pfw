package config

import (
	"context"
	"fmt"
	"io"
	"os"
	"roeyazroel/kubectl-pfw/pkg/k8s"
	"roeyazroel/kubectl-pfw/pkg/ui"

	"gopkg.in/yaml.v3"
	"k8s.io/apimachinery/pkg/util/intstr"
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
	// Initialize targetPortSpecs for all resource types - will be used for services
	targetPortSpecs := make([]*intstr.IntOrString, len(entry.Ports))

	for i, p := range entry.Ports {
		ports[i] = p.RemotePort

		// Create a target port spec for each port
		// This is needed because the Manager.forwardServicePort function checks this
		targetPort := intstr.FromInt(int(p.RemotePort))
		targetPortSpecs[i] = &targetPort
	}

	// Create a resource object
	return ui.Resource{
		Name:            entry.Name,
		Namespace:       namespace,
		Type:            resourceType,
		Ports:           ports,
		PortNames:       make([]string, len(entry.Ports)), // Empty port names
		TargetPortSpecs: targetPortSpecs,
		DisplayName:     fmt.Sprintf("%s/%s", entry.ResourceType, entry.Name),
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

// GenerateConfig creates a ForwardingConfig from a list of resources and port mappings
func GenerateConfig(resources []ui.Resource, portMappings map[string]map[int]int32, resolvedPorts map[string]map[int]int32, defaultNamespace string) *ForwardingConfig {
	config := &ForwardingConfig{
		DefaultNamespace: defaultNamespace,
		Resources:        make([]PortForwardEntry, 0, len(resources)),
	}

	for _, resource := range resources {
		// Create a new entry
		entry := PortForwardEntry{
			Name:  resource.Name,
			Ports: make([]PortMapping, 0, len(resource.Ports)),
		}

		// Set resource type based on the ui.ResourceType
		switch resource.Type {
		case ui.ServiceResource:
			entry.ResourceType = "service"
		case ui.PodResource:
			entry.ResourceType = "pod"
		case ui.DeploymentResource:
			entry.ResourceType = "deployment"
		case ui.StatefulSetResource:
			entry.ResourceType = "statefulset"
		}

		// Set namespace if different from default
		if resource.Namespace != defaultNamespace {
			entry.Namespace = resource.Namespace
		}

		// Get port mappings for this resource
		portMap := portMappings[resource.Name]

		// Get resolved ports for services
		resolvedPortMap := make(map[int]int32)
		if resource.Type == ui.ServiceResource {
			if resolved, ok := resolvedPorts[resource.Name]; ok {
				resolvedPortMap = resolved
			}
		}

		// Add each port mapping
		for i, remotePort := range resource.Ports {
			localPort := remotePort // Default to same as remote port

			// If we have a specific mapping, use it
			if specifiedPort, exists := portMap[i]; exists {
				localPort = specifiedPort
			}

			// For services, use the resolved container port if available
			targetPort := remotePort
			if resource.Type == ui.ServiceResource {
				if resolvedTargetPort, exists := resolvedPortMap[i]; exists {
					targetPort = resolvedTargetPort
				}
			}

			entry.Ports = append(entry.Ports, PortMapping{
				LocalPort:  localPort,
				RemotePort: targetPort, // Use targetPort which may be the resolved container port
			})
		}

		config.Resources = append(config.Resources, entry)
	}

	return config
}

// WriteConfig writes a ForwardingConfig to a YAML file
func WriteConfig(config *ForwardingConfig, filePath string) error {
	// Generate YAML with comments
	content := "# Configuration file for kubectl-pfw\n"
	content += "# Generated by kubectl-pfw\n"
	content += "# Usage: kubectl pfw -f " + filePath + "\n"
	content += "# \n"
	content += "# This file can be used to consistently port-forward the same resources.\n"
	content += "# For services, the remotePort values are the resolved container ports (target ports),\n"
	content += "# not the service ports. This ensures that port forwarding works correctly.\n"
	content += "# Edit as needed to adjust port mappings or add/remove resources.\n\n"

	// Marshal the config to YAML
	yamlData, err := yaml.Marshal(config)
	if err != nil {
		return fmt.Errorf("failed to marshal config to YAML: %w", err)
	}

	// Combine the header and YAML data
	content += string(yamlData)

	// Write to file
	err = os.WriteFile(filePath, []byte(content), 0644)
	if err != nil {
		return fmt.Errorf("failed to write config file: %w", err)
	}

	return nil
}

// ResolveTargetPorts resolves service ports to actual container ports for services
// Returns a map of resource names to a map of port indices to resolved container ports
func ResolveTargetPorts(ctx context.Context, resources []ui.Resource, k8sClient *k8s.Client) (map[string]map[int]int32, error) {
	resolvedPorts := make(map[string]map[int]int32)

	for _, resource := range resources {
		// Only process service resources
		if resource.Type != ui.ServiceResource {
			continue
		}

		// Find pods that back this service
		pods, err := k8sClient.GetPodsForService(ctx, resource.Name)
		if err != nil {
			return nil, fmt.Errorf("failed to find pods for service %s: %w", resource.Name, err)
		}

		// Get the first pod (same logic as in portforward.Manager.forwardServicePort)
		var selectedPod *k8s.Pod
		for i := range pods {
			selectedPod = &pods[i]
			break
		}

		if selectedPod == nil {
			return nil, fmt.Errorf("no pods found for service %s", resource.Name)
		}

		// Create mapping of port indices to resolved container ports
		portMap := make(map[int]int32)
		resolvedPorts[resource.Name] = portMap

		// Resolve each port
		for i, servicePort := range resource.Ports {
			// Skip if we don't have the target port spec
			if i >= len(resource.TargetPortSpecs) || resource.TargetPortSpecs[i] == nil {
				continue
			}

			// Use the same logic as in portforward.Manager.resolveTargetPort
			targetSpec := resource.TargetPortSpecs[i]

			var resolvedPort int32

			// Logic from portforward.Manager.resolveTargetPort
			if targetSpec == nil {
				// Default to service port
				resolvedPort = servicePort
			} else {
				switch targetSpec.Type {
				case intstr.Int:
					// If IntVal is 0, default to service port
					if targetSpec.IntVal == 0 {
						resolvedPort = servicePort
					} else {
						// Use specified numeric target port
						resolvedPort = targetSpec.IntVal
					}
				case intstr.String:
					// Find container port with matching name
					found := false
					portName := targetSpec.StrVal
					for _, podPort := range selectedPod.Ports {
						if podPort.Name == portName {
							resolvedPort = podPort.ContainerPort
							found = true
							break
						}
					}

					if !found {
						return nil, fmt.Errorf("named target port '%s' not found on pod '%s' in namespace '%s'",
							portName, selectedPod.Name, selectedPod.Namespace)
					}
				default:
					return nil, fmt.Errorf("unknown targetPort type: %v", targetSpec.Type)
				}
			}

			// Store the resolved port
			portMap[i] = resolvedPort
		}
	}

	return resolvedPorts, nil
}
