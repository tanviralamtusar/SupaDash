package api

import (
	"fmt"
	"net/http"
	"net/http/httputil"
	"net/url"

	"github.com/gin-gonic/gin"
)

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

	kongUrlStr := fmt.Sprintf("http://localhost:%d", project.KongHttpPort.Int32)
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
