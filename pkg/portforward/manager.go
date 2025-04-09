package portforward

import (
	"context"
	"fmt"
	"os"
	"os/signal"
	"sync"
	"syscall"
	"time"

	"roeyazroel/kubectl-pfw/pkg/k8s"
	"roeyazroel/kubectl-pfw/pkg/ui"

	"k8s.io/apimachinery/pkg/util/intstr"
	"k8s.io/cli-runtime/pkg/genericclioptions"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/rest"
)

// Manager manages multiple port forwarding connections
type Manager struct {
	RestConfig  *rest.Config
	ClientSet   *kubernetes.Clientset
	K8sClient   *k8s.Client
	Streams     genericclioptions.IOStreams
	Context     context.Context
	Forwarders  []*PortForwarder
	ForwardWait sync.WaitGroup
	mutex       sync.Mutex
	// Port allocator for managing local ports
	PortAllocator *PortAllocator
}

// NewManager creates a new port forward manager
func NewManager(config *rest.Config, clientset *kubernetes.Clientset, k8sClient *k8s.Client, streams genericclioptions.IOStreams, ctx context.Context) *Manager {
	return &Manager{
		RestConfig:    config,
		ClientSet:     clientset,
		K8sClient:     k8sClient,
		Streams:       streams,
		Context:       ctx,
		Forwarders:    []*PortForwarder{},
		PortAllocator: NewPortAllocator(),
	}
}

// ForwardResource starts port forwarding for a resource
func (m *Manager) ForwardResource(resource ui.Resource, portMapping map[int]int32) error {
	m.mutex.Lock()
	defer m.mutex.Unlock()

	for i, portValue := range resource.Ports {
		// Get local port. Check if explicitly mapped by user
		var localPort int32
		if mappedPort, ok := portMapping[i]; ok {
			// Use the explicitly mapped port (may be 0 for ephemeral)
			localPort = mappedPort
		} else {
			// If no explicit mapping, default local port depends on the *target*
			if resource.Type == ui.ServiceResource || resource.Type == ui.DeploymentResource || resource.Type == ui.StatefulSetResource {
				// We don't know the resolved target port yet. Set to 0 and determine in forward*Port.
				localPort = 0 // Will allocate an ephemeral port later
			} else {
				// For pods, the target *is* the container port.
				localPort = portValue // Default local to container port
			}
		}

		switch resource.Type {
		case ui.ServiceResource:
			// portValue represents the service port here
			servicePort := portValue
			// Pass localPort (might be 0 if defaulting or for ephemeral port allocation)
			err := m.forwardServicePort(resource, i, localPort, servicePort)
			if err != nil {
				return err // Propagate error from forwarding attempt
			}
		case ui.DeploymentResource:
			err := m.forwardDeploymentPort(resource, i, localPort, portValue)
			if err != nil {
				return err
			}
		case ui.StatefulSetResource:
			err := m.forwardStatefulSetPort(resource, i, localPort, portValue)
			if err != nil {
				return err
			}
		default: // PodResource
			// portValue represents the container port here
			podContainerPort := portValue

			// If localPort was 0 (ephemeral case), allocate a port
			if localPort == 0 {
				// Use a default suggestion based on the remote port
				suggestedPort := podContainerPort

				// Try to allocate that port
				allocatedPort, err := m.PortAllocator.AllocatePort(suggestedPort)
				if err != nil {
					// If the suggested port is unavailable, try to get any available port
					allocatedPort, err = m.PortAllocator.AllocatePort(0)
					if err != nil {
						return fmt.Errorf("failed to allocate local port: %w", err)
					}
				}
				localPort = allocatedPort
			} else {
				// If a specific port was requested, try to allocate it
				_, err := m.PortAllocator.AllocatePort(localPort)
				if err != nil {
					return fmt.Errorf("failed to allocate requested local port %d: %w", localPort, err)
				}
			}

			// For pods, forward directly
			req := ForwardRequest{
				RestConfig: m.RestConfig,
				ClientSet:  m.ClientSet,
				Resource:   resource,
				LocalPort:  localPort,        // Use allocated port
				RemotePort: podContainerPort, // Use the container port directly as the remote port
				Streams:    m.Streams,
				Context:    m.Context,
				// PodName is not needed when forwarding directly to a pod
				AutoRetry: true, // Enable auto-retry by default
			}

			forwarder, err := StartPortForward(req)
			if err != nil {
				// Release the allocated port
				m.PortAllocator.ReleasePort(localPort)
				// Stop any previously started forwarders for this resource
				m.stopResourceForwarders(resource)
				return fmt.Errorf("failed to start port forward for %s: %w", resource.Name, err)
			}

			m.Forwarders = append(m.Forwarders, forwarder)
			m.startForwarderMonitor(forwarder)
		}
	}

	return nil
}

