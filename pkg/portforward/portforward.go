package portforward

import (
	"context"
	"fmt"
	"net/http"
	"net/url"
	"strings"
	"time"

	"roeyazroel/kubectl-pfw/pkg/ui"

	"k8s.io/cli-runtime/pkg/genericclioptions"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/rest"
	"k8s.io/client-go/tools/portforward"
	"k8s.io/client-go/transport/spdy"
)

// MaxRetries is the maximum number of reconnection attempts
const (
	MaxRetries      = 5
	InitialBackoff  = 1 * time.Second
	MaxBackoff      = 30 * time.Second
	BackoffFactor   = 2    // Exponential backoff factor
	AutoRetryEnable = true // Default setting for auto-retry
)

// PortForwarder represents a port forwarding connection
type PortForwarder struct {
	Resource     ui.Resource
	LocalPort    int32
	RemotePort   int32
	StopChannel  chan struct{}
	ReadyChannel chan struct{}
	ForwardFn    *portforward.PortForwarder
	ErrorChannel chan error
	// Auto-retry settings
	AutoRetry     bool
	RetryAttempts int
}

// ForwardRequest contains the information needed to start port forwarding
type ForwardRequest struct {
	RestConfig *rest.Config
	ClientSet  *kubernetes.Clientset
	Resource   ui.Resource
	LocalPort  int32
	RemotePort int32 // For Pods: the container port; For Services/Deployments/StatefulSets: the target port
	Streams    genericclioptions.IOStreams
	Context    context.Context
	// If not pod type, we need to port-forward to a specific pod
	PodName string
	// Auto-retry settings
	AutoRetry bool
	// TargetPort field removed - not needed as K8s handles service->pod target port resolution.
}

// StartPortForward starts a port forward connection for a service or pod
func StartPortForward(req ForwardRequest) (*PortForwarder, error) {
	var path string
	var remotePort int32

	// Determine the path and remote port based on resource type
	switch req.Resource.Type {
	case ui.ServiceResource, ui.DeploymentResource, ui.StatefulSetResource:
		// For services, deployments, and statefulsets, we need to port-forward to a pod
		if req.PodName == "" {
			return nil, fmt.Errorf("pod name is required for %s port forwarding", req.Resource.Type)
		}
		path = fmt.Sprintf("/api/v1/namespaces/%s/pods/%s/portforward", req.Resource.Namespace, req.PodName)
		// Use the service/deployment/statefulset port. Kubernetes port-forward handles TargetPort resolution.
		remotePort = req.RemotePort
	case ui.PodResource:
		path = fmt.Sprintf("/api/v1/namespaces/%s/pods/%s/portforward", req.Resource.Namespace, req.Resource.Name)
		remotePort = req.RemotePort
	default:
		return nil, fmt.Errorf("unsupported resource type: %s", req.Resource.Type)
	}

	hostIP := strings.TrimPrefix(req.RestConfig.Host, "https://")

	transport, upgrader, err := spdy.RoundTripperFor(req.RestConfig)
	if err != nil {
		return nil, fmt.Errorf("failed to create round tripper: %w", err)
	}

	dialer := spdy.NewDialer(upgrader, &http.Client{Transport: transport}, http.MethodPost, &url.URL{
		Scheme: "https",
		Path:   path,
		Host:   hostIP,
	})

	stopChannel := make(chan struct{}, 1)
	readyChannel := make(chan struct{}, 1)
	errorChannel := make(chan error, 1)

	// Format as localPort:remotePort
	// The remotePort is now correctly set to the service port for services, or pod port for pods.
	ports := []string{fmt.Sprintf("%d:%d", req.LocalPort, remotePort)}

	// Default to global setting if not specified in request
	autoRetry := AutoRetryEnable
	if req.AutoRetry {
		autoRetry = req.AutoRetry
	}

	forwarder := &PortForwarder{
		Resource:      req.Resource,
		LocalPort:     req.LocalPort,
		RemotePort:    req.RemotePort, // Note: we're keeping the logical service port here for display
		StopChannel:   stopChannel,
		ReadyChannel:  readyChannel,
		ForwardFn:     nil, // Will be set in the goroutine
		ErrorChannel:  errorChannel,
		AutoRetry:     autoRetry,
		RetryAttempts: 0,
	}

	// Start port forwarding in a goroutine
	go func() {
		var retryCount int
		var backoff time.Duration = InitialBackoff

		// Create a new pf instance to use within this loop
		pf, err := portforward.New(dialer, ports, stopChannel, readyChannel, req.Streams.Out, req.Streams.ErrOut)
		if err != nil {
			errorChannel <- fmt.Errorf("failed to create port forwarder: %w", err)
			close(stopChannel)
			return
		}

		// Set the ForwardFn so the caller can reference it
		forwarder.ForwardFn = pf

		for {
			// Check if we should stop
			select {
			case <-stopChannel:
				return
			default:
				// Continue with the forwarding
			}

			// Start the port forwarding
			err := pf.ForwardPorts()

			// If forwarding ended without error, just return
			if err == nil {
				return
			}

			// Error occurred, decide whether to retry
			if !forwarder.AutoRetry || retryCount >= MaxRetries {
				// Either auto-retry is disabled or we've reached the max retry count
				errorChannel <- fmt.Errorf("port forwarding failed after %d attempts: %w", retryCount+1, err)
				close(stopChannel)
				return
			}

			// Log the retry attempt
			fmt.Fprintf(req.Streams.ErrOut, "Port forwarding error: %v. Retrying (%d/%d) in %v...\n",
				err, retryCount+1, MaxRetries, backoff)

			// Wait before retrying
			select {
			case <-stopChannel:
				return
			case <-time.After(backoff):
				// Continue with retry
			}

			// Create a new port forwarder for the retry
			pf, err = portforward.New(dialer, ports, stopChannel, readyChannel, req.Streams.Out, req.Streams.ErrOut)
			if err != nil {
				errorChannel <- fmt.Errorf("failed to create port forwarder for retry: %w", err)
				close(stopChannel)
				return
			}

			// Update forwarder's pf reference
			forwarder.ForwardFn = pf

			// Increase retry count and backoff
			retryCount++
			forwarder.RetryAttempts = retryCount

			// Apply exponential backoff with a maximum limit
			backoff = time.Duration(float64(backoff) * BackoffFactor)
			if backoff > MaxBackoff {
				backoff = MaxBackoff
			}
		}
	}()

	return forwarder, nil
}

// Stop stops the port forwarding
func (pf *PortForwarder) Stop() {
	close(pf.StopChannel)
}

// GetPortForwardString returns a string representation of the port forwarding
func (pf *PortForwarder) GetPortForwardString() string {
	var resourceType string

	switch pf.Resource.Type {
	case ui.ServiceResource:
		resourceType = "service"
	case ui.DeploymentResource:
		resourceType = "deployment"
	case ui.StatefulSetResource:
		resourceType = "statefulset"
	default:
		resourceType = "pod"
	}

	// Simplified message showing the actual local and remote (container) ports being used.
	return fmt.Sprintf("Forwarding %s/%s (target port %d) -> localhost:%d",
		resourceType, pf.Resource.Name, pf.RemotePort, pf.LocalPort)
}
