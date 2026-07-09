package api

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httputil"
	"net/url"

	"github.com/gin-gonic/gin"
)

// projectReachabilityEnsurer is implemented by the Docker provisioner: it joins
// the API container to a project's Docker network and returns the internal Kong
// base URL. Declared locally so the api package doesn't depend on the concrete
// provisioner type (and degrades gracefully for provisioners that don't support it).
type projectReachabilityEnsurer interface {
	EnsureProjectReachable(ctx context.Context, ref string) (string, error)
}

// projectKongBaseURL returns the base URL the API should use to reach a
// project's Kong gateway. It prefers container-to-container networking
// (http://<ref>-kong:8000), which avoids the host firewall, and falls back to
// the published host port via the configured project host if that fails.
func (a *Api) projectKongBaseURL(ctx context.Context, ref string, kongHostPort int32) string {
	fallback := fmt.Sprintf("http://%s:%d", a.config.Provisioning.ProjectHost, kongHostPort)
	ensurer, ok := a.provisioner.(projectReachabilityEnsurer)
	if !ok {
		return fallback
	}
	base, err := ensurer.EnsureProjectReachable(ctx, ref)
	if err != nil {
		a.logger.Warn("could not join project network; falling back to host port", "ref", ref, "error", err.Error())
		return fallback
	}
	return base
}

func (a *Api) anyPlatformPgMetaProxy(c *gin.Context) {
	_, err := a.GetAccountFromRequest(c)
	if err != nil {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "Unauthorized"})
		return
	}

	projectRef := c.Param("ref")
	path := c.Param("path")

	// Special case for root `/platform/pg-meta/:ref` or `/platform/pg-meta/:ref/`
	if path == "" {
		path = "/"
	}

	// Fetch project to get the KongHttpPort
	project, err := a.queries.GetProjectByRef(c, projectRef)
	if err != nil {
		a.logger.Error("Failed to fetch project for pg-meta proxy", "ref", projectRef, "error", err)
		c.JSON(http.StatusNotFound, gin.H{"error": "Project not found"})
		return
	}

	if !project.KongHttpPort.Valid {
		a.logger.Error("Project has no Kong HTTP port assigned", "ref", projectRef)
		c.JSON(http.StatusServiceUnavailable, gin.H{"error": "Project is not fully provisioned yet"})
		return
	}

	kongUrlStr := a.projectKongBaseURL(c.Request.Context(), projectRef, project.KongHttpPort.Int32)
	remote, err := url.Parse(kongUrlStr)
	if err != nil {
		a.logger.Error("Failed to parse kong URL", "url", kongUrlStr, "error", err)
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Internal Proxy Error"})
		return
	}

	proxy := httputil.NewSingleHostReverseProxy(remote)

	// Update the request Director to rewrite the path
	originalDirector := proxy.Director
	proxy.Director = func(req *http.Request) {
		originalDirector(req)
		
		// The incoming path is /platform/pg-meta/:ref/something
		// We want to rewrite it to /pg/something
		req.URL.Path = "/pg" + path
		
		// Ensure Host header matches the target
		req.Host = remote.Host
		
		// Inject the project's service_role key to bypass Kong key-auth
		if project.ServiceRoleKey.Valid {
			req.Header.Set("apikey", project.ServiceRoleKey.String)
			req.Header.Set("Authorization", "Bearer "+project.ServiceRoleKey.String)
		}
	}

	proxy.ModifyResponse = func(res *http.Response) error {
		// Log response for debugging
		if res.StatusCode >= 400 {
			a.logger.Warn("pg-meta proxy returned error status", "status", res.StatusCode, "path", path)
		}
		
		// Ensure CORS headers are maintained inside proxy response if necessary
		// Currently Kong returns CORS using its plugin.
		return nil
	}

	// Execute proxy
	proxy.ServeHTTP(c.Writer, c.Request)
}
