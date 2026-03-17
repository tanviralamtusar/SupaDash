package provisioner

import (
	"context"
	"fmt"
	"log/slog"
	"os/exec"
	"runtime"
	"strconv"
	"strings"
	"sync"
)

// ResourceManager handles resource limits and capacity for projects
type ResourceManager struct {
	logger      *slog.Logger
	provisioner *DockerProvisioner
	mu          sync.RWMutex

	// Server capacity
	totalCPU    float64
	totalMemory int64 // bytes

	// Track allocated resources
	allocated map[string]*ResourceAllocation // projectID -> allocation
}

// ResourceAllocation tracks what resources are allocated to a project
type ResourceAllocation struct {
	ProjectID         string
	Plan              string
	CPULimit          float64
	CPUReservation    float64
	MemoryLimit       int64
	MemoryReservation int64
}

// ServerCapacity represents total and available server resources
type ServerCapacity struct {
	TotalCPU       float64 `json:"total_cpu"`
	TotalMemoryMB  int64   `json:"total_memory_mb"`
	UsedCPU        float64 `json:"used_cpu"`
	UsedMemoryMB   int64   `json:"used_memory_mb"`
	FreeCPU        float64 `json:"free_cpu"`
	FreeMemoryMB   int64   `json:"free_memory_mb"`
	ProjectCount   int     `json:"project_count"`
	UtilizationPct float64 `json:"utilization_percent"`
}

// NewResourceManager creates a new resource manager
func NewResourceManager(logger *slog.Logger, provisioner *DockerProvisioner) *ResourceManager {
	totalCPU, totalMem := detectServerResources()

	rm := &ResourceManager{
		logger:      logger,
		provisioner: provisioner,
		totalCPU:    totalCPU,
		totalMemory: totalMem,
		allocated:   make(map[string]*ResourceAllocation),
	}

	logger.Info("Resource manager initialized",
		"totalCPU", totalCPU,
		"totalMemoryMB", totalMem/(1024*1024),
	)

	return rm
}

// SetProjectResources applies CPU/memory limits to a project's containers via docker update
func (rm *ResourceManager) SetProjectResources(ctx context.Context, projectID string, cpuLimit float64, memoryLimit int64) error {
	rm.mu.Lock()
	defer rm.mu.Unlock()

	// Capacity check
	currentUsed := rm.calculateUsedResources()
	newCPU := currentUsed.UsedCPU + cpuLimit
	newMem := currentUsed.UsedMemoryMB + memoryLimit/(1024*1024)

	if existing, ok := rm.allocated[projectID]; ok {
		newCPU -= existing.CPULimit
		newMem -= existing.MemoryLimit / (1024 * 1024)
	}

	if newCPU > rm.totalCPU*0.9 { // Reserve 10% for system
		return fmt.Errorf("insufficient CPU capacity: need %.2f, available %.2f", cpuLimit, rm.totalCPU*0.9-currentUsed.UsedCPU)
	}
	if newMem > rm.totalMemory/(1024*1024)*9/10 {
		return fmt.Errorf("insufficient memory capacity: need %dMB, available %dMB", memoryLimit/(1024*1024), rm.totalMemory/(1024*1024)*9/10-currentUsed.UsedMemoryMB)
	}

	// Apply limits via docker update on all containers in the project
	projectDir := rm.provisioner.getProjectDir(projectID)
	cpuQuota := int64(cpuLimit * 100000) // Docker CPU quota in microseconds
	memoryBytes := memoryLimit

	// Get list of containers
	psCmd := exec.CommandContext(ctx, "docker", "compose", "ps", "-q")
	psCmd.Dir = projectDir
	output, err := psCmd.Output()
	if err != nil {
		return fmt.Errorf("failed to list containers: %w", err)
	}

	containerIDs := strings.Split(strings.TrimSpace(string(output)), "\n")
	for _, containerID := range containerIDs {
		containerID = strings.TrimSpace(containerID)
		if containerID == "" {
			continue
		}

		updateCmd := exec.CommandContext(ctx, "docker", "update",
			"--cpus", fmt.Sprintf("%.2f", cpuLimit),
			"--cpu-quota", fmt.Sprintf("%d", cpuQuota),
			"--memory", fmt.Sprintf("%d", memoryBytes),
			containerID,
		)
		if err := updateCmd.Run(); err != nil {
			rm.logger.Warn("Failed to update container resources",
				"container", containerID,
				"error", err.Error(),
			)
		}
	}

	// Update internal tracking
	rm.allocated[projectID] = &ResourceAllocation{
		ProjectID:   projectID,
		CPULimit:    cpuLimit,
		MemoryLimit: memoryLimit,
	}

	rm.logger.Info("Resources updated",
		"projectID", projectID,
		"cpuLimit", cpuLimit,
		"memoryMB", memoryLimit/(1024*1024),
	)

	return nil
}

