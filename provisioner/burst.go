package provisioner

import (
	"log/slog"
	"sync"
)

// BurstPoolManager manages a shared RAM pool across projects
// Uses Docker's memory reservation vs. limit model:
// - memory_reservation = guaranteed minimum RAM
// - memory_limit = maximum RAM (can use burst pool up to this)
// - burst pool = total_server_ram - sum(all reservations) - system_reserve
type BurstPoolManager struct {
	logger *slog.Logger
	mu     sync.RWMutex

	// Pool configuration
	totalPoolBytes  int64 // Total burst pool size
	systemReserve   int64 // Reserved for host OS

	// Per-project burst state
	projects        map[string]*BurstProjectState
}

// BurstProjectState tracks a project's burst usage
type BurstProjectState struct {
	ProjectID         string
	Reservation       int64   // Guaranteed RAM (memory_reservation)
	Limit             int64   // Max RAM (memory_limit)
	CurrentUsage      int64   // Current actual usage
	BurstUsage        int64   // Amount currently using from burst pool
	Priority          int     // Higher = gets burst first
	Eligible          bool    // Whether project can use burst
}

// BurstPoolStatus represents the current state of the burst pool
type BurstPoolStatus struct {
	TotalPoolMB     int64   `json:"total_pool_mb"`
	UsedPoolMB      int64   `json:"used_pool_mb"`
	FreePoolMB      int64   `json:"free_pool_mb"`
	UtilizationPct  float64 `json:"utilization_percent"`
	ActiveBursts    int     `json:"active_bursts"`
	EligibleCount   int     `json:"eligible_count"`
}

// NewBurstPoolManager creates a burst pool from available server resources
func NewBurstPoolManager(logger *slog.Logger, totalServerMemory int64) *BurstPoolManager {
	systemReserve := totalServerMemory / 10 // 10% for host OS
	poolSize := totalServerMemory - systemReserve

	bpm := &BurstPoolManager{
		logger:         logger,
		totalPoolBytes: poolSize,
		systemReserve:  systemReserve,
		projects:       make(map[string]*BurstProjectState),
	}

	logger.Info("Burst pool initialized",
		"totalPoolMB", poolSize/(1024*1024),
		"systemReserveMB", systemReserve/(1024*1024),
	)

	return bpm
}

// RegisterProject adds a project to the burst pool
func (bpm *BurstPoolManager) RegisterProject(projectID string, reservation, limit int64, priority int, eligible bool) {
	bpm.mu.Lock()
	defer bpm.mu.Unlock()

	bpm.projects[projectID] = &BurstProjectState{
		ProjectID:   projectID,
		Reservation: reservation,
		Limit:       limit,
		Priority:    priority,
		Eligible:    eligible,
	}
}

// UnregisterProject removes a project from the burst pool
func (bpm *BurstPoolManager) UnregisterProject(projectID string) {
	bpm.mu.Lock()
	defer bpm.mu.Unlock()
	delete(bpm.projects, projectID)
}

// UpdateUsage updates a project's current memory usage
func (bpm *BurstPoolManager) UpdateUsage(projectID string, currentUsage int64) {
	bpm.mu.Lock()
	defer bpm.mu.Unlock()

	state, ok := bpm.projects[projectID]
	if !ok {
		return
	}

	state.CurrentUsage = currentUsage

	// Calculate burst usage — any usage above reservation
	if currentUsage > state.Reservation {
		state.BurstUsage = currentUsage - state.Reservation
	} else {
		state.BurstUsage = 0
	}
}

// GetStatus returns the current burst pool status
func (bpm *BurstPoolManager) GetStatus() *BurstPoolStatus {
	bpm.mu.RLock()
	defer bpm.mu.RUnlock()

	var totalReservations int64
	var totalBurstUsage int64
	activeBursts := 0
	eligibleCount := 0

	for _, state := range bpm.projects {
		totalReservations += state.Reservation
		totalBurstUsage += state.BurstUsage
		if state.BurstUsage > 0 {
			activeBursts++
		}
		if state.Eligible {
			eligibleCount++
		}
	}

	availablePool := bpm.totalPoolBytes - totalReservations
	if availablePool < 0 {
		availablePool = 0
	}

	utilization := 0.0
	if availablePool > 0 {
		utilization = float64(totalBurstUsage) / float64(availablePool) * 100
	}

	return &BurstPoolStatus{
		TotalPoolMB:    availablePool / (1024 * 1024),
		UsedPoolMB:     totalBurstUsage / (1024 * 1024),
		FreePoolMB:     (availablePool - totalBurstUsage) / (1024 * 1024),
		UtilizationPct: utilization,
		ActiveBursts:   activeBursts,
		EligibleCount:  eligibleCount,
	}
}

// CanBurst checks if a project can use burst memory
func (bpm *BurstPoolManager) CanBurst(projectID string, additionalBytes int64) bool {
	bpm.mu.RLock()
	defer bpm.mu.RUnlock()

	state, ok := bpm.projects[projectID]
	if !ok || !state.Eligible {
		return false
	}

	// Check project hasn't exceeded its own limit
	if state.CurrentUsage+additionalBytes > state.Limit {
		return false
	}

	// Check pool has capacity
	var totalBurstUsage int64
	var totalReservations int64
	for _, s := range bpm.projects {
		totalBurstUsage += s.BurstUsage
		totalReservations += s.Reservation
	}

	availablePool := bpm.totalPoolBytes - totalReservations
	return totalBurstUsage+additionalBytes <= availablePool
}

// Rebalance adjusts burst allocations based on priority
// Higher priority projects get burst memory first when pool is constrained
func (bpm *BurstPoolManager) Rebalance() {
	bpm.mu.Lock()
	defer bpm.mu.Unlock()

	// Calculate available burst pool
	var totalReservations int64
	for _, state := range bpm.projects {
		totalReservations += state.Reservation
	}
	availablePool := bpm.totalPoolBytes - totalReservations
	if availablePool <= 0 {
		return
	}

	// Sort by priority (higher = more burst access)
	// For now, just log the rebalance
	bpm.logger.Debug("Burst pool rebalanced",
		"availablePoolMB", availablePool/(1024*1024),
		"projectCount", len(bpm.projects),
	)
}
