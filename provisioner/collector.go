package provisioner

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"os/exec"
	"strconv"
	"strings"
	"time"

	"github.com/jackc/pgx/v5/pgtype"
	"supadash/database"
)

func floatToNumeric(f float64) pgtype.Numeric {
	var n pgtype.Numeric
	n.Scan(fmt.Sprintf("%f", f))
	return n
}

// StatsBroadcaster defines an interface for broadcasting real-time stats
type StatsBroadcaster interface {
	BroadcastStats(projectRef string, stats interface{})
}

// AnalysisCollector runs background goroutines to collect container stats
type AnalysisCollector struct {
	logger      *slog.Logger
	queries     *database.Queries
	provisioner *DockerProvisioner
	burstPool   *BurstPoolManager
	broadcaster StatsBroadcaster
}

// NewAnalysisCollector creates a new analysis collector
func NewAnalysisCollector(
	logger *slog.Logger,
	queries *database.Queries,
	provisioner *DockerProvisioner,
	burstPool *BurstPoolManager,
	broadcaster StatsBroadcaster,
) *AnalysisCollector {
	return &AnalysisCollector{
		logger:      logger,
		queries:     queries,
		provisioner: provisioner,
		burstPool:   burstPool,
		broadcaster: broadcaster,
	}
}

// Run starts the background collection loop with 4 tickers
func (ac *AnalysisCollector) Run(ctx context.Context) {
	snapshotTicker := time.NewTicker(30 * time.Second)
	diskUsageTicker := time.NewTicker(5 * time.Minute)
	aggregateTicker := time.NewTicker(1 * time.Hour)
	recommendTicker := time.NewTicker(6 * time.Hour)

	ac.logger.Info("Analysis collector started")

	for {
		select {
		case <-snapshotTicker.C:
			ac.collectSnapshots(ctx)
		case <-diskUsageTicker.C:
			ac.collectDiskUsage(ctx)
		case <-aggregateTicker.C:
			ac.aggregateHourly(ctx)
			ac.cleanupOldData(ctx)
		case <-recommendTicker.C:
			ac.generateRecommendations(ctx)
		case <-ctx.Done():
			ac.logger.Info("Analysis collector stopped")
			snapshotTicker.Stop()
			diskUsageTicker.Stop()
			aggregateTicker.Stop()
			recommendTicker.Stop()
			return
		}
	}
}

// DockerStats represents the JSON output from docker stats
type DockerStats struct {
	Name     string `json:"Name"`
	MemUsage string `json:"MemUsage"`
	MemPerc  string `json:"MemPerc"`
	CPUPerc  string `json:"CPUPerc"`
	NetIO    string `json:"NetIO"`
	BlockIO  string `json:"BlockIO"`
	PIDs     string `json:"PIDs"`
}

// collectSnapshots collects stats from all running project containers
func (ac *AnalysisCollector) collectSnapshots(ctx context.Context) {
	projects, err := ac.provisioner.ListProjects(ctx)
	if err != nil {
		ac.logger.Warn("Failed to list projects for snapshot collection", "error", err.Error())
		return
	}

	for _, project := range projects {
		if project.Status != StatusActive {
			continue
		}

		ac.collectProjectSnapshots(ctx, project.ProjectID)
	}
}

// collectProjectSnapshots collects stats for a single project
func (ac *AnalysisCollector) collectProjectSnapshots(ctx context.Context, projectID string) {
	projectDir := ac.provisioner.getProjectDir(projectID)

	// Use docker compose stats with JSON format
	statsCmd := exec.CommandContext(ctx, "docker", "compose", "ps", "--format", "{{.Name}}")
	statsCmd.Dir = projectDir
	output, err := statsCmd.Output()
	if err != nil {
		return
	}

	containerNames := strings.Split(strings.TrimSpace(string(output)), "\n")
	for _, name := range containerNames {
		name = strings.TrimSpace(name)
		if name == "" {
			continue
		}

		ac.collectContainerStats(ctx, projectID, name)
	}
}

