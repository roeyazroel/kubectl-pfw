package portforward

import (
	"testing"

	"roeyazroel/kubectl-pfw/pkg/ui"
)

// TestPortForwarder_GetPortForwardString verifies the output string for various resource types.
func TestPortForwarder_GetPortForwardString(t *testing.T) {
	cases := []struct {
		name     string
		pf       PortForwarder
		expected string
	}{
		{
			name: "service resource",
			pf: PortForwarder{
				Resource:   ui.Resource{Name: "svc1", Namespace: "ns1", Type: ui.ServiceResource},
				LocalPort:  8080,
				RemotePort: 80,
			},
			expected: "Forwarding service/svc1 (target port 80) -> localhost:8080",
		},
		{
			name: "deployment resource",
			pf: PortForwarder{
				Resource:   ui.Resource{Name: "dep1", Namespace: "ns1", Type: ui.DeploymentResource},
				LocalPort:  8081,
				RemotePort: 81,
			},
			expected: "Forwarding deployment/dep1 (target port 81) -> localhost:8081",
		},
		{
			name: "statefulset resource",
			pf: PortForwarder{
				Resource:   ui.Resource{Name: "ss1", Namespace: "ns1", Type: ui.StatefulSetResource},
				LocalPort:  8082,
				RemotePort: 82,
			},
			expected: "Forwarding statefulset/ss1 (target port 82) -> localhost:8082",
		},
		{
			name: "pod resource",
			pf: PortForwarder{
				Resource:   ui.Resource{Name: "pod1", Namespace: "ns1", Type: ui.PodResource},
				LocalPort:  8083,
				RemotePort: 83,
			},
			expected: "Forwarding pod/pod1 (target port 83) -> localhost:8083",
		},
	}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			got := c.pf.GetPortForwardString()
			if got != c.expected {
				t.Errorf("expected %q, got %q", c.expected, got)
			}
		})
	}
}

// TestPortForwarder_Stop verifies that Stop closes the StopChannel.
func TestPortForwarder_Stop(t *testing.T) {
	pf := &PortForwarder{
		StopChannel: make(chan struct{}, 1),
	}
	pf.Stop()
	select {
	case <-pf.StopChannel:
		// ok
	default:
		t.Error("expected StopChannel to be closed after Stop")
	}
}
