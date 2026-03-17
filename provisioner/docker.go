package provisioner

import (
	"bufio"
	"context"
	"fmt"
	"log/slog"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync"
	"text/template"
	"time"

	"github.com/docker/docker/api/types"
	"github.com/docker/docker/client"
)

// DockerProvisioner implements the Provisioner interface using Docker
type DockerProvisioner struct {
	client *client.Client
	logger *slog.Logger

	// Base directory for storing project files
	// Each project gets a subdirectory: baseDir/projectID/
	baseDir string

	// Template directory containing docker-compose templates
	templateDir string

	// Port allocator for unique port assignment
	portAllocator *PortAllocator

	// Mutex for thread-safe operations
	mu sync.RWMutex

	// Cache of project information
	projects map[string]*ProjectInfo
}

// NewDockerProvisioner creates a new Docker-based provisioner
func NewDockerProvisioner(baseDir, templateDir string, logger *slog.Logger) (*DockerProvisioner, error) {
	// Create Docker client
	cli, err := client.NewClientWithOpts(client.FromEnv, client.WithAPIVersionNegotiation())
	if err != nil {
		return nil, fmt.Errorf("failed to create Docker client: %w", err)
	}

	// Verify Docker is accessible
	ctx := context.Background()
	_, err = cli.Ping(ctx)
	if err != nil {
		return nil, fmt.Errorf("failed to connect to Docker daemon: %w", err)
	}

	// Ensure base directory exists
	if err := os.MkdirAll(baseDir, 0755); err != nil {
		return nil, fmt.Errorf("failed to create projects directory: %w", err)
	}

	return &DockerProvisioner{
		client:        cli,
		logger:        logger,
		baseDir:       baseDir,
		templateDir:   templateDir,
		portAllocator: NewPortAllocator(5433, 54321),
		projects:      make(map[string]*ProjectInfo),
	}, nil
}

