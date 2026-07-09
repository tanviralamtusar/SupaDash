package api

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strconv"

	"github.com/modelcontextprotocol/go-sdk/mcp"
	"supadash/database"
	"supadash/provisioner"
)

// mcpBranchById loads a branch by id and verifies caller access + role.
func (a *Api) mcpBranchById(ctx context.Context, req *mcp.CallToolRequest, branchId, scopedRef string, roles ...string) (database.ProjectBranch, error) {
	var zero database.ProjectBranch

	id, err := strconv.Atoi(branchId)
	if err != nil {
		return zero, fmt.Errorf("invalid branch_id %q", branchId)
	}
	branch, err := a.queries.GetProjectBranch(ctx, int32(id))
	if err != nil {
		return zero, errors.New("branch not found")
	}
	if scopedRef != "" && branch.ParentProjectRef != scopedRef {
		return zero, fmt.Errorf("this server is scoped to project %q", scopedRef)
	}
	if _, err := a.mcpProjectWithRole(ctx, req, branch.ParentProjectRef, "", roles...); err != nil {
		return zero, err
	}
	return branch, nil
}

func mcpBranchOut(b database.ProjectBranch) map[string]any {
	return map[string]any{
		"id":                 fmt.Sprintf("%d", b.ID),
		"name":               b.BranchName,
		"parent_project_ref": b.ParentProjectRef,
		"db_name":            b.DbName,
		"status":             b.Status,
		"created_at":         b.CreatedAt.Time.Format("2006-01-02T15:04:05Z"),
	}
}

func (a *Api) registerBranchingTools(server *mcp.Server, scopedRef string) {
	type createBranchIn struct {
		ProjectId string `json:"project_id,omitempty" jsonschema:"the project ref (omit when the server is scoped to a project)"`
		Name      string `json:"name" jsonschema:"the name of the branch to create"`
	}
	mcp.AddTool(server, &mcp.Tool{
		Name:        "create_branch",
		Description: "Creates a development branch: a copy of the project's database that can be changed and merged back.",
	}, func(ctx context.Context, req *mcp.CallToolRequest, in createBranchIn) (*mcp.CallToolResult, any, error) {
		project, err := a.mcpProjectWithRole(ctx, req, in.ProjectId, scopedRef, "owner", "admin", "member")
		if err != nil {
			return nil, nil, err
		}
		if a.brancher == nil {
			return nil, nil, errors.New("branching is not available (provisioner disabled)")
		}
		if err := provisioner.ValidateBranchName(in.Name); err != nil {
			return nil, nil, err
		}

		branch, err := a.queries.CreateProjectBranch(ctx, database.CreateProjectBranchParams{
			ParentProjectRef: project.ProjectRef,
			BranchName:       in.Name,
			DbName:           provisioner.BranchDBName(in.Name),
			Status:           "CREATING",
		})
		if err != nil {
			return nil, nil, errors.New("a branch with this name already exists")
		}

		a.runBranchOperation(branch, "CREATING", func(opCtx context.Context) error {
			return a.brancher.CreateBranch(opCtx, project.ProjectRef, branch.DbName)
		})

		return nil, mcpBranchOut(branch), nil
	})

	mcp.AddTool(server, &mcp.Tool{
		Name:        "list_branches",
		Description: "Lists the development branches of a project.",
	}, func(ctx context.Context, req *mcp.CallToolRequest, in mcpOptionalProjectIn) (*mcp.CallToolResult, any, error) {
		project, err := a.mcpProject(ctx, req, in.ProjectId, scopedRef)
		if err != nil {
			return nil, nil, err
		}
		branches, err := a.queries.GetProjectBranches(ctx, project.ProjectRef)
		if err != nil {
			return nil, nil, err
		}
		out := make([]map[string]any, 0, len(branches))
		for _, b := range branches {
			out = append(out, mcpBranchOut(b))
		}
		return nil, map[string]any{"branches": out}, nil
	})

	type branchIdIn struct {
		BranchId string `json:"branch_id" jsonschema:"the id of the branch"`
	}
	mcp.AddTool(server, &mcp.Tool{
		Name:        "delete_branch",
		Description: "Deletes a development branch and its database.",
	}, func(ctx context.Context, req *mcp.CallToolRequest, in branchIdIn) (*mcp.CallToolResult, any, error) {
		branch, err := a.mcpBranchById(ctx, req, in.BranchId, scopedRef, "owner", "admin")
		if err != nil {
			return nil, nil, err
		}
		if a.brancher == nil {
			return nil, nil, errors.New("branching is not available (provisioner disabled)")
		}
		if err := a.brancher.DeleteBranch(ctx, branch.ParentProjectRef, branch.DbName); err != nil {
			return nil, nil, err
		}
		if err := a.queries.DeleteProjectBranch(ctx, branch.ID); err != nil {
			return nil, nil, err
		}
		return textResult(fmt.Sprintf("Branch %s deleted", branch.BranchName)), nil, nil
	})

	mcp.AddTool(server, &mcp.Tool{
		Name:        "merge_branch",
		Description: "Merges migrations from a development branch onto its parent project (asynchronous — poll list_branches for status).",
	}, func(ctx context.Context, req *mcp.CallToolRequest, in branchIdIn) (*mcp.CallToolResult, any, error) {
		branch, err := a.mcpBranchById(ctx, req, in.BranchId, scopedRef, "owner", "admin", "member")
		if err != nil {
			return nil, nil, err
		}
		if a.brancher == nil {
			return nil, nil, errors.New("branching is not available (provisioner disabled)")
		}
		a.runBranchOperation(branch, "MERGING", func(opCtx context.Context) error {
			return a.brancher.MergeBranch(opCtx, branch.ParentProjectRef, branch.DbName)
		})
		return textResult(fmt.Sprintf("Merge of branch %s started", branch.BranchName)), nil, nil
	})

	mcp.AddTool(server, &mcp.Tool{
		Name:        "reset_branch",
		Description: "Resets a development branch to a fresh copy of the parent project's database (asynchronous).",
	}, func(ctx context.Context, req *mcp.CallToolRequest, in branchIdIn) (*mcp.CallToolResult, any, error) {
		branch, err := a.mcpBranchById(ctx, req, in.BranchId, scopedRef, "owner", "admin", "member")
		if err != nil {
			return nil, nil, err
		}
		if a.brancher == nil {
			return nil, nil, errors.New("branching is not available (provisioner disabled)")
		}
		a.runBranchOperation(branch, "RESETTING", func(opCtx context.Context) error {
			return a.brancher.ResetBranch(opCtx, branch.ParentProjectRef, branch.DbName)
		})
		return textResult(fmt.Sprintf("Reset of branch %s started", branch.BranchName)), nil, nil
	})

	mcp.AddTool(server, &mcp.Tool{
		Name:        "rebase_branch",
		Description: "Rebases a development branch on the parent project's current state, re-applying the branch's own migrations (asynchronous).",
	}, func(ctx context.Context, req *mcp.CallToolRequest, in branchIdIn) (*mcp.CallToolResult, any, error) {
		branch, err := a.mcpBranchById(ctx, req, in.BranchId, scopedRef, "owner", "admin", "member")
		if err != nil {
			return nil, nil, err
		}
		if a.brancher == nil {
			return nil, nil, errors.New("branching is not available (provisioner disabled)")
		}
		a.runBranchOperation(branch, "REBASING", func(opCtx context.Context) error {
			return a.brancher.RebaseBranch(opCtx, branch.ParentProjectRef, branch.DbName)
		})
		return textResult(fmt.Sprintf("Rebase of branch %s started", branch.BranchName)), nil, nil
	})
}