// GetServerCapacity returns the current server capacity overview
func (rm *ResourceManager) GetServerCapacity() *ServerCapacity {
	rm.mu.RLock()
	defer rm.mu.RUnlock()
	return rm.calculateUsedResources()
}

// RegisterProject registers a project's resource allocation (for tracking)
func (rm *ResourceManager) RegisterProject(projectID string, alloc *ResourceAllocation) {
	rm.mu.Lock()
	defer rm.mu.Unlock()
	rm.allocated[projectID] = alloc
}

// UnregisterProject removes a project's resource allocation
func (rm *ResourceManager) UnregisterProject(projectID string) {
	rm.mu.Lock()
	defer rm.mu.Unlock()
	delete(rm.allocated, projectID)
}

// calculateUsedResources sums up all allocated resources
func (rm *ResourceManager) calculateUsedResources() *ServerCapacity {
	var usedCPU float64
	var usedMem int64

	for _, alloc := range rm.allocated {
		usedCPU += alloc.CPULimit
		usedMem += alloc.MemoryLimit / (1024 * 1024)
	}

	totalMemMB := rm.totalMemory / (1024 * 1024)
	utilization := 0.0
	if totalMemMB > 0 {
		utilization = float64(usedMem) / float64(totalMemMB) * 100
	}

	return &ServerCapacity{
		TotalCPU:       rm.totalCPU,
		TotalMemoryMB:  totalMemMB,
		UsedCPU:        usedCPU,
		UsedMemoryMB:   usedMem,
		FreeCPU:        rm.totalCPU - usedCPU,
		FreeMemoryMB:   totalMemMB - usedMem,
		ProjectCount:   len(rm.allocated),
		UtilizationPct: utilization,
	}
}

// detectServerResources auto-detects server CPU and memory
func detectServerResources() (float64, int64) {
	cpuCount := float64(runtime.NumCPU())

	// Detect total memory
	var totalMemory int64
	if runtime.GOOS == "windows" {
		cmd := exec.Command("powershell", "-Command",
			"(Get-CimInstance Win32_ComputerSystem).TotalPhysicalMemory")
		output, err := cmd.Output()
		if err == nil {
			memStr := strings.TrimSpace(string(output))
			if mem, err := strconv.ParseInt(memStr, 10, 64); err == nil {
				totalMemory = mem
			}
		}
	} else {
		cmd := exec.Command("sh", "-c", "grep MemTotal /proc/meminfo | awk '{print $2}'")
		output, err := cmd.Output()
		if err == nil {
			memStr := strings.TrimSpace(string(output))
			if memKB, err := strconv.ParseInt(memStr, 10, 64); err == nil {
				totalMemory = memKB * 1024
			}
		}
	}

	// Fallback: assume 8GB if detection fails
	if totalMemory == 0 {
		totalMemory = 8 * 1024 * 1024 * 1024
	}

	return cpuCount, totalMemory
}
