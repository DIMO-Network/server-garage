package mcpserver

import (
	"context"
	"net/http"

	"github.com/modelcontextprotocol/go-sdk/mcp"
	"github.com/rs/zerolog"
)

// GraphQLExecutor executes GraphQL operations and returns the JSON response.
type GraphQLExecutor interface {
	Execute(ctx context.Context, query string, variables map[string]any) ([]byte, error)
}

// ArgDefinition describes a single argument for a tool.
type ArgDefinition struct {
	Name        string
	Type        string // "string", "integer", "number", "boolean", "object"
	Description string
	Required    bool
}

// ToolDefinition describes an MCP tool backed by a GraphQL query.
type ToolDefinition struct {
	Name        string
	Description string
	Args        []ArgDefinition
	Query       string
}

// Option configures the MCP server.
type Option func(*config)

// config holds internal configuration for the MCP server.
type config struct {
	tools []ToolDefinition
}

// WithTools returns an Option that registers additional tool definitions.
func WithTools(tools []ToolDefinition) Option {
	return func(c *config) {
		c.tools = append(c.tools, tools...)
	}
}

// New creates an http.Handler that serves an MCP Streamable HTTP server.
// It wraps a GraphQL executor and exposes registered tools as MCP tools.
func New(exec GraphQLExecutor, serverName string, toolPrefix string, opts ...Option) http.Handler {
	cfg := &config{}
	for _, opt := range opts {
		opt(cfg)
	}

	server := mcp.NewServer(&mcp.Implementation{
		Name:    serverName,
		Version: "0.1.0",
	}, nil)

	cache := &schemaCache{exec: exec}

	// Pre-warm the schema cache so the first MCP client doesn't pay introspection cost.
	if _, err := cache.getSchema(context.Background()); err != nil {
		zerolog.Ctx(context.Background()).Warn().Err(err).Msg("failed to pre-warm schema cache")
	}

	registerBuiltinTools(server, exec, cache, toolPrefix)
	registerShortcutTools(server, exec, cfg.tools)

	return mcp.NewStreamableHTTPHandler(func(r *http.Request) *mcp.Server {
		return server
	}, nil)
}
