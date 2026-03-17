package database

import (
	"context"
	"time"

	"github.com/jackc/pgx/v5/pgtype"
)

// --- Project Resources ---

type ProjectResource struct {
	ID                 int32
	ProjectRef         string
	Plan               string
	CPULimit           float64
	CPUReservation     float64
	MemoryLimit        int64
	MemoryReservation  int64
	BurstEligible      bool
	BurstPriority      int32
	CreatedAt          pgtype.Timestamptz
	UpdatedAt          pgtype.Timestamptz
}

const getProjectResources = `
SELECT id, project_ref, plan, cpu_limit, cpu_reservation, memory_limit, memory_reservation,
       burst_eligible, burst_priority, created_at, updated_at
FROM project_resources WHERE project_ref = $1
`

func (q *Queries) GetProjectResources(ctx context.Context, projectRef string) (ProjectResource, error) {
	row := q.db.QueryRow(ctx, getProjectResources, projectRef)
	var r ProjectResource
	err := row.Scan(
		&r.ID, &r.ProjectRef, &r.Plan, &r.CPULimit, &r.CPUReservation,
		&r.MemoryLimit, &r.MemoryReservation, &r.BurstEligible, &r.BurstPriority,
		&r.CreatedAt, &r.UpdatedAt,
	)
	return r, err
}

const upsertProjectResources = `
INSERT INTO project_resources (project_ref, plan, cpu_limit, cpu_reservation, memory_limit, memory_reservation, burst_eligible, burst_priority, updated_at)
VALUES ($1, $2, $3, $4, $5, $6, $7, $8, now())
ON CONFLICT (project_ref) DO UPDATE
SET plan = $2, cpu_limit = $3, cpu_reservation = $4, memory_limit = $5,
    memory_reservation = $6, burst_eligible = $7, burst_priority = $8, updated_at = now()
RETURNING *
`

type UpsertProjectResourcesParams struct {
	ProjectRef        string
	Plan              string
	CPULimit          float64
	CPUReservation    float64
	MemoryLimit       int64
	MemoryReservation int64
	BurstEligible     bool
	BurstPriority     int32
}

func (q *Queries) UpsertProjectResources(ctx context.Context, arg UpsertProjectResourcesParams) (ProjectResource, error) {
	row := q.db.QueryRow(ctx, upsertProjectResources,
		arg.ProjectRef, arg.Plan, arg.CPULimit, arg.CPUReservation,
		arg.MemoryLimit, arg.MemoryReservation, arg.BurstEligible, arg.BurstPriority,
	)
	var r ProjectResource
	err := row.Scan(
		&r.ID, &r.ProjectRef, &r.Plan, &r.CPULimit, &r.CPUReservation,
		&r.MemoryLimit, &r.MemoryReservation, &r.BurstEligible, &r.BurstPriority,
		&r.CreatedAt, &r.UpdatedAt,
	)
	return r, err
}

const getAllProjectResources = `
SELECT id, project_ref, plan, cpu_limit, cpu_reservation, memory_limit, memory_reservation,
       burst_eligible, burst_priority, created_at, updated_at
FROM project_resources ORDER BY project_ref
`

func (q *Queries) GetAllProjectResources(ctx context.Context) ([]ProjectResource, error) {
	rows, err := q.db.Query(ctx, getAllProjectResources)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var items []ProjectResource
	for rows.Next() {
		var r ProjectResource
		if err := rows.Scan(
			&r.ID, &r.ProjectRef, &r.Plan, &r.CPULimit, &r.CPUReservation,
			&r.MemoryLimit, &r.MemoryReservation, &r.BurstEligible, &r.BurstPriority,
			&r.CreatedAt, &r.UpdatedAt,
		); err != nil {
			return nil, err
		}
		items = append(items, r)
	}
	return items, rows.Err()
}

// --- Resource Snapshots ---

type ResourceSnapshot struct {
	ID               int64
	ProjectRef       string
	ServiceName      string
	MemoryUsageBytes int64
	MemoryLimitBytes int64
	CPUUsagePercent  float64
	CPULimitCores    float64
	DiskReadBytes    int64
	DiskWriteBytes   int64
	NetworkRxBytes   int64
	NetworkTxBytes   int64
	ContainerStatus  string
	RestartCount     int32
	OOMKilled        bool
	RecordedAt       time.Time
}

const insertResourceSnapshot = `
INSERT INTO resource_snapshots (project_ref, service_name, memory_usage_bytes, memory_limit_bytes,
    cpu_usage_percent, cpu_limit_cores, disk_read_bytes, disk_write_bytes,
    network_rx_bytes, network_tx_bytes, container_status, restart_count, oom_killed, recorded_at)
VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12, $13, now())
`

