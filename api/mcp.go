package api

import (
	"context"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/modelcontextprotocol/go-sdk/auth"
	"github.com/modelcontextprotocol/go-sdk/mcp"
	"supadash/database"
)

// The MCP server exposes SupaDash management operations to AI clients over
// streamable HTTP (with SSE), authenticated by personal access tokens. Tool
// names and shapes follow the official Supabase MCP server so existing client
// configurations work against SupaDash unchanged.
//
// Scoping: connecting to /mcp exposes account-level tools plus project tools
// (which take a project_ref argument). Connecting to /mcp?project_ref=<ref>
// scopes the session to one project and hides account-level tools.

// MCPHandler returns the fully authenticated /mcp HTTP handler.
func (a *Api) MCPHandler() http.Handler {
	verifier := func(ctx context.Context, token string, _ *http.Request) (*auth.TokenInfo, error) {
		pat, err := a.queries.GetAccessTokenByHash(ctx, hashAccessToken(token))
		if err != nil {
			return nil, auth.ErrInvalidToken
		}
		if err := a.queries.TouchAccessToken(ctx, pat.ID); err != nil {
			a.logger.Warn("Failed to touch access token", "id", pat.ID, "error", err.Error())
		}
		return &auth.TokenInfo{
			UserID: strconv.Itoa(int(pat.AccountID)),
			// PATs do not expire; give the verifier a far-future expiry
			Expiration: time.Now().Add(24 * time.Hour),
		}, nil
	}

	streamable := mcp.NewStreamableHTTPHandler(func(r *http.Request) *mcp.Server {
		return a.buildMCPServer(r.URL.Query().Get("project_ref"))
	}, nil)

	return auth.RequireBearerToken(verifier, nil)(streamable)
}

// buildMCPServer assembles the tool set for a session. scopedRef == ""
// means account-level (all tools); otherwise the session is scoped to a
// single project and account-level tools are unavailable.
func (a *Api) buildMCPServer(scopedRef string) *mcp.Server {
	server := mcp.NewServer(&mcp.Implementation{
		Name:    "supadash",
		Title:   "SupaDash MCP Server",
		Version: "1.0.0",
	}, nil)

	if scopedRef == "" {
		a.registerAccountTools(server)
	}
	a.registerDatabaseTools(server, scopedRef)
	a.registerDebuggingTools(server, scopedRef)
	a.registerDevelopmentTools(server, scopedRef)
	a.registerBranchingTools(server, scopedRef)
	a.registerFunctionTools(server, scopedRef)
	a.registerStorageTools(server, scopedRef)

	return server
}

// --- Shared helpers ---

var errMCPUnauthenticated = errors.New("unauthenticated: missing or invalid access token")

// mcpAccount resolves the calling account from the request's bearer token.
func (a *Api) mcpAccount(ctx context.Context, req *mcp.CallToolRequest) (database.Account, error) {
	var zero database.Account
	extra := req.GetExtra()
	if extra == nil || extra.TokenInfo == nil || extra.TokenInfo.UserID == "" {
		return zero, errMCPUnauthenticated
	}
	id, err := strconv.Atoi(extra.TokenInfo.UserID)
	if err != nil {
		return zero, errMCPUnauthenticated
	}
	return a.queries.GetAccountByID(ctx, int32(id))
}

// mcpProject resolves the target project for a tool call, honoring session
// scoping, and verifies the caller belongs to the project's organization.
func (a *Api) mcpProject(ctx context.Context, req *mcp.CallToolRequest, refArg, scopedRef string) (database.Project, error) {
	var zero database.Project

	ref := scopedRef
	if ref == "" {
		ref = refArg
	}
	if ref == "" {
		return zero, errors.New("project_ref is required")
	}
	if scopedRef != "" && refArg != "" && refArg != scopedRef {
		return zero, fmt.Errorf("this server is scoped to project %q", scopedRef)
	}

	account, err := a.mcpAccount(ctx, req)
	if err != nil {
		return zero, err
	}

	if _, err := a.queries.GetOrganizationMembershipByProjectRef(ctx, database.GetOrganizationMembershipByProjectRefParams{
		ProjectRef: ref,
		AccountID:  account.ID,
	}); err != nil {
		return zero, fmt.Errorf("access denied: not a member of project %q's organization", ref)
	}

	return a.queries.GetProjectByRef(ctx, ref)
}

// mcpResolveDB maps an optional branch name to the database psql should
// target. Empty branch means the main ("postgres") database.
func (a *Api) mcpResolveDB(ctx context.Context, projectRef, branch string) (string, error) {
	if branch == "" {
		return "postgres", nil
	}
	branches, err := a.queries.GetProjectBranches(ctx, projectRef)
	if err != nil {
		return "", err
	}
	for _, b := range branches {
		if b.BranchName == branch {
			return b.DbName, nil
		}
	}
	return "", fmt.Errorf("branch %q not found", branch)
}

// mcpExecSQL runs SQL inside the project's Postgres container and returns
// CSV-formatted output.
func (a *Api) mcpExecSQL(ctx context.Context, projectRef, dbName, sql string) (string, error) {
	if a.provisioner == nil {
		return "", errors.New("provisioner is not available")
	}
	return a.provisioner.ExecuteCommand(ctx, projectRef, "db",
		[]string{"psql", "-U", "postgres", "-d", dbName, "-v", "ON_ERROR_STOP=1", "--csv", "-c", sql})
}

// projectKongRequest performs an HTTP request against a project's Kong
// gateway (via the configured project host) with the project's service-role
// credentials.
func (a *Api) projectKongRequest(ctx context.Context, project database.Project, method, path string, body io.Reader) ([]byte, error) {
	if !project.KongHttpPort.Valid {
		return nil, errors.New("project is not fully provisioned yet")
	}
	if !project.ServiceRoleKey.Valid {
		return nil, errors.New("project has no service role key")
	}

	url := fmt.Sprintf("http://%s:%d%s", a.config.Provisioning.ProjectHost, project.KongHttpPort.Int32, path)
	httpReq, err := http.NewRequestWithContext(ctx, method, url, body)
	if err != nil {
		return nil, err
	}
	httpReq.Header.Set("apikey", project.ServiceRoleKey.String)
	httpReq.Header.Set("Authorization", "Bearer "+project.ServiceRoleKey.String)
	if body != nil {
		httpReq.Header.Set("Content-Type", "application/json")
	}

	client := &http.Client{Timeout: 30 * time.Second}
	resp, err := client.Do(httpReq)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	data, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, err
	}
	if resp.StatusCode >= 400 {
		return nil, fmt.Errorf("%s %s returned %d: %s", method, path, resp.StatusCode, strings.TrimSpace(string(data)))
	}
	return data, nil
}

// textResult wraps plain text in a CallToolResult.
func textResult(text string) *mcp.CallToolResult {
	return &mcp.CallToolResult{
		Content: []mcp.Content{&mcp.TextContent{Text: text}},
	}
}
