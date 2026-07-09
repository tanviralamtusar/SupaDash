package api

import (
	"context"
	"log/slog"
	"strings"
	"testing"

	"github.com/modelcontextprotocol/go-sdk/mcp"
	"supadash/conf"
)

// listMCPTools spins up the given server over an in-memory transport and
// returns the names of its registered tools.
func listMCPTools(t *testing.T, server *mcp.Server) map[string]bool {
	t.Helper()
	ctx := context.Background()

	serverTransport, clientTransport := mcp.NewInMemoryTransports()
	serverSession, err := server.Connect(ctx, serverTransport, nil)
	if err != nil {
		t.Fatalf("server connect: %v", err)
	}
	defer serverSession.Close()

	client := mcp.NewClient(&mcp.Implementation{Name: "test-client", Version: "0.0.1"}, nil)
	clientSession, err := client.Connect(ctx, clientTransport, nil)
	if err != nil {
		t.Fatalf("client connect: %v", err)
	}
	defer clientSession.Close()

	result, err := clientSession.ListTools(ctx, &mcp.ListToolsParams{})
	if err != nil {
		t.Fatalf("list tools: %v", err)
	}

	names := make(map[string]bool, len(result.Tools))
	for _, tool := range result.Tools {
		names[tool.Name] = true
	}
	return names
}

func newTestMCPApi() *Api {
	return &Api{
		logger:  slog.Default(),
		config:  &conf.Config{},
		queries: unimplementedQuerier{},
	}
}

var accountLevelTools = []string{
	"list_projects", "get_project", "create_project", "pause_project",
	"restore_project", "list_organizations", "get_organization",
}

var projectLevelTools = []string{
	"list_tables", "list_extensions", "list_migrations", "apply_migration", "execute_sql",
	"create_branch", "list_branches", "delete_branch", "merge_branch", "reset_branch", "rebase_branch",
	"get_logs", "get_advisors",
	"get_project_url", "get_anon_key", "generate_typescript_types",
	"list_edge_functions", "get_edge_function", "deploy_edge_function",
	"list_storage_buckets", "update_storage_bucket",
}

func TestMCPServerUnscopedHasAllTools(t *testing.T) {
	a := newTestMCPApi()
	tools := listMCPTools(t, a.buildMCPServer(""))

	for _, name := range append(append([]string{}, accountLevelTools...), projectLevelTools...) {
		if !tools[name] {
			t.Errorf("unscoped server missing tool %q", name)
		}
	}
}

func TestMCPServerScopedHidesAccountTools(t *testing.T) {
	a := newTestMCPApi()
	tools := listMCPTools(t, a.buildMCPServer("someproject"))

	for _, name := range accountLevelTools {
		if tools[name] {
			t.Errorf("scoped server must not expose account-level tool %q", name)
		}
	}
	for _, name := range projectLevelTools {
		if !tools[name] {
			t.Errorf("scoped server missing project tool %q", name)
		}
	}
}

func TestGenerateAccessToken(t *testing.T) {
	token, hash, err := generateAccessToken()
	if err != nil {
		t.Fatal(err)
	}
	if !strings.HasPrefix(token, "sbp_") {
		t.Errorf("token %q missing sbp_ prefix", token)
	}
	if len(token) != len("sbp_")+40 {
		t.Errorf("unexpected token length %d", len(token))
	}
	if hashAccessToken(token) != hash {
		t.Error("hash mismatch between generation and verification paths")
	}

	// Tokens must be unique
	token2, _, _ := generateAccessToken()
	if token == token2 {
		t.Error("two generated tokens are identical")
	}
}
