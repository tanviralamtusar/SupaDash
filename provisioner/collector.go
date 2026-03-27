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

// Run starts the background collection loop with 3 tickers
func (ac *AnalysisCollector) Run(ctx context.Context) {
	snapshotTicker := time.NewTicker(30 * time.Second)
	aggregateTicker := time.NewTicker(1 * time.Hour)
	recommendTicker := time.NewTicker(6 * time.Hour)

	ac.logger.Info("Analysis collector started")

	for {
		select {
		case <-snapshotTicker.C:
			ac.collectSnapshots(ctx)
		case <-aggregateTicker.C:
			ac.aggregateHourly(ctx)
			ac.cleanupOldData(ctx)
		case <-recommendTicker.C:
			ac.generateRecommendations(ctx)
		case <-ctx.Done():
			ac.logger.Info("Analysis collector stopped")
			snapshotTicker.Stop()
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
