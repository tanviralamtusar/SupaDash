package provisioner

import (
	"fmt"
	"net"
	"sync"
)

// PortAllocation contains all ports assigned to a project
type PortAllocation struct {
	DBPort        int // PostgreSQL exposed port
	APIPort       int // Kong API Gateway HTTP port
	APIPortHTTPS  int // Kong API Gateway HTTPS port
	StudioPort    int // Studio UI port
	PoolerPort    int // PgBouncer pooler port
	AnalyticsPort int // Analytics/Logflare port
}

// PortAllocator manages port assignments across projects to avoid conflicts
type PortAllocator struct {
	mu sync.Mutex

	// Base ports (from config)
	baseDBPort  int
	baseAPIPort int

	// Track allocated ports
	allocatedPorts map[int]string // port -> projectID
}

// NewPortAllocator creates a new port allocator with configurable base ports
func NewPortAllocator(baseDBPort, baseAPIPort int) *PortAllocator {
	return &PortAllocator{
		baseDBPort:     baseDBPort,
		baseAPIPort:    baseAPIPort,
		allocatedPorts: make(map[int]string),
	}
}

// RegisterExistingPorts registers ports from existing projects (loaded from DB)
func (pa *PortAllocator) RegisterExistingPorts(projectID string, ports PortAllocation) {
	pa.mu.Lock()
	defer pa.mu.Unlock()

	for _, port := range []int{
		ports.DBPort, ports.APIPort, ports.APIPortHTTPS,
		ports.StudioPort, ports.PoolerPort, ports.AnalyticsPort,
	} {
		if port > 0 {
			pa.allocatedPorts[port] = projectID
		}
	}
}

// AllocatePorts assigns a unique set of ports for a new project
func (pa *PortAllocator) AllocatePorts(projectID string) (*PortAllocation, error) {
	pa.mu.Lock()
	defer pa.mu.Unlock()

	// Each project gets a block of 10 ports
	// We search for the next available block starting from base ports
	blockSize := 10

	// Find next available block for API ports (starting from baseAPIPort)
	apiBase, err := pa.findAvailableBlock(pa.baseAPIPort, blockSize)
	if err != nil {
		return nil, fmt.Errorf("failed to allocate API ports: %w", err)
	}

	// Find next available block for DB ports (starting from baseDBPort)
	dbBase, err := pa.findAvailableBlock(pa.baseDBPort, blockSize)
	if err != nil {
		return nil, fmt.Errorf("failed to allocate DB ports: %w", err)
	}

	allocation := &PortAllocation{
		DBPort:        dbBase,
		APIPort:       apiBase,
		APIPortHTTPS:  apiBase + 1,
		StudioPort:    apiBase + 2,
		PoolerPort:    dbBase + 1,
		AnalyticsPort: apiBase + 3,
	}

	// Register all ports
	pa.allocatedPorts[allocation.DBPort] = projectID
	pa.allocatedPorts[allocation.APIPort] = projectID
	pa.allocatedPorts[allocation.APIPortHTTPS] = projectID
	pa.allocatedPorts[allocation.StudioPort] = projectID
	pa.allocatedPorts[allocation.PoolerPort] = projectID
	pa.allocatedPorts[allocation.AnalyticsPort] = projectID

	return allocation, nil
}

// ReleasePorts frees all ports allocated to a project
func (pa *PortAllocator) ReleasePorts(projectID string) {
	pa.mu.Lock()
	defer pa.mu.Unlock()

	for port, pid := range pa.allocatedPorts {
		if pid == projectID {
			delete(pa.allocatedPorts, port)
		}
	}
}

// findAvailableBlock searches for a contiguous block of available ports
func (pa *PortAllocator) findAvailableBlock(startPort, blockSize int) (int, error) {
	maxPort := 65535

	for base := startPort; base < maxPort-blockSize; base += blockSize {
		available := true
		for offset := 0; offset < blockSize; offset++ {
			port := base + offset
			// Check if already allocated to a project
			if _, exists := pa.allocatedPorts[port]; exists {
				available = false
				break
			}
			// Check if port is in use on the host
			if !isPortAvailable(port) {
				available = false
				break
			}
		}
		if available {
			return base, nil
		}
	}

	return 0, fmt.Errorf("no available port block found starting from %d", startPort)
}

// isPortAvailable checks if a port is free on the host
func isPortAvailable(port int) bool {
	ln, err := net.Listen("tcp", fmt.Sprintf(":%d", port))
	if err != nil {
		return false
	}
	ln.Close()
	return true
}
