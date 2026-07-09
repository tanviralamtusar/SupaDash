package provisioner

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
)

// defaultMainFunction is the router deployed as the edge-runtime "main service".
// It spawns a worker per function directory, mirroring the official
// supabase self-hosted setup.
const defaultMainFunction = `// SupaDash edge-functions main router (auto-generated — do not edit)
console.log('main function started');

Deno.serve(async (req: Request) => {
  const url = new URL(req.url);
  const pathParts = url.pathname.split('/').filter(Boolean);
  const serviceName = pathParts[0];

  if (!serviceName) {
    return new Response(JSON.stringify({ msg: 'missing function name in request path' }), {
      status: 400,
      headers: { 'Content-Type': 'application/json' },
    });
  }

  const servicePath = ` + "`/home/deno/functions/${serviceName}`" + `;

  try {
    // @ts-ignore EdgeRuntime is provided by supabase/edge-runtime
    const worker = await EdgeRuntime.userWorkers.create({
      servicePath,
      memoryLimitMb: 150,
      workerTimeoutMs: 60 * 1000,
      noModuleCache: false,
      envVars: Object.entries(Deno.env.toObject()),
    });
    return await worker.fetch(req);
  } catch (e) {
    console.error(e);
    return new Response(JSON.stringify({ msg: String(e) }), {
      status: 500,
      headers: { 'Content-Type': 'application/json' },
    });
  }
});
`

var functionSlugPattern = regexp.MustCompile(`^[A-Za-z0-9][A-Za-z0-9_-]{0,62}$`)

// ValidateFunctionSlug rejects slugs that could escape the functions directory
// or collide with the reserved main router.
func ValidateFunctionSlug(slug string) error {
	if !functionSlugPattern.MatchString(slug) {
		return fmt.Errorf("invalid function slug %q: use letters, digits, '-' and '_' (max 63 chars)", slug)
	}
	if slug == "main" {
		return fmt.Errorf("slug %q is reserved", slug)
	}
	return nil
}

// functionsDir returns the host directory mounted into the edge-runtime container.
func (p *DockerProvisioner) functionsDir(projectID string) string {
	return filepath.Join(p.getProjectDir(projectID), "functions")
}

// EnsureFunctionsDir creates the functions directory, the main router and an
// (initially empty) env file for a project. Idempotent; called at provisioning
// and defensively before any function write.
func (p *DockerProvisioner) EnsureFunctionsDir(projectID string) error {
	mainDir := filepath.Join(p.functionsDir(projectID), "main")
	if err := os.MkdirAll(mainDir, 0755); err != nil {
		return fmt.Errorf("failed to create functions dir: %w", err)
	}

	mainPath := filepath.Join(mainDir, "index.ts")
	if _, err := os.Stat(mainPath); os.IsNotExist(err) {
		if err := os.WriteFile(mainPath, []byte(defaultMainFunction), 0644); err != nil {
			return fmt.Errorf("failed to write main function: %w", err)
		}
	}

	envPath := filepath.Join(p.getProjectDir(projectID), "functions.env")
	if _, err := os.Stat(envPath); os.IsNotExist(err) {
		if err := os.WriteFile(envPath, []byte(""), 0600); err != nil {
			return fmt.Errorf("failed to write functions env file: %w", err)
		}
	}

	return nil
}

// WriteFunction writes (or overwrites) a function's source files under
// functions/<slug>/ and returns the entrypoint file name.
func (p *DockerProvisioner) WriteFunction(projectID, slug string, files map[string]string) error {
	if err := ValidateFunctionSlug(slug); err != nil {
		return err
	}
	if err := p.EnsureFunctionsDir(projectID); err != nil {
		return err
	}

	fnDir := filepath.Join(p.functionsDir(projectID), slug)
	if err := os.MkdirAll(fnDir, 0755); err != nil {
		return fmt.Errorf("failed to create function dir: %w", err)
	}

	for name, content := range files {
		// Prevent path traversal out of the function directory
		cleaned := filepath.Clean(name)
		if strings.HasPrefix(cleaned, "..") || filepath.IsAbs(cleaned) {
			return fmt.Errorf("invalid file name %q", name)
		}
		dest := filepath.Join(fnDir, cleaned)
		if err := os.MkdirAll(filepath.Dir(dest), 0755); err != nil {
			return fmt.Errorf("failed to create dir for %s: %w", name, err)
		}
		if err := os.WriteFile(dest, []byte(content), 0644); err != nil {
			return fmt.Errorf("failed to write %s: %w", name, err)
		}
	}

	return nil
}

// ReadFunctionFile returns the content of a single file of a deployed function.
func (p *DockerProvisioner) ReadFunctionFile(projectID, slug, file string) (string, error) {
	if err := ValidateFunctionSlug(slug); err != nil {
		return "", err
	}
	cleaned := filepath.Clean(file)
	if strings.HasPrefix(cleaned, "..") || filepath.IsAbs(cleaned) {
		return "", fmt.Errorf("invalid file name %q", file)
	}
	data, err := os.ReadFile(filepath.Join(p.functionsDir(projectID), slug, cleaned))
	if err != nil {
		return "", err
	}
	return string(data), nil
}

// DeleteFunction removes a function's directory.
func (p *DockerProvisioner) DeleteFunction(projectID, slug string) error {
	if err := ValidateFunctionSlug(slug); err != nil {
		return err
	}
	return os.RemoveAll(filepath.Join(p.functionsDir(projectID), slug))
}

// WriteFunctionsEnv renders the plaintext env file consumed by the
// edge-runtime container (via env_file in the compose template).
func (p *DockerProvisioner) WriteFunctionsEnv(projectID string, secrets map[string]string) error {
	if err := p.EnsureFunctionsDir(projectID); err != nil {
		return err
	}

	// Deterministic ordering keeps the file diff-friendly
	names := make([]string, 0, len(secrets))
	for name := range secrets {
		names = append(names, name)
	}
	sort.Strings(names)

	var b strings.Builder
	b.WriteString("# Managed by SupaDash — deployed edge function secrets\n")
	for _, name := range names {
		// env-file format: no quoting of values, strip newlines defensively
		value := strings.ReplaceAll(secrets[name], "\n", "")
		fmt.Fprintf(&b, "%s=%s\n", name, value)
	}

	envPath := filepath.Join(p.getProjectDir(projectID), "functions.env")
	return os.WriteFile(envPath, []byte(b.String()), 0600)
}

// RestartService restarts a single compose service of a project (e.g. the
// edge-functions runtime after a deploy).
func (p *DockerProvisioner) RestartService(ctx context.Context, projectID, service string) error {
	restartCmd := exec.CommandContext(ctx, "docker", "compose", "restart", service)
	restartCmd.Dir = p.getProjectDir(projectID)
	if output, err := restartCmd.CombinedOutput(); err != nil {
		return fmt.Errorf("failed to restart %s: %w\nOutput: %s", service, err, string(output))
	}
	return nil
}