func (a *Api) registerFunctionTools(server *mcp.Server, scopedRef string) {
	mcp.AddTool(server, &mcp.Tool{
		Name:        "list_edge_functions",
		Description: "Lists the Edge Functions deployed to a project.",
	}, func(ctx context.Context, req *mcp.CallToolRequest, in mcpOptionalProjectIn) (*mcp.CallToolResult, any, error) {
		project, err := a.mcpProject(ctx, req, in.ProjectId, scopedRef)
		if err != nil {
			return nil, nil, err
		}
		functions, err := a.queries.GetEdgeFunctions(ctx, project.ProjectRef)
		if err != nil {
			return nil, nil, err
		}
		out := make([]EdgeFunctionResponse, 0, len(functions))
		for _, f := range functions {
			out = append(out, edgeFunctionResponseFromRow(f))
		}
		return nil, map[string]any{"functions": out}, nil
	})

	type getFunctionIn struct {
		ProjectId string `json:"project_id,omitempty" jsonschema:"the project ref (omit when the server is scoped to a project)"`
		Slug      string `json:"function_slug" jsonschema:"the slug of the function"`
	}
	mcp.AddTool(server, &mcp.Tool{
		Name:        "get_edge_function",
		Description: "Gets an Edge Function's metadata and source code.",
	}, func(ctx context.Context, req *mcp.CallToolRequest, in getFunctionIn) (*mcp.CallToolResult, any, error) {
		project, err := a.mcpProject(ctx, req, in.ProjectId, scopedRef)
		if err != nil {
			return nil, nil, err
		}
		function, err := a.queries.GetEdgeFunction(ctx, project.ProjectRef, in.Slug)
		if err != nil {
			return nil, nil, errors.New("function not found")
		}

		out := map[string]any{"function": edgeFunctionResponseFromRow(function)}
		if fp := a.getFunctionProvisioner(); fp != nil {
			if content, err := fp.ReadFunctionFile(project.ProjectRef, function.Slug, function.EntrypointPath); err == nil {
				out["source"] = content
			}
		}
		return nil, out, nil
	})

	type deployFunctionIn struct {
		ProjectId      string             `json:"project_id,omitempty" jsonschema:"the project ref (omit when the server is scoped to a project)"`
		Slug           string             `json:"slug" jsonschema:"the slug of the function"`
		Name           string             `json:"name,omitempty" jsonschema:"optional display name (defaults to the slug)"`
		EntrypointPath string             `json:"entrypoint_path,omitempty" jsonschema:"the entrypoint file (default index.ts)"`
		Files          []EdgeFunctionFile `json:"files" jsonschema:"the source files to deploy"`
	}
	mcp.AddTool(server, &mcp.Tool{
		Name:        "deploy_edge_function",
		Description: "Deploys (creates or updates) an Edge Function to a project.",
	}, func(ctx context.Context, req *mcp.CallToolRequest, in deployFunctionIn) (*mcp.CallToolResult, any, error) {
		project, err := a.mcpProjectWithRole(ctx, req, in.ProjectId, scopedRef, "owner", "admin", "member")
		if err != nil {
			return nil, nil, err
		}
		fp := a.getFunctionProvisioner()
		if fp == nil {
			return nil, nil, errors.New("edge functions are not available (provisioner disabled)")
		}
		if err := provisioner.ValidateFunctionSlug(in.Slug); err != nil {
			return nil, nil, err
		}
		if len(in.Files) == 0 {
			return nil, nil, errors.New("files is required")
		}
		if in.Name == "" {
			in.Name = in.Slug
		}
		if in.EntrypointPath == "" {
			in.EntrypointPath = "index.ts"
		}

		files := make(map[string]string, len(in.Files))
		for _, f := range in.Files {
			files[f.Name] = f.Content
		}
		if _, ok := files[in.EntrypointPath]; !ok {
			return nil, nil, fmt.Errorf("entrypoint %q missing from files", in.EntrypointPath)
		}

		if err := fp.WriteFunction(project.ProjectRef, in.Slug, files); err != nil {
			return nil, nil, err
		}
		function, err := a.queries.UpsertEdgeFunction(ctx, database.UpsertEdgeFunctionParams{
			ProjectRef:     project.ProjectRef,
			Slug:           in.Slug,
			Name:           in.Name,
			Status:         "ACTIVE",
			VerifyJwt:      true,
			EntrypointPath: in.EntrypointPath,
		})
		if err != nil {
			return nil, nil, err
		}
		if err := fp.RestartService(ctx, project.ProjectRef, "edge-functions"); err != nil {
			a.logger.Warn(fmt.Sprintf("Failed to restart edge runtime for %s: %v", project.ProjectRef, err))
		}
		return nil, edgeFunctionResponseFromRow(function), nil
	})
}