// CreateProject provisions a new Supabase instance using Docker Compose
func (p *DockerProvisioner) CreateProject(ctx context.Context, config *ProjectConfig) (*ProjectInfo, error) {
	p.logger.Info("Starting project provisioning", "projectID", config.ProjectID, "name", config.ProjectName)

	// Step 1: Run pre-flight checks
	preflight := RunPreflightChecks(ctx)
	if !preflight.CriticalPassed() {
		return nil, &ProvisionerError{
			ProjectID: config.ProjectID,
			Operation: "preflight",
			Err:       fmt.Errorf("pre-flight checks failed: %v", preflight.Errors),
		}
	}
	if !preflight.InternetOK {
		p.logger.Warn("No internet connectivity — will use cached Docker images if available")
	}

	// Step 2: Generate secrets if not provided
	if config.JWTSecret == "" || config.AnonKey == "" || config.ServiceKey == "" {
		secrets, err := GenerateProjectSecrets()
		if err != nil {
			return nil, &ProvisionerError{
				ProjectID: config.ProjectID,
				Operation: "generate_secrets",
				Err:       err,
			}
		}
		config.JWTSecret = secrets.JWTSecret
		config.AnonKey = secrets.AnonKey
		config.ServiceKey = secrets.ServiceKey
		config.DBPassword = secrets.DBPassword
		config.DashboardUser = secrets.DashboardUser
		config.DashboardPass = secrets.DashboardPass
	}

	// Step 3: Allocate unique ports
	ports, err := p.portAllocator.AllocatePorts(config.ProjectID)
	if err != nil {
		return nil, &ProvisionerError{
			ProjectID: config.ProjectID,
			Operation: "allocate_ports",
			Err:       err,
		}
	}
	config.DBPort = ports.DBPort
	config.APIPort = ports.APIPort
	config.StudioPort = ports.StudioPort
	p.logger.Info("Ports allocated",
		"projectID", config.ProjectID,
		"dbPort", ports.DBPort,
		"apiPort", ports.APIPort,
		"studioPort", ports.StudioPort,
	)

	// Step 4: Create project directory
	projectDir := p.getProjectDir(config.ProjectID)
	if err := os.MkdirAll(projectDir, 0755); err != nil {
		p.portAllocator.ReleasePorts(config.ProjectID)
		return nil, &ProvisionerError{
			ProjectID: config.ProjectID,
			Operation: "create_directory",
			Err:       fmt.Errorf("failed to create project directory: %w", err),
		}
	}

	// Step 5: Render and write templates
	templates := map[string]string{
		"project-compose.tmpl.yml": "docker-compose.yml",
		"kong.tmpl.yml":            "kong.yml",
		"vector.tmpl.yml":          "vector.yml",
	}

	for tmplFile, outputFile := range templates {
		content, err := p.renderTemplate(tmplFile, config)
		if err != nil {
			p.cleanup(config.ProjectID)
			return nil, &ProvisionerError{
				ProjectID: config.ProjectID,
				Operation: "render_template",
				Err:       fmt.Errorf("failed to render %s: %w", tmplFile, err),
			}
		}

		outputPath := filepath.Join(projectDir, outputFile)
		if err := os.WriteFile(outputPath, []byte(content), 0644); err != nil {
			p.cleanup(config.ProjectID)
			return nil, &ProvisionerError{
				ProjectID: config.ProjectID,
				Operation: "write_template",
				Err:       fmt.Errorf("failed to write %s: %w", outputFile, err),
			}
		}
	}

	p.logger.Info("Templates rendered successfully", "projectID", config.ProjectID, "dir", projectDir)

	// Step 6: Pull images (if internet available)
	if preflight.InternetOK {
		p.logger.Info("Pulling Docker images...", "projectID", config.ProjectID)
		pullCtx, pullCancel := context.WithTimeout(ctx, 5*time.Minute)
		defer pullCancel()

		pullCmd := exec.CommandContext(pullCtx, "docker", "compose", "pull")
		pullCmd.Dir = projectDir
		if output, err := pullCmd.CombinedOutput(); err != nil {
			p.logger.Warn("Failed to pull some images, will try cached",
				"projectID", config.ProjectID,
				"error", err.Error(),
				"output", string(output),
			)
			// Continue — cached images may work
		}
	}

	// Step 7: Start containers with docker compose up
	p.logger.Info("Starting Supabase containers...", "projectID", config.ProjectID)
	upCtx, upCancel := context.WithTimeout(ctx, 5*time.Minute)
	defer upCancel()

	upCmd := exec.CommandContext(upCtx, "docker", "compose", "up", "-d", "--remove-orphans")
	upCmd.Dir = projectDir
	upOutput, err := upCmd.CombinedOutput()
	if err != nil {
		p.logger.Error("Docker compose up failed",
			"projectID", config.ProjectID,
			"error", err.Error(),
			"output", string(upOutput),
		)
		p.cleanup(config.ProjectID)
		return nil, &ProvisionerError{
			ProjectID: config.ProjectID,
			Operation: "docker_compose_up",
			Err:       fmt.Errorf("docker compose up failed: %w\nOutput: %s", err, string(upOutput)),
		}
	}

	p.logger.Info("Docker compose up completed", "projectID", config.ProjectID)

	// Step 8: Verify containers are running
	info, err := p.waitForHealthy(ctx, config, ports)
	if err != nil {
		p.logger.Warn("Some containers may not be healthy yet",
			"projectID", config.ProjectID,
			"error", err.Error(),
		)
		// Don't fail — containers may still be starting
	}

	// Step 9: Cache project info
	p.mu.Lock()
	p.projects[config.ProjectID] = info
	p.mu.Unlock()

	p.logger.Info("Project provisioned successfully",
		"projectID", config.ProjectID,
		"name", config.ProjectName,
		"apiPort", ports.APIPort,
		"dbPort", ports.DBPort,
	)

	return info, nil
}

// GetProjectInfo retrieves current information about a project
func (p *DockerProvisioner) GetProjectInfo(ctx context.Context, projectID string) (*ProjectInfo, error) {
	projectDir := p.getProjectDir(projectID)

	// Check if project directory exists
	if _, err := os.Stat(projectDir); os.IsNotExist(err) {
		return nil, fmt.Errorf("project %s not found", projectID)
	}

	// Get container status via docker compose ps
	psCmd := exec.CommandContext(ctx, "docker", "compose", "ps", "--format", "{{.Name}}\t{{.State}}\t{{.Health}}")
	psCmd.Dir = projectDir
	output, err := psCmd.Output()
	if err != nil {
		return nil, fmt.Errorf("failed to get container status: %w", err)
	}

	containers := make(map[string]string)
	healthChecks := make(map[string]bool)

	scanner := bufio.NewScanner(strings.NewReader(string(output)))
	for scanner.Scan() {
		parts := strings.Split(scanner.Text(), "\t")
		if len(parts) >= 2 {
			name := parts[0]
			state := parts[1]
			containers[name] = state
			healthChecks[name] = state == "running"
		}
	}

	status := StatusActive
	for _, state := range containers {
		if state != "running" {
			status = StatusUnhealthy
			break
		}
	}

	return &ProjectInfo{
		ProjectID:    projectID,
		Status:       status,
		Containers:   containers,
		HealthChecks: healthChecks,
		UpdatedAt:    time.Now().Format(time.RFC3339),
	}, nil
}