// collectContainerStats collects stats for a single container
func (ac *AnalysisCollector) collectContainerStats(ctx context.Context, projectID, containerName string) {
	// Get stats using docker stats --no-stream --format json
	statsCmd := exec.CommandContext(ctx, "docker", "stats", containerName, "--no-stream",
		"--format", `{"Name":"{{.Name}}","MemUsage":"{{.MemUsage}}","MemPerc":"{{.MemPerc}}","CPUPerc":"{{.CPUPerc}}","NetIO":"{{.NetIO}}","BlockIO":"{{.BlockIO}}"}`)
	output, err := statsCmd.Output()
	if err != nil {
		return
	}

	var stats DockerStats
	if err := json.Unmarshal(output, &stats); err != nil {
		return
	}

	// Parse memory usage
	memUsage, memLimit := parseMemUsage(stats.MemUsage)
	cpuPercent := parsePercent(stats.CPUPerc)
	netRx, netTx := parseIOPair(stats.NetIO)
	diskRead, diskWrite := parseIOPair(stats.BlockIO)

	// Extract service name from container name (projectID-serviceName-1)
	serviceName := extractServiceName(containerName, projectID)

	// Check for OOM kills
	oomKilled := false
	inspectCmd := exec.CommandContext(ctx, "docker", "inspect", containerName,
		"--format", "{{.State.OOMKilled}}")
	if inspectOutput, err := inspectCmd.Output(); err == nil {
		oomKilled = strings.TrimSpace(string(inspectOutput)) == "true"
	}

	// Get restart count
	var restartCount int32
	restartCmd := exec.CommandContext(ctx, "docker", "inspect", containerName,
		"--format", "{{.RestartCount}}")
	if restartOutput, err := restartCmd.Output(); err == nil {
		if rc, err := strconv.Atoi(strings.TrimSpace(string(restartOutput))); err == nil {
			restartCount = int32(rc)
		}
	}

	// Insert snapshot into DB
	if err := ac.queries.InsertResourceSnapshot(ctx, database.InsertResourceSnapshotParams{
		ProjectRef:       projectID,
		ServiceName:      serviceName,
		MemoryUsageBytes: memUsage,
		MemoryLimitBytes: memLimit,
		CPUUsagePercent:  cpuPercent,
		CPULimitCores:    0, // Filled from project_resources
		DiskReadBytes:    diskRead,
		DiskWriteBytes:   diskWrite,
		NetworkRxBytes:   netRx,
		NetworkTxBytes:   netTx,
		ContainerStatus:  "running",
		RestartCount:     restartCount,
		OOMKilled:        oomKilled,
	}); err != nil {
		ac.logger.Warn("Failed to insert snapshot", "project", projectID, "container", containerName, "error", err.Error())
	}

	// Update burst pool usage
	if ac.burstPool != nil {
		ac.burstPool.UpdateUsage(projectID, memUsage)
	}

	// Broadcast stats in real-time
	if ac.broadcaster != nil {
		ac.broadcaster.BroadcastStats(projectID, map[string]interface{}{
			"service_name":   serviceName,
			"cpu_usage":      cpuPercent,
			"memory_usage":   memUsage,
			"memory_limit":   memLimit,
			"network_rx":     netRx,
			"network_tx":     netTx,
			"disk_read":      diskRead,
			"disk_write":     diskWrite,
			"container_status": "running",
			"timestamp":      time.Now().Unix(),
		})
	}
}

