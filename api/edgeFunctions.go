package api

import (
	"context"
	"fmt"
	"net/http"
	"time"

	"github.com/gin-gonic/gin"
	"supadash/database"
	"supadash/provisioner"
)

// EdgeFunctionResponse mirrors the Supabase v1 API function object.
type EdgeFunctionResponse struct {
	Id             string `json:"id"`
	Slug           string `json:"slug"`
	Name           string `json:"name"`
	Status         string `json:"status"`
	Version        int32  `json:"version"`
	VerifyJwt      bool   `json:"verify_jwt"`
	EntrypointPath string `json:"entrypoint_path"`
	CreatedAt      int64  `json:"created_at"` // epoch ms
	UpdatedAt      int64  `json:"updated_at"` // epoch ms
}

func edgeFunctionResponseFromRow(f database.EdgeFunction) EdgeFunctionResponse {
	return EdgeFunctionResponse{
		Id:             fmt.Sprintf("%d", f.ID),
		Slug:           f.Slug,
		Name:           f.Name,
		Status:         f.Status,
		Version:        f.Version,
		VerifyJwt:      f.VerifyJwt,
		EntrypointPath: f.EntrypointPath,
		CreatedAt:      f.CreatedAt.Time.UnixMilli(),
		UpdatedAt:      f.UpdatedAt.Time.UnixMilli(),
	}
}

type EdgeFunctionFile struct {
	Name    string `json:"name"`
	Content string `json:"content"`
}

type DeployFunctionBody struct {
	Slug           string             `json:"slug"`
	Name           string             `json:"name"`
	VerifyJwt      *bool              `json:"verify_jwt,omitempty"`
	EntrypointPath string             `json:"entrypoint_path"`
	Body           string             `json:"body,omitempty"`  // single-file source
	Files          []EdgeFunctionFile `json:"files,omitempty"` // multi-file source
}

// GET /v1/projects/:ref/functions
func (a *Api) getV1ProjectFunctions(c *gin.Context) {
	projectRef := c.Param("ref")

	functions, err := a.queries.GetEdgeFunctions(c.Request.Context(), projectRef)
	if err != nil {
		a.logger.Error(fmt.Sprintf("Failed to list functions for %s: %v", projectRef, err))
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Internal Server Error"})
		return
	}

	response := make([]EdgeFunctionResponse, 0, len(functions))
	for _, f := range functions {
		response = append(response, edgeFunctionResponseFromRow(f))
	}
	c.JSON(http.StatusOK, response)
}

// GET /v1/projects/:ref/functions/:slug
func (a *Api) getV1ProjectFunction(c *gin.Context) {
	projectRef := c.Param("ref")
	slug := c.Param("function_slug")

	function, err := a.queries.GetEdgeFunction(c.Request.Context(), projectRef, slug)
	if err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "Function not found"})
		return
	}
	c.JSON(http.StatusOK, edgeFunctionResponseFromRow(function))
}

// GET /v1/projects/:ref/functions/:slug/body
func (a *Api) getV1ProjectFunctionBody(c *gin.Context) {
	projectRef := c.Param("ref")
	slug := c.Param("function_slug")

	function, err := a.queries.GetEdgeFunction(c.Request.Context(), projectRef, slug)
	if err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "Function not found"})
		return
	}

	fp := a.getFunctionProvisioner()
	if fp == nil {
		c.JSON(http.StatusServiceUnavailable, gin.H{"error": "Provisioner not available"})
		return
	}

	content, err := fp.ReadFunctionFile(projectRef, slug, function.EntrypointPath)
	if err != nil {
		a.logger.Error(fmt.Sprintf("Failed to read function body %s/%s: %v", projectRef, slug, err))
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to read function body"})
		return
	}

	c.Data(http.StatusOK, "text/typescript", []byte(content))
}