// UpdateProject updates project configuration
func (p *DockerProvisioner) UpdateProject(ctx context.Context, projectID string, config *ProjectConfig) error {
	return fmt.Errorf("not implemented yet — Phase 2")
}

// PauseProject stops all containers without deleting data
func (p *DockerProvisioner) PauseProject(ctx context.Context, projectID string) error {
	projectDir := p.getProjectDir(projectID)

	stopCmd := exec.CommandContext(ctx, "docker", "compose", "stop")
	stopCmd.Dir = projectDir
	if output, err := stopCmd.CombinedOutput(); err != nil {
		return &ProvisionerError{
			ProjectID: projectID,
			Operation: "pause",
			Err:       fmt.Errorf("docker compose stop failed: %w\nOutput: %s", err, string(output)),
		}
	}

	p.mu.Lock()
	if info, ok := p.projects[projectID]; ok {
		info.Status = StatusPaused
	}
	p.mu.Unlock()

	return nil
}

// ResumeProject restarts all containers for a paused project
func (p *DockerProvisioner) ResumeProject(ctx context.Context, projectID string) error {
	projectDir := p.getProjectDir(projectID)

	startCmd := exec.CommandContext(ctx, "docker", "compose", "start")
	startCmd.Dir = projectDir
	if output, err := startCmd.CombinedOutput(); err != nil {
		return &ProvisionerError{
			ProjectID: projectID,
			Operation: "resume",
			Err:       fmt.Errorf("docker compose start failed: %w\nOutput: %s", err, string(output)),
		}
	}

	p.mu.Lock()
	if info, ok := p.projects[projectID]; ok {
		info.Status = StatusActive
	}
	p.mu.Unlock()

	return nil
}

// DeleteProject removes all containers and volumes
func (p *DockerProvisioner) DeleteProject(ctx context.Context, projectID string) error {
	projectDir := p.getProjectDir(projectID)

	// Step 1: Stop and remove containers + volumes
	downCmd := exec.CommandContext(ctx, "docker", "compose", "down", "--volumes", "--remove-orphans")
	downCmd.Dir = projectDir
	if output, err := downCmd.CombinedOutput(); err != nil {
		p.logger.Warn("docker compose down failed (containers may not be running)",
			"projectID", projectID,
			"error", err.Error(),
			"output", string(output),
		)
	}

	// Step 2: Remove project directory
	if err := os.RemoveAll(projectDir); err != nil {
		p.logger.Warn("Failed to remove project directory",
			"projectID", projectID,
			"error", err.Error(),
		)
	}

	// Step 3: Release ports
	p.portAllocator.ReleasePorts(projectID)

	// Step 4: Remove from cache
	p.mu.Lock()
	delete(p.projects, projectID)
	p.mu.Unlock()

	p.logger.Info("Project deleted", "projectID", projectID)
	return nil
}

// ListProjects returns all provisioned projects
func (p *DockerProvisioner) ListProjects(ctx context.Context) ([]*ProjectInfo, error) {
	entries, err := os.ReadDir(p.baseDir)
	if err != nil {
		return nil, fmt.Errorf("failed to read projects directory: %w", err)
	}

	var projects []*ProjectInfo
	for _, entry := range entries {
		if !entry.IsDir() {
			continue
		}
		// Check if it has a docker-compose.yml (valid project)
		composePath := filepath.Join(p.baseDir, entry.Name(), "docker-compose.yml")
		if _, err := os.Stat(composePath); os.IsNotExist(err) {
			continue
		}

		info, err := p.GetProjectInfo(ctx, entry.Name())
		if err != nil {
			p.logger.Warn("Failed to get info for project", "projectID", entry.Name(), "error", err.Error())
			continue
		}
		projects = append(projects, info)
	}

	return projects, nil
}

// GetLogs retrieves logs for a specific service
func (p *DockerProvisioner) GetLogs(ctx context.Context, projectID string, service string, tail int) ([]string, error) {
	projectDir := p.getProjectDir(projectID)

	args := []string{"compose", "logs", service}
	if tail > 0 {
		args = append(args, "--tail", fmt.Sprintf("%d", tail))
	}

	logsCmd := exec.CommandContext(ctx, "docker", args...)
	logsCmd.Dir = projectDir
	output, err := logsCmd.CombinedOutput()
	if err != nil {
		return nil, fmt.Errorf("failed to get logs for %s: %w", service, err)
	}

	var lines []string
	scanner := bufio.NewScanner(strings.NewReader(string(output)))
	for scanner.Scan() {
		lines = append(lines, scanner.Text())
	}
	return lines, nil
}

