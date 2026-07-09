package api

import (
	"context"
	"errors"
	"fmt"
	"strings"

	"github.com/modelcontextprotocol/go-sdk/mcp"
	"supadash/database"
)

// mcpProjectWithRole is mcpProject plus a role requirement for mutating tools.
func (a *Api) mcpProjectWithRole(ctx context.Context, req *mcp.CallToolRequest, refArg, scopedRef string, roles ...string) (database.Project, error) {
	var zero database.Project

	project, err := a.mcpProject(ctx, req, refArg, scopedRef)
	if err != nil {
		return zero, err
	}

	account, err := a.mcpAccount(ctx, req)
	if err != nil {
		return zero, err
	}
	membership, err := a.queries.GetOrganizationMembershipByProjectRef(ctx, database.GetOrganizationMembershipByProjectRefParams{
		ProjectRef: project.ProjectRef,
		AccountID:  account.ID,
	})
	if err != nil {
		return zero, errors.New("access denied")
	}
	for _, role := range roles {
		if strings.EqualFold(membership.Role, role) {
			return project, nil
		}
	}
	return zero, fmt.Errorf("access denied: requires one of roles %v", roles)
}

type mcpProjectSummary struct {
	Id             string `json:"id"`
	Name           string `json:"name"`
	OrganizationId int32  `json:"organization_id"`
	Region         string `json:"region"`
	Status         string `json:"status"`
	CreatedAt      string `json:"created_at"`
}

func mcpProjectSummaryFromRow(p database.Project) mcpProjectSummary {
	return mcpProjectSummary{
		Id:             p.ProjectRef,
		Name:           p.ProjectName,
		OrganizationId: p.OrganizationID,
		Region:         p.Region,
		Status:         p.Status,
		CreatedAt:      p.CreatedAt.Time.Format("2006-01-02T15:04:05Z"),
	}
}