// aggregateHourly rolls up raw snapshots into hourly summaries
func (ac *AnalysisCollector) aggregateHourly(ctx context.Context) {
	ac.logger.Info("Running hourly aggregation")
	// Aggregate the last hour's raw snapshots
	hourAgo := time.Now().Add(-1 * time.Hour)
	now := time.Now().Truncate(time.Hour)

	projects, err := ac.provisioner.ListProjects(ctx)
	if err != nil {
		return
	}

	for _, project := range projects {
		snapshots, err := ac.queries.GetRecentSnapshots(ctx, project.ProjectID, hourAgo)
		if err != nil || len(snapshots) == 0 {
			continue
		}

		// Group by service name
		serviceSnapshots := make(map[string][]database.ResourceSnapshot)
		for _, s := range snapshots {
			serviceSnapshots[s.ServiceName] = append(serviceSnapshots[s.ServiceName], s)
		}

		for serviceName, snaps := range serviceSnapshots {
			var totalMem, maxMem, totalDiskR, totalDiskW, totalNetRx, totalNetTx int64
			var totalCPU, maxCPU float64
			var oomCount, restartCount int32

			for _, s := range snaps {
				totalMem += s.MemoryUsageBytes.Int64
				if s.MemoryUsageBytes.Int64 > maxMem {
					maxMem = s.MemoryUsageBytes.Int64
				}
				cpuPerc, _ := s.CpuUsagePercent.Float64Value()
				totalCPU += cpuPerc.Float64
				if cpuPerc.Float64 > maxCPU {
					maxCPU = cpuPerc.Float64
				}
				totalDiskR += s.DiskReadBytes.Int64
				totalDiskW += s.DiskWriteBytes.Int64
				totalNetRx += s.NetworkRxBytes.Int64
				totalNetTx += s.NetworkTxBytes.Int64
				if s.OomKilled.Bool {
					oomCount++
				}
				restartCount += s.RestartCount.Int32
			}

			count := int64(len(snaps))
			ac.queries.UpsertHourlySnapshot(ctx, database.UpsertHourlySnapshotParams{
				ProjectRef:          project.ProjectID,
				ServiceName:         serviceName,
				Hour:                now,
				AvgMemoryUsageBytes: totalMem / count,
				MaxMemoryUsageBytes: maxMem,
				AvgCPUPercent:       totalCPU / float64(count),
				MaxCPUPercent:       maxCPU,
				TotalDiskReadBytes:  totalDiskR,
				TotalDiskWriteBytes: totalDiskW,
				TotalNetworkRxBytes: totalNetRx,
				TotalNetworkTxBytes: totalNetTx,
				OOMKillCount:        oomCount,
				RestartCount:        restartCount,
			})
		}
	}
}

// cleanupOldData deletes raw snapshots older than 24 hours
func (ac *AnalysisCollector) cleanupOldData(ctx context.Context) {
	cutoff := time.Now().Add(-24 * time.Hour)
	if err := ac.queries.DeleteOldSnapshots(ctx, cutoff); err != nil {
		ac.logger.Warn("Failed to cleanup old snapshots", "error", err.Error())
	}
}

// generateRecommendations analyzes data and creates optimization suggestions
func (ac *AnalysisCollector) generateRecommendations(ctx context.Context) {
	ac.logger.Info("Generating resource recommendations")

	projects, err := ac.provisioner.ListProjects(ctx)
	if err != nil {
		return
	}

	for _, project := range projects {
		ac.analyzeProject(ctx, project.ProjectID)
	}
}