type InsertResourceSnapshotParams struct {
	ProjectRef       string
	ServiceName      string
	MemoryUsageBytes int64
	MemoryLimitBytes int64
	CPUUsagePercent  float64
	CPULimitCores    float64
	DiskReadBytes    int64
	DiskWriteBytes   int64
	NetworkRxBytes   int64
	NetworkTxBytes   int64
	ContainerStatus  string
	RestartCount     int32
	OOMKilled        bool
}

func (q *Queries) InsertResourceSnapshot(ctx context.Context, arg InsertResourceSnapshotParams) error {
	_, err := q.db.Exec(ctx, insertResourceSnapshot,
		arg.ProjectRef, arg.ServiceName, arg.MemoryUsageBytes, arg.MemoryLimitBytes,
		arg.CPUUsagePercent, arg.CPULimitCores, arg.DiskReadBytes, arg.DiskWriteBytes,
		arg.NetworkRxBytes, arg.NetworkTxBytes, arg.ContainerStatus, arg.RestartCount, arg.OOMKilled,
	)
	return err
}

const getRecentSnapshots = `
SELECT id, project_ref, service_name, memory_usage_bytes, memory_limit_bytes,
       cpu_usage_percent, cpu_limit_cores, disk_read_bytes, disk_write_bytes,
       network_rx_bytes, network_tx_bytes, container_status, restart_count, oom_killed, recorded_at
FROM resource_snapshots
WHERE project_ref = $1 AND recorded_at > $2
ORDER BY recorded_at DESC
`

func (q *Queries) GetRecentSnapshots(ctx context.Context, projectRef string, since time.Time) ([]ResourceSnapshot, error) {
	rows, err := q.db.Query(ctx, getRecentSnapshots, projectRef, since)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var items []ResourceSnapshot
	for rows.Next() {
		var s ResourceSnapshot
		if err := rows.Scan(
			&s.ID, &s.ProjectRef, &s.ServiceName, &s.MemoryUsageBytes, &s.MemoryLimitBytes,
			&s.CPUUsagePercent, &s.CPULimitCores, &s.DiskReadBytes, &s.DiskWriteBytes,
			&s.NetworkRxBytes, &s.NetworkTxBytes, &s.ContainerStatus, &s.RestartCount, &s.OOMKilled, &s.RecordedAt,
		); err != nil {
			return nil, err
		}
		items = append(items, s)
	}
	return items, rows.Err()
}

const deleteOldSnapshots = `DELETE FROM resource_snapshots WHERE recorded_at < $1`

func (q *Queries) DeleteOldSnapshots(ctx context.Context, before time.Time) error {
	_, err := q.db.Exec(ctx, deleteOldSnapshots, before)
	return err
}

// --- Hourly Aggregation ---

type ResourceSnapshotHourly struct {
	ID                    int64
	ProjectRef            string
	ServiceName           string
	Hour                  time.Time
	AvgMemoryUsageBytes   int64
	MaxMemoryUsageBytes   int64
	AvgCPUPercent         float64
	MaxCPUPercent         float64
	TotalDiskReadBytes    int64
	TotalDiskWriteBytes   int64
	TotalNetworkRxBytes   int64
	TotalNetworkTxBytes   int64
	BurstPoolUsageBytes   int64
	BurstPoolDurationSec  int32
	OOMKillCount          int32
	RestartCount          int32
}

const upsertHourlySnapshot = `
INSERT INTO resource_snapshots_hourly (project_ref, service_name, hour,
    avg_memory_usage_bytes, max_memory_usage_bytes, avg_cpu_percent, max_cpu_percent,
    total_disk_read_bytes, total_disk_write_bytes, total_network_rx_bytes, total_network_tx_bytes,
    burst_pool_usage_bytes, burst_pool_duration_sec, oom_kill_count, restart_count)
VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12, $13, $14, $15)
ON CONFLICT (project_ref, service_name, hour) DO UPDATE
SET avg_memory_usage_bytes = $4, max_memory_usage_bytes = $5,
    avg_cpu_percent = $6, max_cpu_percent = $7,
    total_disk_read_bytes = $8, total_disk_write_bytes = $9,
    total_network_rx_bytes = $10, total_network_tx_bytes = $11,
    burst_pool_usage_bytes = $12, burst_pool_duration_sec = $13,
    oom_kill_count = $14, restart_count = $15
`

type UpsertHourlySnapshotParams struct {
	ProjectRef            string
	ServiceName           string
	Hour                  time.Time
	AvgMemoryUsageBytes   int64
	MaxMemoryUsageBytes   int64
	AvgCPUPercent         float64
	MaxCPUPercent         float64
	TotalDiskReadBytes    int64
	TotalDiskWriteBytes   int64
	TotalNetworkRxBytes   int64
	TotalNetworkTxBytes   int64
	BurstPoolUsageBytes   int64
	BurstPoolDurationSec  int32
	OOMKillCount          int32
	RestartCount          int32
}

