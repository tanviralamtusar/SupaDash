package api

import (
	"context"
	"fmt"
	"net/http"
	"time"

	"github.com/gin-gonic/gin"
	"supadash/database"
	"supadash/utils"
)

type Secret struct {
	Name  string `json:"name" binding:"required"`
	Value string `json:"value"`
}

// functionProvisioner is the optional provisioner capability set needed for
// edge function file/secret management. The Docker provisioner implements it.
type functionProvisioner interface {
	EnsureFunctionsDir(projectID string) error
	WriteFunction(projectID, slug string, files map[string]string) error
	ReadFunctionFile(projectID, slug, file string) (string, error)
	DeleteFunction(projectID, slug string) error
	WriteFunctionsEnv(projectID string, secrets map[string]string) error
	RestartService(ctx context.Context, projectID, service string) error
}

// getFunctionProvisioner returns the provisioner's function-management
// capabilities, or nil when unavailable (provisioning disabled).
func (a *Api) getFunctionProvisioner() functionProvisioner {
	if a.provisioner == nil {
		return nil
	}
	fp, ok := a.provisioner.(functionProvisioner)
	if !ok {
		return nil
	}
	return fp
}

// decryptedProjectSecrets loads and decrypts all secrets of a project.
func (a *Api) decryptedProjectSecrets(ctx context.Context, projectRef string) (map[string]string, error) {
	rows, err := a.queries.GetProjectSecrets(ctx, projectRef)
	if err != nil {
		return nil, err
	}
	secrets := make(map[string]string, len(rows))
	for _, row := range rows {
		value, err := utils.DecryptString(a.config.EncryptionSecret, row.ValueEncrypted)
		if err != nil {
			// Skip undecryptable rows (e.g. rotated encryption secret) rather than failing the whole set
			a.logger.Warn("Failed to decrypt project secret", "project", projectRef, "name", row.Name, "error", err.Error())
			continue
		}
		secrets[row.Name] = value
	}
	return secrets, nil
}

// syncFunctionsEnv rewrites the project's functions.env from the stored
// secrets and restarts the edge runtime so changes take effect.
func (a *Api) syncFunctionsEnv(ctx context.Context, projectRef string) error {
	fp := a.getFunctionProvisioner()
	if fp == nil {
		return nil // provisioning disabled — secrets are stored but not injected
	}

	secrets, err := a.decryptedProjectSecrets(ctx, projectRef)
	if err != nil {
		return err
	}
	if err := fp.WriteFunctionsEnv(projectRef, secrets); err != nil {
		return err
	}

	restartCtx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()
	if err := fp.RestartService(restartCtx, projectRef, "edge-functions"); err != nil {
		// Non-fatal: the env file is in place, next container start picks it up
		a.logger.Warn("Failed to restart edge runtime after secrets change", "project", projectRef, "error", err.Error())
	}
	return nil
}

func (a *Api) getV1ProjectSecrets(c *gin.Context) {
	_, err := a.GetAccountFromRequest(c)
	if err != nil {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "Unauthorized"})
		return
	}

	projectRef := c.Param("ref")
	secrets, err := a.decryptedProjectSecrets(c.Request.Context(), projectRef)
	if err != nil {
		a.logger.Error(fmt.Sprintf("Failed to load secrets for %s: %v", projectRef, err))
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Internal Server Error"})
		return
	}

	response := make([]Secret, 0, len(secrets))
	for name, value := range secrets {
		response = append(response, Secret{Name: name, Value: value})
	}
	c.JSON(http.StatusOK, response)
}

func (a *Api) postV1ProjectSecrets(c *gin.Context) {
	_, err := a.GetAccountFromRequest(c)
	if err != nil {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "Unauthorized"})
		return
	}

	projectRef := c.Param("ref")

	var req []Secret
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid request body"})
		return
	}
	if len(req) == 0 {
		c.JSON(http.StatusBadRequest, gin.H{"error": "No secrets provided"})
		return
	}

	for _, secret := range req {
		encrypted, err := utils.EncryptString(a.config.EncryptionSecret, secret.Value)
		if err != nil {
			a.logger.Error(fmt.Sprintf("Failed to encrypt secret for %s: %v", projectRef, err))
			c.JSON(http.StatusInternalServerError, gin.H{"error": "Internal Server Error"})
			return
		}
		if err := a.queries.UpsertProjectSecret(c.Request.Context(), database.UpsertProjectSecretParams{
			ProjectRef:     projectRef,
			Name:           secret.Name,
			ValueEncrypted: encrypted,
		}); err != nil {
			a.logger.Error(fmt.Sprintf("Failed to store secret for %s: %v", projectRef, err))
			c.JSON(http.StatusInternalServerError, gin.H{"error": "Internal Server Error"})
			return
		}
	}

	if err := a.syncFunctionsEnv(c.Request.Context(), projectRef); err != nil {
		a.logger.Warn(fmt.Sprintf("Failed to sync functions env for %s: %v", projectRef, err))
	}

	c.JSON(http.StatusCreated, gin.H{"message": "Successfully created secrets"})
}

func (a *Api) deleteV1ProjectSecrets(c *gin.Context) {
	_, err := a.GetAccountFromRequest(c)
	if err != nil {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "Unauthorized"})
		return
	}

	projectRef := c.Param("ref")

	var req []string
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid request body"})
		return
	}

	for _, name := range req {
		if err := a.queries.DeleteProjectSecret(c.Request.Context(), projectRef, name); err != nil {
			a.logger.Error(fmt.Sprintf("Failed to delete secret %s for %s: %v", name, projectRef, err))
			c.JSON(http.StatusInternalServerError, gin.H{"error": "Internal Server Error"})
			return
		}
	}

	if err := a.syncFunctionsEnv(c.Request.Context(), projectRef); err != nil {
		a.logger.Warn(fmt.Sprintf("Failed to sync functions env for %s: %v", projectRef, err))
	}

	c.JSON(http.StatusOK, gin.H{"message": "Successfully deleted secrets"})
}