func (a *Api) registerAccountTools(server *mcp.Server) {
	mcp.AddTool(server, &mcp.Tool{
		Name:        "list_projects",
		Description: "Lists all Supabase projects the authenticated user can access.",
	}, func(ctx context.Context, req *mcp.CallToolRequest, _ any) (*mcp.CallToolResult, any, error) {
		account, err := a.mcpAccount(ctx, req)
		if err != nil {
			return nil, nil, err
		}
		projects, err := a.queries.GetProjectsForAccountId(ctx, account.ID)
		if err != nil {
			return nil, nil, err
		}
		out := make([]mcpProjectSummary, 0, len(projects))
		for _, p := range projects {
			out = append(out, mcpProjectSummaryFromRow(p))
		}
		return nil, out, nil
	})

	type getProjectIn struct {
		Id string `json:"id" jsonschema:"the project ref"`
	}
	mcp.AddTool(server, &mcp.Tool{
		Name:        "get_project",
		Description: "Gets details for a Supabase project.",
	}, func(ctx context.Context, req *mcp.CallToolRequest, in getProjectIn) (*mcp.CallToolResult, any, error) {
		project, err := a.mcpProject(ctx, req, in.Id, "")
		if err != nil {
			return nil, nil, err
		}
		return nil, mcpProjectSummaryFromRow(project), nil
	})

	type createProjectIn struct {
		Name           string `json:"name" jsonschema:"the name of the project"`
		OrganizationId string `json:"organization_id" jsonschema:"the organization slug to create the project in"`
		DbPass         string `json:"db_pass,omitempty" jsonschema:"optional database password (generated when omitted)"`
		InstanceSize   string `json:"desired_instance_size,omitempty" jsonschema:"optional instance size: micro, small, medium or large"`
	}
	mcp.AddTool(server, &mcp.Tool{
		Name:        "create_project",
		Description: "Creates a new Supabase project. Provisioning continues asynchronously; poll get_project for status.",
	}, func(ctx context.Context, req *mcp.CallToolRequest, in createProjectIn) (*mcp.CallToolResult, any, error) {
		account, err := a.mcpAccount(ctx, req)
		if err != nil {
			return nil, nil, err
		}
		membership, err := a.queries.GetOrganizationMembershipBySlug(ctx, database.GetOrganizationMembershipBySlugParams{
			Slug:      in.OrganizationId,
			AccountID: account.ID,
		})
		if err != nil {
			return nil, nil, fmt.Errorf("access denied: not a member of organization %q", in.OrganizationId)
		}
		if !strings.EqualFold(membership.Role, "owner") && !strings.EqualFold(membership.Role, "admin") {
			return nil, nil, errors.New("access denied: requires owner or admin role")
		}
		if strings.TrimSpace(in.Name) == "" {
			return nil, nil, errors.New("name is required")
		}

		org, err := a.queries.GetOrganizationBySlug(ctx, in.OrganizationId)
		if err != nil {
			return nil, nil, fmt.Errorf("organization %q not found", in.OrganizationId)
		}

		proj, _, err := a.createProjectCore(ctx, org, in.Name, in.DbPass, in.InstanceSize)
		if err != nil {
			return nil, nil, err
		}
		return nil, mcpProjectSummaryFromRow(proj), nil
	})

	type projectIdIn struct {
		ProjectId string `json:"project_id" jsonschema:"the project ref"`
	}
	mcp.AddTool(server, &mcp.Tool{
		Name:        "pause_project",
		Description: "Pauses a Supabase project (stops its containers without deleting data).",
	}, func(ctx context.Context, req *mcp.CallToolRequest, in projectIdIn) (*mcp.CallToolResult, any, error) {
		project, err := a.mcpProjectWithRole(ctx, req, in.ProjectId, "", "owner", "admin")
		if err != nil {
			return nil, nil, err
		}
		if project.Status == "PAUSED" {
			return nil, nil, errors.New("project is already paused")
		}
		if a.provisioner == nil {
			return nil, nil, errors.New("provisioner is not available")
		}

		if err := a.provisioner.PauseProject(ctx, project.ProjectRef); err != nil {
			return nil, nil, fmt.Errorf("failed to pause project: %w", err)
		}
		if _, err := a.queries.UpdateProjectStatus(ctx, database.UpdateProjectStatusParams{
			ProjectRef: project.ProjectRef,
			Status:     "PAUSED",
		}); err != nil {
			a.logger.Error(fmt.Sprintf("Failed to update status to PAUSED: %v", err))
		}
		a.wsHub.BroadcastStatus(project.ProjectRef, "PAUSED")
		return textResult(fmt.Sprintf("Project %s paused", project.ProjectRef)), nil, nil
	})

	mcp.AddTool(server, &mcp.Tool{
		Name:        "restore_project",
		Description: "Restores (resumes) a paused Supabase project.",
	}, func(ctx context.Context, req *mcp.CallToolRequest, in projectIdIn) (*mcp.CallToolResult, any, error) {
		project, err := a.mcpProjectWithRole(ctx, req, in.ProjectId, "", "owner", "admin")
		if err != nil {
			return nil, nil, err
		}
		if a.provisioner == nil {
			return nil, nil, errors.New("provisioner is not available")
		}

		if err := a.provisioner.ResumeProject(ctx, project.ProjectRef); err != nil {
			return nil, nil, fmt.Errorf("failed to restore project: %w", err)
		}
		if _, err := a.queries.UpdateProjectStatus(ctx, database.UpdateProjectStatusParams{
			ProjectRef: project.ProjectRef,
			Status:     "ACTIVE_HEALTHY",
		}); err != nil {
			a.logger.Error(fmt.Sprintf("Failed to update status to ACTIVE: %v", err))
		}
		a.wsHub.BroadcastStatus(project.ProjectRef, "ACTIVE_HEALTHY")
		return textResult(fmt.Sprintf("Project %s restored", project.ProjectRef)), nil, nil
	})

	mcp.AddTool(server, &mcp.Tool{
		Name:        "list_organizations",
		Description: "Lists all organizations the authenticated user belongs to.",
	}, func(ctx context.Context, req *mcp.CallToolRequest, _ any) (*mcp.CallToolResult, any, error) {
		account, err := a.mcpAccount(ctx, req)
		if err != nil {
			return nil, nil, err
		}
		orgs, err := a.queries.GetOrganizationsForAccountId(ctx, account.ID)
		if err != nil {
			return nil, nil, err
		}
		type orgOut struct {
			Id   string `json:"id"`
			Name string `json:"name"`
		}
		out := make([]orgOut, 0, len(orgs))
		for _, o := range orgs {
			out = append(out, orgOut{Id: o.Slug, Name: o.Name})
		}
		return nil, out, nil
	})

	type getOrgIn struct {
		Id string `json:"id" jsonschema:"the organization slug"`
	}
	mcp.AddTool(server, &mcp.Tool{
		Name:        "get_organization",
		Description: "Gets details for an organization.",
	}, func(ctx context.Context, req *mcp.CallToolRequest, in getOrgIn) (*mcp.CallToolResult, any, error) {
		account, err := a.mcpAccount(ctx, req)
		if err != nil {
			return nil, nil, err
		}
		if _, err := a.queries.GetOrganizationMembershipBySlug(ctx, database.GetOrganizationMembershipBySlugParams{
			Slug:      in.Id,
			AccountID: account.ID,
		}); err != nil {
			return nil, nil, fmt.Errorf("access denied: not a member of organization %q", in.Id)
		}
		org, err := a.queries.GetOrganizationBySlug(ctx, in.Id)
		if err != nil {
			return nil, nil, err
		}
		return nil, map[string]any{
			"id":         org.Slug,
			"name":       org.Name,
			"created_at": org.CreatedAt.Time.Format("2006-01-02T15:04:05Z"),
		}, nil
	})
}

