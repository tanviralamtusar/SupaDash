package api

import (
	"context"
	"fmt"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/gin-gonic/gin"
	"supadash/database"
	"supadash/provisioner"
)

// BranchResponse mirrors the Supabase v1 API branch object.
type BranchResponse struct {
	Id               string `json:"id"`
	Name             string `json:"name"`
	ProjectRef       string `json:"project_ref"`        // ref the branch is served under (parent for DB-only branches)
	ParentProjectRef string `json:"parent_project_ref"`
	IsDefault        bool   `json:"is_default"`
	GitBranch        string `json:"git_branch,omitempty"`
	Status           string `json:"status"`
	DbName           string `json:"db_name"`
	CreatedAt        string `json:"created_at"`
	UpdatedAt        string `json:"updated_at"`
}

func branchResponseFromRow(b database.ProjectBranch) BranchResponse {
	return BranchResponse{
		Id:               fmt.Sprintf("%d", b.ID),
		Name:             b.BranchName,
		ProjectRef:       b.ParentProjectRef,
		ParentProjectRef: b.ParentProjectRef,
		IsDefault:        false,
		GitBranch:        b.GitBranch.String,
		Status:           b.Status,
		DbName:           b.DbName,
		CreatedAt:        b.CreatedAt.Time.Format(time.RFC3339),
		UpdatedAt:        b.UpdatedAt.Time.Format(time.RFC3339),
	}
}

type CreateBranchBody struct {
	BranchName string `json:"branch_name" binding:"required"`
	GitBranch  string `json:"git_branch"`
}

// branchOperationTimeout bounds async clone/merge/rebase work.
const branchOperationTimeout = 10 * time.Minute

// requireBranchRole loads a branch by the :branch_id path param and verifies
// the caller holds one of the roles in the parent project's organization.
func (a *Api) requireBranchRole(c *gin.Context, roles ...string) (database.ProjectBranch, bool) {
	var zero database.ProjectBranch

	account, err := a.GetAccountFromRequest(c)
	if err != nil {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "Unauthorized"})
		return zero, false
	}

	id, err := strconv.Atoi(c.Param("branch_id"))
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid branch id"})
		return zero, false
	}

	branch, err := a.queries.GetProjectBranch(c.Request.Context(), int32(id))
	if err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "Branch not found"})
		return zero, false
	}

	membership, err := a.queries.GetOrganizationMembershipByProjectRef(c.Request.Context(), database.GetOrganizationMembershipByProjectRefParams{
		ProjectRef: branch.ParentProjectRef,
		AccountID:  account.ID,
	})
	if err != nil {
		c.JSON(http.StatusForbidden, gin.H{"error": "Forbidden: not a member of this project's organization"})
		return zero, false
	}

	for _, role := range roles {
		if strings.EqualFold(membership.Role, role) {
			return branch, true
		}
	}

	c.JSON(http.StatusForbidden, gin.H{"error": "Forbidden: insufficient role permissions"})
	return zero, false
}

// runBranchOperation transitions a branch through an async brancher operation,
// updating its status on completion.
func (a *Api) runBranchOperation(branch database.ProjectBranch, pendingStatus string, op func(ctx context.Context) error) {
	ctx := context.Background()

	if err := a.queries.UpdateProjectBranchStatus(ctx, branch.ID, pendingStatus); err != nil {
		a.logger.Error(fmt.Sprintf("Failed to update branch %d status: %v", branch.ID, err))
	}

	go func() {
		opCtx, cancel := context.WithTimeout(context.Background(), branchOperationTimeout)
		defer cancel()

		status := "RUNNING"
		if err := op(opCtx); err != nil {
			a.logger.Error(fmt.Sprintf("Branch operation %s failed for branch %s (%s): %v",
				pendingStatus, branch.BranchName, branch.ParentProjectRef, err))
			status = "FAILED"
		}
		if err := a.queries.UpdateProjectBranchStatus(context.Background(), branch.ID, status); err != nil {
			a.logger.Error(fmt.Sprintf("Failed to update branch %d status: %v", branch.ID, err))
		}
	}()
}

// GET /v1/projects/:ref/branches
func (a *Api) getV1ProjectBranches(c *gin.Context) {
	projectRef := c.Param("ref")

	branches, err := a.queries.GetProjectBranches(c.Request.Context(), projectRef)
	if err != nil {
		a.logger.Error(fmt.Sprintf("Failed to list branches for %s: %v", projectRef, err))
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Internal Server Error"})
		return
	}

	response := make([]BranchResponse, 0, len(branches))
	for _, b := range branches {
		response = append(response, branchResponseFromRow(b))
	}
	c.JSON(http.StatusOK, response)
}

