package portforward

import (
	"fmt"
	"net"
	"testing"
)

// TestNewPortAllocator verifies that a new PortAllocator is initialized correctly.
func TestNewPortAllocator(t *testing.T) {
	pa := NewPortAllocator()
	if pa == nil {
		t.Fatal("expected non-nil PortAllocator")
	}
	if len(pa.allocatedPorts) != 0 {
		t.Errorf("expected allocatedPorts to be empty, got %d", len(pa.allocatedPorts))
	}
}

// TestAllocateAndReleasePort tests allocating and releasing specific and ephemeral ports.
func TestAllocateAndReleasePort(t *testing.T) {
	pa := NewPortAllocator()

	t.Run("allocate ephemeral port", func(t *testing.T) {
		port, err := pa.AllocatePort(0)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if port == 0 {
			t.Error("expected non-zero ephemeral port")
		}
		pa.ReleasePort(port)
	})

	t.Run("allocate specific port", func(t *testing.T) {
		// Find an available port
		var testPort int32 = 0
		for p := int32(30000); p < 30100; p++ {
			if IsPortAvailable(p) {
				testPort = p
				break
			}
		}
		if testPort == 0 {
			t.Skip("no available test port found in range")
		}
		port, err := pa.AllocatePort(testPort)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if port != testPort {
			t.Errorf("expected port %d, got %d", testPort, port)
		}
		// Allocating again should fail
		_, err = pa.AllocatePort(testPort)
		if err == nil {
			t.Error("expected error when allocating already allocated port")
		}
		pa.ReleasePort(testPort)
	})
}

// TestIsPortAvailable checks that IsPortAvailable returns true for a free port and false for a bound port.
func TestIsPortAvailable(t *testing.T) {
	// Find an available port
	var testPort int32 = 0
	for p := int32(30000); p < 30100; p++ {
		if IsPortAvailable(p) {
			testPort = p
			break
		}
	}
	if testPort == 0 {
		t.Skip("no available test port found in range")
	}
	if !IsPortAvailable(testPort) {
		t.Errorf("expected port %d to be available", testPort)
	}
	// Bind to the port
	ln, err := listenOnPort(testPort)
	if err != nil {
		t.Fatalf("failed to listen on port %d: %v", testPort, err)
	}
	defer ln.Close()
	if IsPortAvailable(testPort) {
		t.Errorf("expected port %d to be unavailable", testPort)
	}
}

// listenOnPort is a helper to bind to a port for testing.
func listenOnPort(port int32) (interface{ Close() error }, error) {
	return net.Listen("tcp", fmt.Sprintf(":%d", port))
}