// forwardServicePort handles port forwarding for a service by finding a backing pod and resolving the target port
func (m *Manager) forwardServicePort(resource ui.Resource, portIndex int, localPort, servicePort int32) error {
	// Get the target port spec for this service port
	if portIndex >= len(resource.TargetPortSpecs) {
		return fmt.Errorf("port index %d out of bounds for target port specs of service %s", portIndex, resource.Name)
	}
	targetSpec := resource.TargetPortSpecs[portIndex]

	// Find pods that back this service
	pods, err := m.K8sClient.GetPodsForService(m.Context, resource.Name)
	if err != nil {
		// If pods cannot be found, we cannot forward.
		return fmt.Errorf("failed to find pods for service %s: %w", resource.Name, err)
	}

	// Use the first ready pod
	// TODO: Implement better pod selection (e.g., check readiness, round-robin?)
	var selectedPod *k8s.Pod
	for i := range pods {
		// Simple selection of the first pod for now
		selectedPod = &pods[i]
		break
	}

	if selectedPod == nil {
		return fmt.Errorf("no pods found for service %s to forward port %d", resource.Name, servicePort)
	}

	// Resolve the target container port on the selected pod
	resolvedPodPort, err := resolveTargetPort(targetSpec, servicePort, *selectedPod)
	if err != nil {
		// If target port cannot be resolved (e.g., named port not found), we cannot forward this specific port.
		return fmt.Errorf("failed to resolve target port for service %s port %d on pod %s: %w", resource.Name, servicePort, selectedPod.Name, err)
	}

	// If localPort was 0 (ephemeral case), allocate a port
	if localPort == 0 {
		// Try to use the resolved pod port as a suggestion
		suggestedPort := resolvedPodPort

		// Try to allocate that port
		allocatedPort, err := m.PortAllocator.AllocatePort(suggestedPort)
		if err != nil {
			// If the suggested port is unavailable, try to get any available port
			allocatedPort, err = m.PortAllocator.AllocatePort(0)
			if err != nil {
				return fmt.Errorf("failed to allocate local port: %w", err)
			}
		}
		localPort = allocatedPort
	} else {
		// If a specific port was requested, try to allocate it
		_, err := m.PortAllocator.AllocatePort(localPort)
		if err != nil {
			return fmt.Errorf("failed to allocate requested local port %d: %w", localPort, err)
		}
	}

	// Start port forwarding to the selected pod and resolved port
	req := ForwardRequest{
		RestConfig: m.RestConfig,
		ClientSet:  m.ClientSet,
		Resource:   resource,
		LocalPort:  localPort,
		RemotePort: resolvedPodPort, // Use the RESOLVED container port
		Streams:    m.Streams,
		Context:    m.Context,
		PodName:    selectedPod.Name, // Pod needed for the port-forward API call
		AutoRetry:  true,             // Enable auto-retry by default
	}

	forwarder, err := StartPortForward(req)
	if err != nil {
		// Release the allocated port
		m.PortAllocator.ReleasePort(localPort)
		// Stop any previously started forwarders
		m.stopResourceForwarders(resource)
		return fmt.Errorf("failed to start port forward for service %s via pod %s: %w",
			resource.Name, selectedPod.Name, err)
	}

	m.Forwarders = append(m.Forwarders, forwarder)
	m.startForwarderMonitor(forwarder)
	return nil
}

