package portforward

import (
	"fmt"
	"net"
	"sync"
)

// PortAllocator manages port allocation for port forwarding
type PortAllocator struct {
	// Track ports that have been allocated by this tool
	allocatedPorts map[int32]bool
	// Protect the allocatedPorts map from concurrent access
	mu sync.Mutex
}

// NewPortAllocator creates a new port allocator
func NewPortAllocator() *PortAllocator {
	return &PortAllocator{
		allocatedPorts: make(map[int32]bool),
	}
}

// AllocatePort allocates a port for port forwarding.
// If the requested port is 0, an ephemeral port is allocated.
// If the requested port is already allocated, an error is returned.
func (pa *PortAllocator) AllocatePort(requestedPort int32) (int32, error) {
	// If a specific port was requested (not 0), check if it's available
	if requestedPort > 0 {
		if err := pa.reserveSpecificPort(requestedPort); err != nil {
			return 0, err
		}
		return requestedPort, nil
	}

	// Otherwise, allocate an ephemeral port
	port, err := pa.findAvailableEphemeralPort()
	if err != nil {
		return 0, fmt.Errorf("failed to allocate ephemeral port: %w", err)
	}

	return port, nil
}

// ReleasePort releases a previously allocated port
func (pa *PortAllocator) ReleasePort(port int32) {
	pa.mu.Lock()
	defer pa.mu.Unlock()

	delete(pa.allocatedPorts, port)
}

// findAvailableEphemeralPort finds an available ephemeral port by binding to port 0
func (pa *PortAllocator) findAvailableEphemeralPort() (int32, error) {
	// Bind to port 0 to get an available port from the OS
	listener, err := net.Listen("tcp", ":0")
	if err != nil {
		return 0, fmt.Errorf("failed to bind to ephemeral port: %w", err)
	}

	// Get the actual port number
	port := int32(listener.Addr().(*net.TCPAddr).Port)

	// Close the listener to release the port for use by port-forward
	listener.Close()

	// Mark the port as allocated
	pa.mu.Lock()
	defer pa.mu.Unlock()
	pa.allocatedPorts[port] = true

	return port, nil
}

// reserveSpecificPort checks if a specific port is available and reserves it
func (pa *PortAllocator) reserveSpecificPort(port int32) error {
	pa.mu.Lock()
	defer pa.mu.Unlock()

	// Check if port is already allocated by this tool
	if pa.allocatedPorts[port] {
		return fmt.Errorf("port %d is already allocated", port)
	}

	// Try to bind to the port to check availability
	listener, err := net.Listen("tcp", fmt.Sprintf(":%d", port))
	if err != nil {
		return fmt.Errorf("port %d is not available: %w", port, err)
	}

	// Close the listener to release the port for use by port-forward
	listener.Close()

	// Mark the port as allocated
	pa.allocatedPorts[port] = true

	return nil
}

// IsPortAvailable checks if a port is available for use
func IsPortAvailable(port int32) bool {
	listener, err := net.Listen("tcp", fmt.Sprintf(":%d", port))
	if err != nil {
		return false
	}
	listener.Close()
	return true
}