// analyzeProject generates recommendations for a single project
func (ac *AnalysisCollector) analyzeProject(ctx context.Context, projectID string) {
	hourlyData, err := ac.queries.GetHourlySnapshots(ctx, projectID, time.Now().Add(-24*time.Hour))
	if err != nil || len(hourlyData) == 0 {
		return
	}

	// Anomaly 1: OOM kills detected
	var totalOOM int32
	for _, h := range hourlyData {
		totalOOM += h.OOMKillCount
	}
	if totalOOM > 0 {
		ac.queries.InsertRecommendation(ctx, database.InsertRecommendationParams{
			ProjectRef:        projectID,
			Type:              "alert",
			Severity:          "critical",
			Title:             "OOM kills detected",
			Description:       fmt.Sprintf("%d out-of-memory kills in the last 24 hours. Consider increasing memory limits or optimizing queries.", totalOOM),
			PotentialSavingsMB: 0,
		})
	}

	// Anomaly 2: Low memory utilization → can downsize
	var avgMemPercent float64
	var maxMemUsage int64
	var samples int
	for _, h := range hourlyData {
		if h.MaxMemoryUsageBytes > 0 {
			avgMemPercent += float64(h.AvgMemoryUsageBytes) / float64(h.MaxMemoryUsageBytes) * 100
			if h.MaxMemoryUsageBytes > maxMemUsage {
				maxMemUsage = h.MaxMemoryUsageBytes
			}
			samples++
		}
	}
	if samples > 0 {
		avgMemPercent /= float64(samples)
		if avgMemPercent < 30 && maxMemUsage > 0 {
			savingsMB := int32((float64(maxMemUsage) * 0.5) / (1024 * 1024))
			ac.queries.InsertRecommendation(ctx, database.InsertRecommendationParams{
				ProjectRef:        projectID,
				Type:              "cost_saving",
				Severity:          "info",
				Title:             "Memory over-provisioned",
				Description:       fmt.Sprintf("Average memory utilization is only %.0f%%. You could safely reduce the memory limit to save resources.", avgMemPercent),
				PotentialSavingsMB: savingsMB,
			})
		}
	}

	// Anomaly 3: High CPU usage → consider upgrading
	var maxCPU float64
	for _, h := range hourlyData {
		if h.MaxCPUPercent > maxCPU {
			maxCPU = h.MaxCPUPercent
		}
	}
	if maxCPU > 85 {
		ac.queries.InsertRecommendation(ctx, database.InsertRecommendationParams{
			ProjectRef:        projectID,
			Type:              "performance",
			Severity:          "warning",
			Title:             "High CPU utilization",
			Description:       fmt.Sprintf("Peak CPU usage reached %.0f%%. Consider upgrading to a higher tier plan for better performance.", maxCPU),
			PotentialSavingsMB: 0,
		})
	}
}

// --- Disk usage quota enforcement ---

// collectDiskUsage measures DB and file-storage usage for all active projects,
// records it, and toggles the soft write-block (read-only mode) when a project
// crosses 100% of its size limits. Warnings are emitted when crossing 80%.
func (ac *AnalysisCollector) collectDiskUsage(ctx context.Context) {
	projects, err := ac.provisioner.ListProjects(ctx)
	if err != nil {
		ac.logger.Warn("Failed to list projects for disk usage collection", "error", err.Error())
		return
	}

	for _, project := range projects {
		if project.Status != StatusActive {
			continue
		}
		ac.checkProjectDiskUsage(ctx, project.ProjectID)
	}
}