// forwardDeploymentPort handles port forwarding for a deployment by finding a backing pod
func (m *Manager) forwardDeploymentPort(resource ui.Resource, portIndex int, localPort, deploymentPort int32) error {
	// Find pods that back this deployment
	pods, err := m.K8sClient.GetPodsForDeployment(m.Context, resource.Name)
	if err != nil {
		return fmt.Errorf("failed to find pods for deployment %s: %w", resource.Name, err)
	}

	// Use the first ready pod
	// TODO: Implement better pod selection (e.g., check readiness, round-robin?)
	var selectedPod *k8s.Pod
	for i := range pods {
		// Simple selection of the first pod for now
		selectedPod = &pods[i]
		break
	}

	if selectedPod == nil {
		return fmt.Errorf("no pods found for deployment %s to forward port", resource.Name)
	}

	// Find the container port in the selected pod
	// Unlike services, we need to find the actual container port
	var podPort int32
	if portIndex < len(selectedPod.Ports) {
		podPort = selectedPod.Ports[portIndex].ContainerPort
	} else if len(selectedPod.Ports) > 0 {
		// If port index is out of bounds but pod has ports, use the first port
		podPort = selectedPod.Ports[0].ContainerPort
	} else {
		return fmt.Errorf("no container ports found in pod %s for deployment %s", selectedPod.Name, resource.Name)
	}

	// If localPort was 0 (ephemeral case), allocate a port
	if localPort == 0 {
		// Try to use the pod port as a suggestion
		suggestedPort := podPort

		// Try to allocate that port
		allocatedPort, err := m.PortAllocator.AllocatePort(suggestedPort)
		if err != nil {
			// If the suggested port is unavailable, try to get any available port
			allocatedPort, err = m.PortAllocator.AllocatePort(0)
			if err != nil {
				return fmt.Errorf("failed to allocate local port: %w", err)
			}
		}
		localPort = allocatedPort
	} else {
		// If a specific port was requested, try to allocate it
		_, err := m.PortAllocator.AllocatePort(localPort)
		if err != nil {
			return fmt.Errorf("failed to allocate requested local port %d: %w", localPort, err)
		}
	}

	// Start port forwarding to the selected pod
	req := ForwardRequest{
		RestConfig: m.RestConfig,
		ClientSet:  m.ClientSet,
		Resource:   resource,
		LocalPort:  localPort,
		RemotePort: podPort,
		Streams:    m.Streams,
		Context:    m.Context,
		PodName:    selectedPod.Name,
		AutoRetry:  true, // Enable auto-retry by default
	}

	forwarder, err := StartPortForward(req)
	if err != nil {
		// Release the allocated port
		m.PortAllocator.ReleasePort(localPort)
		// Stop any previously started forwarders
		m.stopResourceForwarders(resource)
		return fmt.Errorf("failed to start port forward for deployment %s via pod %s: %w",
			resource.Name, selectedPod.Name, err)
	}

	m.Forwarders = append(m.Forwarders, forwarder)
	m.startForwarderMonitor(forwarder)
	return nil
}

// forwardStatefulSetPort handles port forwarding for a statefulset by finding a backing pod
func (m *Manager) forwardStatefulSetPort(resource ui.Resource, portIndex int, localPort, statefulSetPort int32) error {
	// Find pods that back this statefulset
	pods, err := m.K8sClient.GetPodsForStatefulSet(m.Context, resource.Name)
	if err != nil {
		return fmt.Errorf("failed to find pods for statefulset %s: %w", resource.Name, err)
	}

	// Use the first ready pod
	// TODO: Implement better pod selection (e.g., check readiness, round-robin?)
	var selectedPod *k8s.Pod
	for i := range pods {
		// Simple selection of the first pod for now
		selectedPod = &pods[i]
		break
	}

	if selectedPod == nil {
		return fmt.Errorf("no pods found for statefulset %s to forward port", resource.Name)
	}

	// Find the container port in the selected pod
	// Unlike services, we need to find the actual container port
	var podPort int32
	if portIndex < len(selectedPod.Ports) {
		podPort = selectedPod.Ports[portIndex].ContainerPort
	} else if len(selectedPod.Ports) > 0 {
		// If port index is out of bounds but pod has ports, use the first port
		podPort = selectedPod.Ports[0].ContainerPort
	} else {
		return fmt.Errorf("no container ports found in pod %s for statefulset %s", selectedPod.Name, resource.Name)
	}

	// If localPort was 0 (ephemeral case), allocate a port
	if localPort == 0 {
		// Try to use the pod port as a suggestion
		suggestedPort := podPort

		// Try to allocate that port
		allocatedPort, err := m.PortAllocator.AllocatePort(suggestedPort)
		if err != nil {
			// If the suggested port is unavailable, try to get any available port
			allocatedPort, err = m.PortAllocator.AllocatePort(0)
			if err != nil {
				return fmt.Errorf("failed to allocate local port: %w", err)
			}
		}
		localPort = allocatedPort
	} else {
		// If a specific port was requested, try to allocate it
		_, err := m.PortAllocator.AllocatePort(localPort)
		if err != nil {
			return fmt.Errorf("failed to allocate requested local port %d: %w", localPort, err)
		}
	}

	// Start port forwarding to the selected pod
	req := ForwardRequest{
		RestConfig: m.RestConfig,
		ClientSet:  m.ClientSet,
		Resource:   resource,
		LocalPort:  localPort,
		RemotePort: podPort,
		Streams:    m.Streams,
		Context:    m.Context,
		PodName:    selectedPod.Name,
		AutoRetry:  true, // Enable auto-retry by default
	}

	forwarder, err := StartPortForward(req)
	if err != nil {
		// Release the allocated port
		m.PortAllocator.ReleasePort(localPort)
		// Stop any previously started forwarders
		m.stopResourceForwarders(resource)
		return fmt.Errorf("failed to start port forward for statefulset %s via pod %s: %w",
			resource.Name, selectedPod.Name, err)
	}

	m.Forwarders = append(m.Forwarders, forwarder)
	m.startForwarderMonitor(forwarder)
	return nil
}