// ExecuteCommand runs a command in a service container
func (p *DockerProvisioner) ExecuteCommand(ctx context.Context, projectID string, service string, cmd []string) (string, error) {
	projectDir := p.getProjectDir(projectID)

	args := append([]string{"compose", "exec", service}, cmd...)
	execCmd := exec.CommandContext(ctx, "docker", args...)
	execCmd.Dir = projectDir
	output, err := execCmd.CombinedOutput()
	if err != nil {
		return "", fmt.Errorf("failed to execute command in %s: %w", service, err)
	}

	return string(output), nil
}

// --- Helper functions ---

// getProjectDir returns the directory path for a project
func (p *DockerProvisioner) getProjectDir(projectID string) string {
	return filepath.Join(p.baseDir, projectID)
}

// renderTemplate renders a Go template file with project config
func (p *DockerProvisioner) renderTemplate(templateFile string, config *ProjectConfig) (string, error) {
	tmplPath := filepath.Join(p.templateDir, templateFile)

	tmpl, err := template.ParseFiles(tmplPath)
	if err != nil {
		return "", fmt.Errorf("failed to parse template %s: %w", templateFile, err)
	}

	var buf strings.Builder
	if err := tmpl.Execute(&buf, config); err != nil {
		return "", fmt.Errorf("failed to execute template %s: %w", templateFile, err)
	}

	return buf.String(), nil
}

// waitForHealthy waits for containers to reach healthy state
func (p *DockerProvisioner) waitForHealthy(ctx context.Context, config *ProjectConfig, ports *PortAllocation) (*ProjectInfo, error) {
	info := &ProjectInfo{
		ProjectID:   config.ProjectID,
		ProjectName: config.ProjectName,
		Status:      StatusActive,
		Endpoint:    fmt.Sprintf("http://localhost:%d", ports.APIPort),
		DBEndpoint:  fmt.Sprintf("postgresql://postgres:%s@localhost:%d/postgres", config.DBPassword, ports.DBPort),
		Containers:  make(map[string]string),
		HealthChecks: make(map[string]bool),
		CreatedAt:   time.Now().Format(time.RFC3339),
		UpdatedAt:   time.Now().Format(time.RFC3339),
	}

	// Wait up to 60 seconds for containers to start
	deadline := time.Now().Add(60 * time.Second)
	for time.Now().Before(deadline) {
		projectDir := p.getProjectDir(config.ProjectID)
		psCmd := exec.CommandContext(ctx, "docker", "compose", "ps", "--format", "{{.Name}}\t{{.State}}")
		psCmd.Dir = projectDir
		output, err := psCmd.Output()
		if err != nil {
			time.Sleep(5 * time.Second)
			continue
		}

		allRunning := true
		containerCount := 0
		scanner := bufio.NewScanner(strings.NewReader(string(output)))
		for scanner.Scan() {
			parts := strings.Split(scanner.Text(), "\t")
			if len(parts) >= 2 {
				containerCount++
				name := parts[0]
				state := parts[1]
				info.Containers[name] = state
				info.HealthChecks[name] = state == "running"
				if state != "running" {
					allRunning = false
				}
			}
		}

		if containerCount > 0 && allRunning {
			p.logger.Info("All containers healthy",
				"projectID", config.ProjectID,
				"count", containerCount,
			)
			return info, nil
		}

		time.Sleep(5 * time.Second)
	}

	return info, fmt.Errorf("timeout waiting for containers to be healthy")
}

// cleanup removes all resources on provisioning failure
func (p *DockerProvisioner) cleanup(projectID string) {
	projectDir := p.getProjectDir(projectID)

	// Try to stop any partially started containers
	downCmd := exec.Command("docker", "compose", "down", "--volumes", "--remove-orphans")
	downCmd.Dir = projectDir
	_ = downCmd.Run()

	// Remove project directory
	_ = os.RemoveAll(projectDir)

	// Release ports
	p.portAllocator.ReleasePorts(projectID)
}

// GetPortAllocator returns the port allocator (for registering existing projects)
func (p *DockerProvisioner) GetPortAllocator() *PortAllocator {
	return p.portAllocator
}

// listContainers lists all containers for a project using the Docker API
func (p *DockerProvisioner) listContainers(ctx context.Context, projectID string) ([]string, error) {
	containers, err := p.client.ContainerList(ctx, types.ContainerListOptions{})
	if err != nil {
		return nil, err
	}

	var projectContainers []string
	prefix := projectID + "-"
	for _, c := range containers {
		for _, name := range c.Names {
			cleanName := strings.TrimPrefix(name, "/")
			if strings.HasPrefix(cleanName, prefix) {
				projectContainers = append(projectContainers, cleanName)
			}
		}
	}

	return projectContainers, nil
}