// checkProjectDiskUsage measures and enforces disk quotas for a single project.
func (ac *AnalysisCollector) checkProjectDiskUsage(ctx context.Context, projectID string) {
	res, err := ac.queries.GetProjectResources(ctx, projectID)
	if err != nil {
		// No resource plan recorded — nothing to enforce
		return
	}

	dbSize, dbErr := ac.measureDatabaseSize(ctx, projectID)
	storageSize, stErr := ac.measureStorageSize(ctx, projectID)
	if dbErr != nil && stErr != nil {
		// Both measurements failed (containers restarting?) — skip this round
		return
	}
	// Keep previous readings for any failed measurement
	if dbErr != nil {
		dbSize = res.DatabaseSizeBytes
	}
	if stErr != nil {
		storageSize = res.StorageSizeBytes
	}

	// A limit of 0 means unlimited
	overDB := res.DatabaseSizeLimitBytes > 0 && dbSize >= res.DatabaseSizeLimitBytes
	overStorage := res.StorageSizeLimitBytes > 0 && storageSize >= res.StorageSizeLimitBytes
	shouldBlock := overDB || overStorage

	// Warning threshold crossings (80%), deduped against the previous reading
	warnPct := 0.8
	newWarnDB := res.DatabaseSizeLimitBytes > 0 &&
		float64(dbSize) >= warnPct*float64(res.DatabaseSizeLimitBytes) &&
		float64(res.DatabaseSizeBytes) < warnPct*float64(res.DatabaseSizeLimitBytes)
	newWarnStorage := res.StorageSizeLimitBytes > 0 &&
		float64(storageSize) >= warnPct*float64(res.StorageSizeLimitBytes) &&
		float64(res.StorageSizeBytes) < warnPct*float64(res.StorageSizeLimitBytes)

	if newWarnDB {
		ac.queries.InsertRecommendation(ctx, database.InsertRecommendationParams{
			ProjectRef:  projectID,
			Type:        "alert",
			Severity:    "warning",
			Title:       "Database size approaching limit",
			Description: fmt.Sprintf("Database is using %d MB of its %d MB limit (over 80%%). Free up space or increase the limit to avoid the project entering read-only mode.", dbSize/(1024*1024), res.DatabaseSizeLimitBytes/(1024*1024)),
		})
	}
	if newWarnStorage {
		ac.queries.InsertRecommendation(ctx, database.InsertRecommendationParams{
			ProjectRef:  projectID,
			Type:        "alert",
			Severity:    "warning",
			Title:       "File storage approaching limit",
			Description: fmt.Sprintf("Storage is using %d MB of its %d MB limit (over 80%%). Free up space or increase the limit to avoid the project entering read-only mode.", storageSize/(1024*1024), res.StorageSizeLimitBytes/(1024*1024)),
		})
	}

	// Toggle the soft block only on state change
	if shouldBlock != res.WritesBlocked {
		if err := ac.setReadOnlyMode(ctx, projectID, shouldBlock); err != nil {
			ac.logger.Warn("Failed to toggle read-only mode", "project", projectID, "block", shouldBlock, "error", err.Error())
		} else if shouldBlock {
			ac.logger.Info("Project entered read-only mode (disk quota exceeded)", "project", projectID)
			ac.queries.InsertRecommendation(ctx, database.InsertRecommendationParams{
				ProjectRef:  projectID,
				Type:        "alert",
				Severity:    "critical",
				Title:       "Disk quota exceeded — project is read-only",
				Description: fmt.Sprintf("Usage exceeded the configured limits (DB: %d/%d MB, storage: %d/%d MB). New writes are blocked until space is freed or limits are raised.", dbSize/(1024*1024), res.DatabaseSizeLimitBytes/(1024*1024), storageSize/(1024*1024), res.StorageSizeLimitBytes/(1024*1024)),
			})
		} else {
			ac.logger.Info("Project left read-only mode (disk usage back under limits)", "project", projectID)
		}
	}

	if err := ac.queries.UpdateProjectResourceUsage(ctx, database.UpdateProjectResourceUsageParams{
		ProjectRef:        projectID,
		DatabaseSizeBytes: dbSize,
		StorageSizeBytes:  storageSize,
		WritesBlocked:     shouldBlock,
	}); err != nil {
		ac.logger.Warn("Failed to record disk usage", "project", projectID, "error", err.Error())
	}

	// Broadcast usage for live dashboards
	if ac.broadcaster != nil {
		ac.broadcaster.BroadcastStats(projectID, map[string]interface{}{
			"database_size_bytes":  dbSize,
			"storage_size_bytes":   storageSize,
			"database_limit_bytes": res.DatabaseSizeLimitBytes,
			"storage_limit_bytes":  res.StorageSizeLimitBytes,
			"writes_blocked":       shouldBlock,
			"timestamp":            time.Now().Unix(),
		})
	}
}