func (q *Queries) UpsertHourlySnapshot(ctx context.Context, arg UpsertHourlySnapshotParams) error {
	_, err := q.db.Exec(ctx, upsertHourlySnapshot,
		arg.ProjectRef, arg.ServiceName, arg.Hour,
		arg.AvgMemoryUsageBytes, arg.MaxMemoryUsageBytes,
		arg.AvgCPUPercent, arg.MaxCPUPercent,
		arg.TotalDiskReadBytes, arg.TotalDiskWriteBytes,
		arg.TotalNetworkRxBytes, arg.TotalNetworkTxBytes,
		arg.BurstPoolUsageBytes, arg.BurstPoolDurationSec,
		arg.OOMKillCount, arg.RestartCount,
	)
	return err
}

const getHourlySnapshots = `
SELECT id, project_ref, service_name, hour,
       avg_memory_usage_bytes, max_memory_usage_bytes, avg_cpu_percent, max_cpu_percent,
       total_disk_read_bytes, total_disk_write_bytes, total_network_rx_bytes, total_network_tx_bytes,
       burst_pool_usage_bytes, burst_pool_duration_sec, oom_kill_count, restart_count
FROM resource_snapshots_hourly
WHERE project_ref = $1 AND hour > $2
ORDER BY hour DESC
`

func (q *Queries) GetHourlySnapshots(ctx context.Context, projectRef string, since time.Time) ([]ResourceSnapshotHourly, error) {
	rows, err := q.db.Query(ctx, getHourlySnapshots, projectRef, since)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var items []ResourceSnapshotHourly
	for rows.Next() {
		var h ResourceSnapshotHourly
		if err := rows.Scan(
			&h.ID, &h.ProjectRef, &h.ServiceName, &h.Hour,
			&h.AvgMemoryUsageBytes, &h.MaxMemoryUsageBytes, &h.AvgCPUPercent, &h.MaxCPUPercent,
			&h.TotalDiskReadBytes, &h.TotalDiskWriteBytes, &h.TotalNetworkRxBytes, &h.TotalNetworkTxBytes,
			&h.BurstPoolUsageBytes, &h.BurstPoolDurationSec, &h.OOMKillCount, &h.RestartCount,
		); err != nil {
			return nil, err
		}
		items = append(items, h)
	}
	return items, rows.Err()
}

// --- Recommendations ---

type ResourceRecommendation struct {
	ID                int32
	ProjectRef        string
	Type              string
	Severity          string
	Title             string
	Description       string
	PotentialSavingsMB int32
	IsDismissed       bool
	CreatedAt         pgtype.Timestamptz
	DismissedAt       pgtype.Timestamptz
}

const getActiveRecommendations = `
SELECT id, project_ref, type, severity, title, description, potential_savings_mb,
       is_dismissed, created_at, dismissed_at
FROM resource_recommendations
WHERE project_ref = $1 AND is_dismissed = false
ORDER BY created_at DESC
`

func (q *Queries) GetActiveRecommendations(ctx context.Context, projectRef string) ([]ResourceRecommendation, error) {
	rows, err := q.db.Query(ctx, getActiveRecommendations, projectRef)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var items []ResourceRecommendation
	for rows.Next() {
		var r ResourceRecommendation
		if err := rows.Scan(
			&r.ID, &r.ProjectRef, &r.Type, &r.Severity, &r.Title, &r.Description,
			&r.PotentialSavingsMB, &r.IsDismissed, &r.CreatedAt, &r.DismissedAt,
		); err != nil {
			return nil, err
		}
		items = append(items, r)
	}
	return items, rows.Err()
}

const insertRecommendation = `
INSERT INTO resource_recommendations (project_ref, type, severity, title, description, potential_savings_mb)
VALUES ($1, $2, $3, $4, $5, $6)
`

type InsertRecommendationParams struct {
	ProjectRef        string
	Type              string
	Severity          string
	Title             string
	Description       string
	PotentialSavingsMB int32
}

func (q *Queries) InsertRecommendation(ctx context.Context, arg InsertRecommendationParams) error {
	_, err := q.db.Exec(ctx, insertRecommendation,
		arg.ProjectRef, arg.Type, arg.Severity, arg.Title, arg.Description, arg.PotentialSavingsMB,
	)
	return err
}

const dismissRecommendation = `
UPDATE resource_recommendations SET is_dismissed = true, dismissed_at = now() WHERE id = $1
`

func (q *Queries) DismissRecommendation(ctx context.Context, id int32) error {
	_, err := q.db.Exec(ctx, dismissRecommendation, id)
	return err
}