// mcpOptionalProjectIn is the input for project tools: project_id is required
// on unscoped servers and forbidden/ignored on scoped ones.
type mcpOptionalProjectIn struct {
	ProjectId string `json:"project_id,omitempty" jsonschema:"the project ref (omit when the server is scoped to a project)"`
}

func (a *Api) registerDevelopmentTools(server *mcp.Server, scopedRef string) {
	mcp.AddTool(server, &mcp.Tool{
		Name:        "get_project_url",
		Description: "Gets the API URL for a project.",
	}, func(ctx context.Context, req *mcp.CallToolRequest, in mcpOptionalProjectIn) (*mcp.CallToolResult, any, error) {
		project, err := a.mcpProject(ctx, req, in.ProjectId, scopedRef)
		if err != nil {
			return nil, nil, err
		}
		url := fmt.Sprintf("https://%s.%s", project.ProjectRef, a.config.Domain.Base)
		return textResult(url), nil, nil
	})

	mcp.AddTool(server, &mcp.Tool{
		Name:        "get_anon_key",
		Description: "Gets the anonymous (public) API key for a project.",
	}, func(ctx context.Context, req *mcp.CallToolRequest, in mcpOptionalProjectIn) (*mcp.CallToolResult, any, error) {
		project, err := a.mcpProject(ctx, req, in.ProjectId, scopedRef)
		if err != nil {
			return nil, nil, err
		}
		if !project.AnonKey.Valid {
			return nil, nil, errors.New("project has no anon key yet (still provisioning?)")
		}
		return textResult(project.AnonKey.String), nil, nil
	})

	mcp.AddTool(server, &mcp.Tool{
		Name:        "generate_typescript_types",
		Description: "Generates TypeScript types from the project's database schema.",
	}, func(ctx context.Context, req *mcp.CallToolRequest, in mcpOptionalProjectIn) (*mcp.CallToolResult, any, error) {
		project, err := a.mcpProject(ctx, req, in.ProjectId, scopedRef)
		if err != nil {
			return nil, nil, err
		}
		data, err := a.projectKongRequest(ctx, project, "GET", "/pg/generators/typescript?included_schemas=public", nil)
		if err != nil {
			return nil, nil, err
		}
		return textResult(string(data)), nil, nil
	})
}