// POST /v1/projects/:ref/branches
func (a *Api) postV1ProjectBranches(c *gin.Context) {
	projectRef := c.Param("ref")

	if a.brancher == nil {
		c.JSON(http.StatusServiceUnavailable, gin.H{"error": "Branching is not available (provisioner disabled)"})
		return
	}

	var body CreateBranchBody
	if err := c.ShouldBindJSON(&body); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "branch_name is required"})
		return
	}
	if err := provisioner.ValidateBranchName(body.BranchName); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	if _, err := a.queries.GetProjectByRef(c.Request.Context(), projectRef); err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "Project not found"})
		return
	}

	branch, err := a.queries.CreateProjectBranch(c.Request.Context(), database.CreateProjectBranchParams{
		ParentProjectRef: projectRef,
		BranchName:       body.BranchName,
		DbName:           provisioner.BranchDBName(body.BranchName),
		Status:           "CREATING",
		GitBranch:        body.GitBranch,
	})
	if err != nil {
		c.JSON(http.StatusConflict, gin.H{"error": "A branch with this name already exists"})
		return
	}

	a.runBranchOperation(branch, "CREATING", func(ctx context.Context) error {
		return a.brancher.CreateBranch(ctx, projectRef, branch.DbName)
	})

	c.JSON(http.StatusCreated, branchResponseFromRow(branch))
}

// GET /v1/branches/:branch_id
func (a *Api) getV1Branch(c *gin.Context) {
	branch, ok := a.requireBranchRole(c, "owner", "admin", "member", "viewer")
	if !ok {
		return
	}
	c.JSON(http.StatusOK, branchResponseFromRow(branch))
}

// DELETE /v1/branches/:branch_id
func (a *Api) deleteV1Branch(c *gin.Context) {
	branch, ok := a.requireBranchRole(c, "owner", "admin")
	if !ok {
		return
	}
	if a.brancher == nil {
		c.JSON(http.StatusServiceUnavailable, gin.H{"error": "Branching is not available (provisioner disabled)"})
		return
	}

	deleteCtx, cancel := context.WithTimeout(c.Request.Context(), 60*time.Second)
	defer cancel()
	if err := a.brancher.DeleteBranch(deleteCtx, branch.ParentProjectRef, branch.DbName); err != nil {
		a.logger.Error(fmt.Sprintf("Failed to drop branch database %s: %v", branch.DbName, err))
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to delete branch database"})
		return
	}

	if err := a.queries.DeleteProjectBranch(c.Request.Context(), branch.ID); err != nil {
		a.logger.Error(fmt.Sprintf("Failed to delete branch record %d: %v", branch.ID, err))
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Internal Server Error"})
		return
	}

	c.JSON(http.StatusOK, gin.H{"message": "Branch deleted"})
}

// POST /v1/branches/:branch_id/merge
func (a *Api) postV1BranchMerge(c *gin.Context) {
	branch, ok := a.requireBranchRole(c, "owner", "admin", "member")
	if !ok {
		return
	}
	if a.brancher == nil {
		c.JSON(http.StatusServiceUnavailable, gin.H{"error": "Branching is not available (provisioner disabled)"})
		return
	}

	a.runBranchOperation(branch, "MERGING", func(ctx context.Context) error {
		return a.brancher.MergeBranch(ctx, branch.ParentProjectRef, branch.DbName)
	})

	c.JSON(http.StatusAccepted, gin.H{"message": "Merge started", "branch_id": branch.ID})
}

// POST /v1/branches/:branch_id/reset
func (a *Api) postV1BranchReset(c *gin.Context) {
	branch, ok := a.requireBranchRole(c, "owner", "admin", "member")
	if !ok {
		return
	}
	if a.brancher == nil {
		c.JSON(http.StatusServiceUnavailable, gin.H{"error": "Branching is not available (provisioner disabled)"})
		return
	}

	a.runBranchOperation(branch, "RESETTING", func(ctx context.Context) error {
		return a.brancher.ResetBranch(ctx, branch.ParentProjectRef, branch.DbName)
	})

	c.JSON(http.StatusAccepted, gin.H{"message": "Reset started", "branch_id": branch.ID})
}

// POST /v1/branches/:branch_id/rebase
func (a *Api) postV1BranchRebase(c *gin.Context) {
	branch, ok := a.requireBranchRole(c, "owner", "admin", "member")
	if !ok {
		return
	}
	if a.brancher == nil {
		c.JSON(http.StatusServiceUnavailable, gin.H{"error": "Branching is not available (provisioner disabled)"})
		return
	}

	a.runBranchOperation(branch, "REBASING", func(ctx context.Context) error {
		return a.brancher.RebaseBranch(ctx, branch.ParentProjectRef, branch.DbName)
	})

	c.JSON(http.StatusAccepted, gin.H{"message": "Rebase started", "branch_id": branch.ID})
}
