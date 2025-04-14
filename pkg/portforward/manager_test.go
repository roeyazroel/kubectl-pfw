package portforward

import (
	"context"
	"testing"

	"roeyazroel/kubectl-pfw/pkg/k8s"

	"k8s.io/cli-runtime/pkg/genericclioptions"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/rest"
)

// TestNewManager verifies that NewManager returns a properly initialized Manager.
func TestNewManager(t *testing.T) {
	cfg := &rest.Config{}
	clientset := &kubernetes.Clientset{}
	k8sClient := &k8s.Client{} // Use real type, not a mock
	streams := genericclioptions.IOStreams{}
	ctx := context.Background()

	mgr := NewManager(cfg, clientset, k8sClient, streams, ctx)
	if mgr == nil {
		t.Fatal("expected non-nil Manager")
	}
	if mgr.RestConfig != cfg {
		t.Error("RestConfig not set correctly")
	}
	if mgr.ClientSet != clientset {
		t.Error("ClientSet not set correctly")
	}
	if mgr.K8sClient != k8sClient {
		t.Error("K8sClient not set correctly")
	}
	if mgr.Streams != streams {
		t.Error("Streams not set correctly")
	}
	if mgr.Context != ctx {
		t.Error("Context not set correctly")
	}
	if mgr.PortAllocator == nil {
		t.Error("PortAllocator should not be nil")
	}
}

// TestManager_Stop verifies that Stop releases all ports and stops all forwarders.
func TestManager_Stop(t *testing.T) {
	mgr := &Manager{
		PortAllocator: NewPortAllocator(),
		Forwarders:    []*PortForwarder{},
	}
	// Add a dummy forwarder
	pf := &PortForwarder{
		LocalPort:   12345,
		StopChannel: make(chan struct{}, 1),
	}
	mgr.Forwarders = append(mgr.Forwarders, pf)
	mgr.PortAllocator.allocatedPorts[12345] = true

	mgr.Stop()
	if mgr.PortAllocator.allocatedPorts[12345] {
		t.Error("expected port to be released after Stop")
	}
	select {
	case <-pf.StopChannel:
		// ok
	default:
		t.Error("expected StopChannel to be closed")
	}
}