// startForwarderMonitor starts a goroutine to monitor the forwarding status
func (m *Manager) startForwarderMonitor(forwarder *PortForwarder) {
	m.ForwardWait.Add(1)

	// Wait for ready or error
	go func(pf *PortForwarder) {
		defer m.ForwardWait.Done()
		select {
		case <-pf.ReadyChannel:
			fmt.Fprintf(m.Streams.Out, "%s\n", pf.GetPortForwardString())
			// After ready, wait for either an error or context done
			select {
			case err := <-pf.ErrorChannel:
				fmt.Fprintf(m.Streams.ErrOut, "Error forwarding ports for %s: %v\n", pf.Resource.Name, err)
				// Release the port when forwarding ends
				m.PortAllocator.ReleasePort(pf.LocalPort)
			case <-m.Context.Done():
				// Release the port when forwarding ends
				m.PortAllocator.ReleasePort(pf.LocalPort)
				// No need to call pf.Stop() here, manager.Stop() handles it
			}
		case err := <-pf.ErrorChannel:
			fmt.Fprintf(m.Streams.ErrOut, "Error forwarding ports for %s: %v\n", pf.Resource.Name, err)
			// Release the port when forwarding ends with error
			m.PortAllocator.ReleasePort(pf.LocalPort)
		case <-m.Context.Done():
			// Release the port when forwarding ends
			m.PortAllocator.ReleasePort(pf.LocalPort)
			// No need to call pf.Stop() here, manager.Stop() handles it
		}
	}(forwarder)
}

// stopResourceForwarders stops forwarders for a specific resource
func (m *Manager) stopResourceForwarders(resource ui.Resource) {
	for _, fw := range m.Forwarders {
		if fw.Resource.Name == resource.Name && fw.Resource.Namespace == resource.Namespace {
			fw.Stop()
			// Release the port
			m.PortAllocator.ReleasePort(fw.LocalPort)
		}
	}
}

// Stop stops all port forwarding
func (m *Manager) Stop() {
	m.mutex.Lock()
	defer m.mutex.Unlock()

	for _, forwarder := range m.Forwarders {
		forwarder.Stop()
		// Release the port
		m.PortAllocator.ReleasePort(forwarder.LocalPort)
	}
}

// SetupSignalHandler sets up a signal handler to stop port forwarding on interrupt
func (m *Manager) SetupSignalHandler() {
	signals := make(chan os.Signal, 1)
	signal.Notify(signals, syscall.SIGINT, syscall.SIGTERM)

	go func() {
		<-signals
		fmt.Fprintln(m.Streams.Out, "\nShutting down port forwarding...")
		m.Stop()

		// Wait briefly for port forwards to clean up
		time.Sleep(500 * time.Millisecond)

		// Force exit to handle the case where some goroutines are stuck
		os.Exit(0)
	}()
}

// WaitForCompletion waits for all port forwards to complete
func (m *Manager) WaitForCompletion() {
	m.ForwardWait.Wait()
}

// resolveTargetPort determines the numeric target port on a pod corresponding to a service's targetPort spec.
func resolveTargetPort(targetSpec *intstr.IntOrString, servicePort int32, pod k8s.Pod) (int32, error) {
	if targetSpec == nil {
		// This case should generally not be hit if a service has ports, but handle defensively.
		// Default to the service port like Kubernetes does.
		return servicePort, nil
	}

	switch targetSpec.Type {
	case intstr.Int:
		// If IntVal is 0, it means TargetPort was not specified, default to ServicePort
		if targetSpec.IntVal == 0 {
			return servicePort, nil
		}
		// Otherwise, use the specified numeric target port
		return targetSpec.IntVal, nil
	case intstr.String:
		// Find the container port with the matching name
		portName := targetSpec.StrVal
		for _, podPort := range pod.Ports {
			if podPort.Name == portName {
				return podPort.ContainerPort, nil
			}
		}
		return 0, fmt.Errorf("named target port '%s' not found on pod '%s' in namespace '%s'", portName, pod.Name, pod.Namespace)
	default:
		return 0, fmt.Errorf("unknown targetPort type: %v", targetSpec.Type)
	}
}
