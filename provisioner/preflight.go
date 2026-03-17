package provisioner

import (
	"context"
	"fmt"
	"net/http"
	"os/exec"
	"runtime"
	"time"
)

// PreflightResult contains the results of all pre-flight checks
type PreflightResult struct {
	DockerAvailable  bool
	ComposeAvailable bool
	InternetOK       bool
	DiskSpaceOK      bool
	Errors           []string
}

// AllPassed returns true if all pre-flight checks passed
func (r *PreflightResult) AllPassed() bool {
	return r.DockerAvailable && r.ComposeAvailable && r.InternetOK && r.DiskSpaceOK
}

// CriticalPassed returns true if critical checks passed (Docker + Compose)
func (r *PreflightResult) CriticalPassed() bool {
	return r.DockerAvailable && r.ComposeAvailable
}

// RunPreflightChecks runs all pre-flight checks before provisioning
func RunPreflightChecks(ctx context.Context) *PreflightResult {
	result := &PreflightResult{
		Errors: []string{},
	}

	// Check Docker daemon
	result.DockerAvailable = checkDocker(ctx)
	if !result.DockerAvailable {
		result.Errors = append(result.Errors,
			"Docker is not installed or not running. Please install Docker Desktop and ensure it is started.")
	}

	// Check Docker Compose
	result.ComposeAvailable = checkCompose(ctx)
	if !result.ComposeAvailable {
		result.Errors = append(result.Errors,
			"Docker Compose is not available. Please ensure Docker Desktop includes Docker Compose.")
	}

	// Check internet connectivity
	result.InternetOK = checkInternet(ctx)
	if !result.InternetOK {
		result.Errors = append(result.Errors,
			"No internet connectivity detected. Will use cached Docker images if available.")
	}

	// Check disk space
	result.DiskSpaceOK = checkDiskSpace()
	if !result.DiskSpaceOK {
		result.Errors = append(result.Errors,
			"Low disk space detected. A Supabase project requires at least 2GB of free space.")
	}

	return result
}

// checkDocker verifies Docker daemon is accessible
func checkDocker(ctx context.Context) bool {
	cmdCtx, cancel := context.WithTimeout(ctx, 10*time.Second)
	defer cancel()

	cmd := exec.CommandContext(cmdCtx, "docker", "version", "--format", "{{.Server.Version}}")
	err := cmd.Run()
	return err == nil
}

// checkCompose verifies Docker Compose is available
func checkCompose(ctx context.Context) bool {
	cmdCtx, cancel := context.WithTimeout(ctx, 10*time.Second)
	defer cancel()

	cmd := exec.CommandContext(cmdCtx, "docker", "compose", "version")
	err := cmd.Run()
	return err == nil
}

// checkInternet tests internet connectivity using multiple methods
func checkInternet(ctx context.Context) bool {
	// Method 1: HTTP HEAD to reliable endpoints
	endpoints := []string{
		"https://www.google.com",
		"https://1.1.1.1",
		"https://8.8.8.8",
	}

	client := &http.Client{Timeout: 10 * time.Second}
	for _, endpoint := range endpoints {
		req, err := http.NewRequestWithContext(ctx, "HEAD", endpoint, nil)
		if err != nil {
			continue
		}
		resp, err := client.Do(req)
		if err == nil {
			resp.Body.Close()
			return true
		}
	}

	// Method 2: DNS resolution
	cmdCtx, cancel := context.WithTimeout(ctx, 10*time.Second)
	defer cancel()

	var dnsCmd *exec.Cmd
	if runtime.GOOS == "windows" {
		dnsCmd = exec.CommandContext(cmdCtx, "nslookup", "google.com")
	} else {
		dnsCmd = exec.CommandContext(cmdCtx, "host", "google.com")
	}
	if err := dnsCmd.Run(); err == nil {
		return true
	}

	// Method 3: Ping test
	cmdCtx2, cancel2 := context.WithTimeout(ctx, 10*time.Second)
	defer cancel2()

	var pingCmd *exec.Cmd
	if runtime.GOOS == "windows" {
		pingCmd = exec.CommandContext(cmdCtx2, "ping", "-n", "1", "8.8.8.8")
	} else {
		pingCmd = exec.CommandContext(cmdCtx2, "ping", "-c", "1", "8.8.8.8")
	}
	return pingCmd.Run() == nil
}

// checkDiskSpace performs a basic disk space check (needs ~2GB for a Supabase stack)
func checkDiskSpace() bool {
	if runtime.GOOS == "windows" {
		cmd := exec.Command("powershell", "-Command",
			"(Get-PSDrive C).Free -gt 2147483648")
		output, err := cmd.Output()
		if err != nil {
			return true // Assume OK if we can't check
		}
		return string(output) != "False\r\n"
	}

	// Unix: check with df
	cmd := exec.Command("sh", "-c",
		fmt.Sprintf("df -k . | tail -1 | awk '{print $4}'"))
	output, err := cmd.Output()
	if err != nil {
		return true // Assume OK if we can't check
	}

	var freeKB int64
	fmt.Sscanf(string(output), "%d", &freeKB)
	return freeKB > 2*1024*1024 // 2GB in KB
}
