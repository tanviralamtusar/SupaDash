package provisioner

import (
	"bufio"
	"context"
	"fmt"
	"log/slog"
	"os"
	"os/exec"
	"path"
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

	// Step 3: Allocate unique ports, avoiding any host ports Docker already
	// has published (survives restarts and pre-existing projects).
	ports, err := p.portAllocator.AllocatePorts(config.ProjectID, p.usedHostPorts(ctx))
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

	// Resolve the project directory as the HOST Docker daemon sees it. Nested
	// `docker compose up` runs against the host daemon, so bind-mount sources in
	// the generated compose (kong.yml, functions) must be host paths — not the
	// API container's internal /app/projects path, which the host can't see
	// (Docker would silently create empty directories in its place).
	hostBase := p.hostProjectsDir(ctx)
	if hostBase == "" {
		// Fall back to an ABSOLUTE container path so the bind mount is at least
		// valid YAML (a bare relative path is parsed as a named volume). This
		// path is only correct when the API runs directly on the host; under
		// docker-in-docker the host can't see it and bind mounts will still fail.
		if abs, absErr := filepath.Abs(p.baseDir); absErr == nil {
			hostBase = abs
		} else {
			hostBase = p.baseDir
		}
		p.logger.Warn("could not resolve host path for projects dir; bind mounts may fail under docker-in-docker",
			"baseDir", p.baseDir, "fallback", hostBase)
	}
	config.HostProjectDir = path.Join(hostBase, config.ProjectID)
	p.logger.Info("resolved host project dir for bind mounts", "projectID", config.ProjectID, "hostDir", config.HostProjectDir)

	// Step 5: Render and write templates
	templates := map[string]string{
		"project-compose.tmpl.yml": "docker-compose.yml",
		"kong.tmpl.yml":            "kong.yml",
		"vector.tmpl.yml":          "vector.yml",
		"roles.tmpl.sql":           "roles.sql",
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

	// Create the functions directory with the main router + empty env file —
	// the edge-functions container mounts ./functions and needs a main service.
	if err := p.EnsureFunctionsDir(config.ProjectID); err != nil {
		p.cleanup(config.ProjectID)
		return nil, &ProvisionerError{
			ProjectID: config.ProjectID,
			Operation: "create_functions_dir",
			Err:       err,
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
		DBPort:       ports.DBPort,
		APIPort:      ports.APIPort,
		APIPortHTTPS: ports.APIPortHTTPS,
		StudioPort:   ports.StudioPort,
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

// hostProjectsDir returns the host filesystem path that backs the API
// container's projects directory (p.baseDir), by inspecting the API's own
// container mounts. Nested `docker compose` runs against the host daemon, so
// bind-mount sources in generated compose files must be host paths. Returns ""
// if it cannot be resolved (e.g. the API runs directly on the host).
func (p *DockerProvisioner) hostProjectsDir(ctx context.Context) string {
	self, err := os.Hostname()
	if err != nil {
		return ""
	}
	insp, err := p.client.ContainerInspect(ctx, self)
	if err != nil {
		return ""
	}

	// baseDir may be relative (e.g. "./projects"); mount destinations are
	// absolute, so resolve it against the API process's working directory first.
	base := p.baseDir
	if abs, absErr := filepath.Abs(base); absErr == nil {
		base = abs
	}
	base = strings.TrimRight(filepath.ToSlash(base), "/")
	best := ""
	for _, m := range insp.Mounts {
		dest := strings.TrimRight(filepath.ToSlash(m.Destination), "/")
		if dest == "" {
			continue
		}
		if dest == base {
			return m.Source // exact mount of the projects dir
		}
		// baseDir lives under this mount: append the remaining sub-path.
		if strings.HasPrefix(base+"/", dest+"/") {
			best = strings.TrimRight(m.Source, "/") + strings.TrimPrefix(base, dest)
		}
	}
	return best
}

// EnsureProjectReachable makes sure the running API container shares a Docker
// network with the given project's Kong gateway, then returns the internal base
// URL (http://<ref>-kong:8000) to reach it.
//
// Provisioned project stacks publish their ports on the Docker host, but the
// host firewall (e.g. Coolify's default rules) can block container->host
// traffic — so dialing the published host port from inside the API container
// times out. Routing container-to-container by name over a shared network
// avoids the host entirely. Idempotent and safe to call on every request.
func (p *DockerProvisioner) EnsureProjectReachable(ctx context.Context, ref string) (string, error) {
	kong := ref + "-kong"

	insp, err := p.client.ContainerInspect(ctx, kong)
	if err != nil {
		return "", fmt.Errorf("inspect kong container %q: %w", kong, err)
	}

	// The project's own network name contains "supabase-<ref>" (compose may
	// prefix it). Fall back to any attached network if the naming changes.
	var netName, kongIP string
	for name, ep := range insp.NetworkSettings.Networks {
		if strings.Contains(name, "supabase-"+ref) {
			netName, kongIP = name, ep.IPAddress
			break
		}
	}
	if netName == "" {
		for name, ep := range insp.NetworkSettings.Networks {
			netName, kongIP = name, ep.IPAddress
			break
		}
	}
	if netName == "" || kongIP == "" {
		return "", fmt.Errorf("kong container %q has no usable network/IP", kong)
	}

	// Docker sets a container's hostname to its own short ID by default, which
	// NetworkConnect accepts as the container identifier.
	self, err := os.Hostname()
	if err != nil {
		return "", fmt.Errorf("determine own container id: %w", err)
	}

	if err := p.client.NetworkConnect(ctx, netName, self, nil); err != nil {
		msg := strings.ToLower(err.Error())
		if !strings.Contains(msg, "already exists") && !strings.Contains(msg, "already connected") {
			return "", fmt.Errorf("connect api container %q to network %q: %w", self, netName, err)
		}
	}

	// Dial Kong by IP rather than by container name: Docker's embedded DNS is
	// unreliable at resolving container names for containers attached to a
	// network at runtime (returns SERVFAIL / "server misbehaving"), but the IP
	// on that network is directly routable once we're connected. We re-inspect
	// every call, so a Kong restart (new IP) is picked up automatically.
	p.logger.Info("project reachable via network", "ref", ref, "network", netName, "kongIP", kongIP, "self", self)
	return fmt.Sprintf("http://%s:8000", kongIP), nil
}

// usedHostPorts returns the set of host ports currently published by any
// Docker container. This is the authoritative source for avoiding port
// collisions and, unlike an in-process net.Listen check, correctly reflects
// ports held by other projects' containers across restarts.
func (p *DockerProvisioner) usedHostPorts(ctx context.Context) map[int]bool {
	used := make(map[int]bool)
	containers, err := p.client.ContainerList(ctx, types.ContainerListOptions{})
	if err != nil {
		p.logger.Warn("Failed to list containers for port check", "error", err.Error())
		return used
	}
	for _, c := range containers {
		for _, port := range c.Ports {
			if port.PublicPort != 0 {
				used[int(port.PublicPort)] = true
			}
		}
	}
	return used
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
