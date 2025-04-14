package ui

import (
	"fmt"

	"roeyazroel/kubectl-pfw/pkg/k8s"

	"github.com/AlecAivazis/survey/v2"
	"k8s.io/apimachinery/pkg/util/intstr"
)

// ResourceType represents the type of Kubernetes resource
type ResourceType string

const (
	// ServiceResource represents a Kubernetes service
	ServiceResource ResourceType = "service"
	// PodResource represents a Kubernetes pod
	PodResource ResourceType = "pod"
	// DeploymentResource represents a Kubernetes deployment
	DeploymentResource ResourceType = "deployment"
	// StatefulSetResource represents a Kubernetes statefulset
	StatefulSetResource ResourceType = "statefulset"
)

// Resource represents a Kubernetes resource that can be port-forwarded
type Resource struct {
	Name            string
	Namespace       string
	Type            ResourceType
	Ports           []int32               // ServicePort or ContainerPort
	PortNames       []string              // Name of the port (if specified)
	TargetPortSpecs []*intstr.IntOrString // For services, the original targetPort spec
	DisplayName     string
	PortMetadata    []k8s.PortMetadata // Additional metadata about ports (like init container info)
}

// NewResourceFromService creates a Resource from a k8s.Service
func NewResourceFromService(svc k8s.Service) Resource {
	ports := make([]int32, len(svc.Ports))
	portNames := make([]string, len(svc.Ports))
	targetPortSpecs := make([]*intstr.IntOrString, len(svc.Ports))

	for i, port := range svc.Ports {
		ports[i] = port.Port
		portNames[i] = port.Name
		targetPortSpecs[i] = port.TargetPortSpec
	}

	return Resource{
		Name:            svc.Name,
		Namespace:       svc.Namespace,
		Type:            ServiceResource,
		Ports:           ports,
		PortNames:       portNames,
		TargetPortSpecs: targetPortSpecs,
		DisplayName:     k8s.ServiceToString(svc),
	}
}

// NewResourceFromPod creates a Resource from a k8s.Pod
func NewResourceFromPod(pod k8s.Pod) Resource {
	ports := make([]int32, len(pod.Ports))
	portNames := make([]string, len(pod.Ports))
	targetPortSpecs := make([]*intstr.IntOrString, len(pod.Ports))
	portMetadata := make([]k8s.PortMetadata, len(pod.Ports))

	for i, port := range pod.Ports {
		ports[i] = port.ContainerPort
		portNames[i] = port.Name
		intOrStr := intstr.FromInt(int(port.ContainerPort))
		targetPortSpecs[i] = &intOrStr

		// Store metadata about the port
		portMetadata[i] = k8s.PortMetadata{
			ContainerName:   port.ContainerName,
			IsInitContainer: port.IsInitContainer,
		}
	}

	return Resource{
		Name:            pod.Name,
		Namespace:       pod.Namespace,
		Type:            PodResource,
		Ports:           ports,
		PortNames:       portNames,
		TargetPortSpecs: targetPortSpecs,
		DisplayName:     k8s.PodToString(pod),
		PortMetadata:    portMetadata,
	}
}

// NewResourceFromDeployment creates a Resource from a k8s.Deployment
func NewResourceFromDeployment(deployment k8s.Deployment) Resource {
	// For deployments, ports will be populated when resolving pods
	return Resource{
		Name:        deployment.Name,
		Namespace:   deployment.Namespace,
		Type:        DeploymentResource,
		Ports:       []int32{},  // Will be populated when selecting a pod
		PortNames:   []string{}, // Will be populated when selecting a pod
		DisplayName: k8s.DeploymentToString(deployment),
	}
}

// NewResourceFromStatefulSet creates a Resource from a k8s.StatefulSet
func NewResourceFromStatefulSet(statefulSet k8s.StatefulSet) Resource {
	// For statefulsets, ports will be populated when resolving pods
	return Resource{
		Name:        statefulSet.Name,
		Namespace:   statefulSet.Namespace,
		Type:        StatefulSetResource,
		Ports:       []int32{},  // Will be populated when selecting a pod
		PortNames:   []string{}, // Will be populated when selecting a pod
		DisplayName: k8s.StatefulSetToString(statefulSet),
	}
}

// SelectResources displays a multi-select UI for services or pods
func SelectResources(resources []Resource, message string) ([]Resource, error) {
	if len(resources) == 0 {
		return nil, fmt.Errorf("no resources available for selection")
	}

	options := make([]string, len(resources))
	for i, resource := range resources {
		options[i] = resource.DisplayName
	}

	selected := []int{}
	prompt := &survey.MultiSelect{
		Message:  message,
		Options:  options,
		Help:     "Use arrow keys to navigate, space to select, and enter to confirm",
		PageSize: len(options),
	}

	err := survey.AskOne(prompt, &selected)
	if err != nil {
		return nil, fmt.Errorf("selection error: %w", err)
	}

	if len(selected) == 0 {
		return nil, fmt.Errorf("no resources selected")
	}

	selectedResources := make([]Resource, len(selected))
	for i, idx := range selected {
		selectedResources[i] = resources[idx]
	}

	return selectedResources, nil
}

// AskForLocalPort asks the user to confirm or change the local port
func AskForLocalPort(resource Resource, suggestedPort int32, portIndex int) (int32, error) {
	// Get port name and container info
	portName := ""
	isInitContainer := false
	containerName := ""

	if portIndex < len(resource.PortNames) {
		if resource.PortNames[portIndex] != "" {
			portName = resource.PortNames[portIndex]
		}

		// Check if port is from a PodResource with IsInitContainer info
		if resource.Type == PodResource && portIndex < len(resource.PortMetadata) {
			metadata := resource.PortMetadata[portIndex]
			isInitContainer = metadata.IsInitContainer
			containerName = metadata.ContainerName
		}
	}

	var message string
	if portName != "" {
		if isInitContainer {
			message = fmt.Sprintf("Local port for %s/%s (remote init container %s port %d)",
				resource.Name, portName, containerName, resource.Ports[portIndex])
		} else {
			message = fmt.Sprintf("Local port for %s/%s (remote port %d)",
				resource.Name, portName, resource.Ports[portIndex])
		}
	} else {
		if isInitContainer {
			message = fmt.Sprintf("Local port for %s (remote init container %s port %d)",
				resource.Name, containerName, resource.Ports[portIndex])
		} else {
			message = fmt.Sprintf("Local port for %s (remote port %d)",
				resource.Name, resource.Ports[portIndex])
		}
	}

	var port string
	prompt := &survey.Input{
		Message: message,
		Default: fmt.Sprintf("%d", suggestedPort),
	}

	err := survey.AskOne(prompt, &port, survey.WithValidator(func(val interface{}) error {
		str, ok := val.(string)
		if !ok {
			return fmt.Errorf("invalid input")
		}

		// Check if the value is a valid port number
		var portNum int
		_, err := fmt.Sscanf(str, "%d", &portNum)
		if err != nil || portNum < 1 || portNum > 65535 {
			return fmt.Errorf("please enter a valid port number (1-65535)")
		}

		return nil
	}))

	if err != nil {
		return 0, err
	}

	var portNum int32
	fmt.Sscanf(port, "%d", &portNum)
	return portNum, nil
}