// measureDatabaseSize returns the size of the project's postgres database in bytes.
func (ac *AnalysisCollector) measureDatabaseSize(ctx context.Context, projectID string) (int64, error) {
	out, err := ac.provisioner.ExecuteCommand(ctx, projectID, "db",
		[]string{"psql", "-U", "postgres", "-tAc", "SELECT pg_database_size('postgres')"})
	if err != nil {
		return 0, err
	}
	size, err := strconv.ParseInt(strings.TrimSpace(out), 10, 64)
	if err != nil {
		return 0, fmt.Errorf("unexpected pg_database_size output %q: %w", out, err)
	}
	return size, nil
}

// measureStorageSize returns the bytes used by the project's object storage (MinIO data dir).
func (ac *AnalysisCollector) measureStorageSize(ctx context.Context, projectID string) (int64, error) {
	out, err := ac.provisioner.ExecuteCommand(ctx, projectID, "minio", []string{"du", "-sk", "/data"})
	if err != nil {
		return 0, err
	}
	fields := strings.Fields(strings.TrimSpace(out))
	if len(fields) == 0 {
		return 0, fmt.Errorf("unexpected du output %q", out)
	}
	kb, err := strconv.ParseInt(fields[0], 10, 64)
	if err != nil {
		return 0, fmt.Errorf("unexpected du output %q: %w", out, err)
	}
	return kb * 1024, nil
}

// setReadOnlyMode soft-blocks (or unblocks) writes by toggling
// default_transaction_read_only on the project's postgres database.
// This is a soft limit: superusers and sessions that explicitly override
// the setting can still write (so users can free up space).
func (ac *AnalysisCollector) setReadOnlyMode(ctx context.Context, projectID string, readOnly bool) error {
	mode := "off"
	if readOnly {
		mode = "on"
	}
	_, err := ac.provisioner.ExecuteCommand(ctx, projectID, "db",
		[]string{"psql", "-U", "postgres", "-c",
			fmt.Sprintf("ALTER DATABASE postgres SET default_transaction_read_only = %s", mode)})
	return err
}

// --- Parsing helpers ---

func parseMemUsage(s string) (usage, limit int64) {
	// Format: "123.4MiB / 512MiB"
	parts := strings.Split(s, " / ")
	if len(parts) != 2 {
		return 0, 0
	}
	return parseByteString(parts[0]), parseByteString(parts[1])
}

func parseByteString(s string) int64 {
	s = strings.TrimSpace(s)
	multiplier := int64(1)
	if strings.HasSuffix(s, "GiB") {
		multiplier = 1024 * 1024 * 1024
		s = strings.TrimSuffix(s, "GiB")
	} else if strings.HasSuffix(s, "MiB") {
		multiplier = 1024 * 1024
		s = strings.TrimSuffix(s, "MiB")
	} else if strings.HasSuffix(s, "KiB") {
		multiplier = 1024
		s = strings.TrimSuffix(s, "KiB")
	} else if strings.HasSuffix(s, "B") {
		s = strings.TrimSuffix(s, "B")
	}
	val, err := strconv.ParseFloat(strings.TrimSpace(s), 64)
	if err != nil {
		return 0
	}
	return int64(val * float64(multiplier))
}

func parsePercent(s string) float64 {
	s = strings.TrimSuffix(strings.TrimSpace(s), "%")
	val, _ := strconv.ParseFloat(s, 64)
	return val
}

func parseIOPair(s string) (int64, int64) {
	parts := strings.Split(s, " / ")
	if len(parts) != 2 {
		return 0, 0
	}
	return parseByteString(parts[0]), parseByteString(parts[1])
}

func extractServiceName(containerName, projectID string) string {
	// Container names are typically: projectID-serviceName-1
	name := strings.TrimPrefix(containerName, projectID+"-")
	name = strings.TrimSuffix(name, "-1")
	if name == containerName {
		return containerName
	}
	return name
}