// POST /v1/projects/:ref/functions (create) and
// PATCH /v1/projects/:ref/functions/:slug (update) share deploy logic.
func (a *Api) deployV1ProjectFunction(c *gin.Context) {
	projectRef := c.Param("ref")

	var body DeployFunctionBody
	if err := c.ShouldBindJSON(&body); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid request body"})
		return
	}

	// PATCH carries the slug in the path
	if pathSlug := c.Param("function_slug"); pathSlug != "" {
		body.Slug = pathSlug
	}
	if body.Slug == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Function slug is required"})
		return
	}
	if err := provisioner.ValidateFunctionSlug(body.Slug); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	if body.Name == "" {
		body.Name = body.Slug
	}
	if body.EntrypointPath == "" {
		body.EntrypointPath = "index.ts"
	}

	// Build the file set to write
	files := make(map[string]string, len(body.Files)+1)
	for _, f := range body.Files {
		files[f.Name] = f.Content
	}
	if body.Body != "" {
		files[body.EntrypointPath] = body.Body
	}
	if len(files) == 0 {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Function source is required (body or files)"})
		return
	}
	if _, ok := files[body.EntrypointPath]; !ok {
		c.JSON(http.StatusBadRequest, gin.H{"error": fmt.Sprintf("entrypoint %q missing from files", body.EntrypointPath)})
		return
	}

	fp := a.getFunctionProvisioner()
	if fp == nil {
		c.JSON(http.StatusServiceUnavailable, gin.H{"error": "Provisioner not available"})
		return
	}

	if err := fp.WriteFunction(projectRef, body.Slug, files); err != nil {
		a.logger.Error(fmt.Sprintf("Failed to write function %s/%s: %v", projectRef, body.Slug, err))
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to write function files"})
		return
	}

	verifyJwt := true
	if body.VerifyJwt != nil {
		verifyJwt = *body.VerifyJwt
	}

	function, err := a.queries.UpsertEdgeFunction(c.Request.Context(), database.UpsertEdgeFunctionParams{
		ProjectRef:     projectRef,
		Slug:           body.Slug,
		Name:           body.Name,
		Status:         "ACTIVE",
		VerifyJwt:      verifyJwt,
		EntrypointPath: body.EntrypointPath,
	})
	if err != nil {
		a.logger.Error(fmt.Sprintf("Failed to save function %s/%s: %v", projectRef, body.Slug, err))
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Internal Server Error"})
		return
	}

	// Restart the runtime so the module cache picks up the new code
	restartCtx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()
	if err := fp.RestartService(restartCtx, projectRef, "edge-functions"); err != nil {
		a.logger.Warn(fmt.Sprintf("Failed to restart edge runtime for %s: %v", projectRef, err))
	}

	status := http.StatusCreated
	if function.Version > 1 {
		status = http.StatusOK
	}
	c.JSON(status, edgeFunctionResponseFromRow(function))
}

// DELETE /v1/projects/:ref/functions/:slug
func (a *Api) deleteV1ProjectFunction(c *gin.Context) {
	projectRef := c.Param("ref")
	slug := c.Param("function_slug")

	if _, err := a.queries.GetEdgeFunction(c.Request.Context(), projectRef, slug); err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "Function not found"})
		return
	}

	fp := a.getFunctionProvisioner()
	if fp != nil {
		if err := fp.DeleteFunction(projectRef, slug); err != nil {
			a.logger.Warn(fmt.Sprintf("Failed to delete function files %s/%s: %v", projectRef, slug, err))
		}
	}

	if err := a.queries.DeleteEdgeFunction(c.Request.Context(), projectRef, slug); err != nil {
		a.logger.Error(fmt.Sprintf("Failed to delete function %s/%s: %v", projectRef, slug, err))
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Internal Server Error"})
		return
	}

	if fp != nil {
		restartCtx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
		defer cancel()
		if err := fp.RestartService(restartCtx, projectRef, "edge-functions"); err != nil {
			a.logger.Warn(fmt.Sprintf("Failed to restart edge runtime for %s: %v", projectRef, err))
		}
	}

	c.JSON(http.StatusOK, gin.H{"message": "Function deleted"})
}