func (a *Api) registerStorageTools(server *mcp.Server, scopedRef string) {
	mcp.AddTool(server, &mcp.Tool{
		Name:        "list_storage_buckets",
		Description: "Lists the storage buckets of a project.",
	}, func(ctx context.Context, req *mcp.CallToolRequest, in mcpOptionalProjectIn) (*mcp.CallToolResult, any, error) {
		project, err := a.mcpProject(ctx, req, in.ProjectId, scopedRef)
		if err != nil {
			return nil, nil, err
		}
		data, err := a.projectKongRequest(ctx, project, "GET", "/storage/v1/bucket", nil)
		if err != nil {
			return nil, nil, err
		}
		return textResult(string(data)), nil, nil
	})

	type updateBucketIn struct {
		ProjectId        string   `json:"project_id,omitempty" jsonschema:"the project ref (omit when the server is scoped to a project)"`
		BucketId         string   `json:"bucket_id" jsonschema:"the id of the bucket to update"`
		Public           *bool    `json:"public,omitempty" jsonschema:"whether the bucket is publicly readable"`
		FileSizeLimit    *int64   `json:"file_size_limit,omitempty" jsonschema:"maximum file size in bytes"`
		AllowedMimeTypes []string `json:"allowed_mime_types,omitempty" jsonschema:"allowed MIME types for uploads"`
	}
	mcp.AddTool(server, &mcp.Tool{
		Name:        "update_storage_bucket",
		Description: "Updates a storage bucket's configuration (visibility, file size limit, allowed MIME types).",
	}, func(ctx context.Context, req *mcp.CallToolRequest, in updateBucketIn) (*mcp.CallToolResult, any, error) {
		project, err := a.mcpProjectWithRole(ctx, req, in.ProjectId, scopedRef, "owner", "admin", "member")
		if err != nil {
			return nil, nil, err
		}

		payload := map[string]any{}
		if in.Public != nil {
			payload["public"] = *in.Public
		}
		if in.FileSizeLimit != nil {
			payload["file_size_limit"] = *in.FileSizeLimit
		}
		if in.AllowedMimeTypes != nil {
			payload["allowed_mime_types"] = in.AllowedMimeTypes
		}
		if len(payload) == 0 {
			return nil, nil, errors.New("nothing to update: provide public, file_size_limit or allowed_mime_types")
		}

		body, err := json.Marshal(payload)
		if err != nil {
			return nil, nil, err
		}
		data, err := a.projectKongRequest(ctx, project, "PUT", "/storage/v1/bucket/"+in.BucketId, bytes.NewReader(body))
		if err != nil {
			return nil, nil, err
		}
		return textResult(string(data)), nil, nil
	})
}
