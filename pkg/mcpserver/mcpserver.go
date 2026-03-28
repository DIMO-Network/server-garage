package mcpserver

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"strings"

	"github.com/modelcontextprotocol/go-sdk/mcp"
)

// GraphQLExecutor executes GraphQL operations and returns the JSON response.
type GraphQLExecutor interface {
	Execute(ctx context.Context, query string, variables map[string]any) ([]byte, error)
}

// ArgDefinition describes a single argument for a tool.
type ArgDefinition struct {
	Name        string
	Type        string // "string", "integer", "number", "boolean", "object", "array"
	Description string
	Required    bool
	ItemsType   string   // JSON Schema type for array elements
	EnumValues  []string // Allowed values for enum types
}

// ToolDefinition describes an MCP tool backed by a GraphQL query.
type ToolDefinition struct {
	Name        string
	Description string
	Args        []ArgDefinition
	Query       string
	Annotations *mcp.ToolAnnotations
}

// Option configures the MCP server.
type Option func(*config)

// TokenVerifier validates a bearer token and returns an enriched context.
type TokenVerifier func(ctx context.Context, token string) (context.Context, error)

// config holds internal configuration for the MCP server.
type config struct {
	tools           []ToolDefinition
	condensedSchema string
	stateless       bool // default true
	maxQuerySize    int  // default 65536
	tokenVerifier   TokenVerifier
	logger          *slog.Logger
}

// WithTools returns an Option that registers additional tool definitions.
func WithTools(tools []ToolDefinition) Option {
	return func(c *config) {
		c.tools = append(c.tools, tools...)
	}
}

// WithCondensedSchema returns an Option that provides a condensed SDL schema
// for the get_schema tool to return instead of the full introspection JSON.
func WithCondensedSchema(schema string) Option {
	return func(c *config) {
		c.condensedSchema = schema
	}
}

// WithStateless returns an Option that controls whether the MCP server runs in stateless mode.
func WithStateless(stateless bool) Option {
	return func(c *config) { c.stateless = stateless }
}

// WithMaxQuerySize returns an Option that sets the maximum allowed query size in bytes.
func WithMaxQuerySize(bytes int) Option {
	return func(c *config) { c.maxQuerySize = bytes }
}

// WithTokenVerifier returns an Option that adds bearer token authentication middleware.
// Use this for simple deployments where the MCP server manages its own auth. For services
// that already have HTTP middleware (e.g., JWT validation chains), wrap the handler
// externally instead — this avoids duplicating auth logic and lets you reuse existing
// middleware stacks.
func WithTokenVerifier(v TokenVerifier) Option {
	return func(c *config) { c.tokenVerifier = v }
}

// WithLogger returns an Option that sets an slog.Logger for MCP-level logging.
func WithLogger(logger *slog.Logger) Option {
	return func(c *config) { c.logger = logger }
}

// New creates an http.Handler that serves an MCP Streamable HTTP server.
// It wraps a GraphQL executor and exposes registered tools as MCP tools.
func New(ctx context.Context, exec GraphQLExecutor, serverName string, version string, toolPrefix string, opts ...Option) (http.Handler, error) {
	if exec == nil {
		return nil, errors.New("mcpserver: executor must not be nil")
	}
	if serverName == "" {
		return nil, errors.New("mcpserver: serverName must be non-empty")
	}
	if toolPrefix == "" {
		return nil, errors.New("mcpserver: toolPrefix must be non-empty")
	}

	cfg := &config{
		stateless:    true,
		maxQuerySize: 65536,
	}
	for _, opt := range opts {
		opt(cfg)
	}

	server := mcp.NewServer(&mcp.Implementation{
		Name:    serverName,
		Version: version,
	}, &mcp.ServerOptions{
		Logger: cfg.logger,
	})

	// Load the schema eagerly so initialization fails fast if the executor is broken.
	schema, err := loadSchema(ctx, exec)
	if err != nil {
		return nil, fmt.Errorf("mcpserver: schema introspection failed: %w", err)
	}

	schemaContent := schema
	if cfg.condensedSchema != "" {
		schemaContent = cfg.condensedSchema
	}
	registerBuiltinTools(server, exec, schemaContent, toolPrefix, cfg.maxQuerySize, cfg.logger)
	registerShortcutTools(server, exec, cfg.tools, cfg.logger)

	handler := mcp.NewStreamableHTTPHandler(func(r *http.Request) *mcp.Server {
		return server
	}, &mcp.StreamableHTTPOptions{
		Stateless: cfg.stateless,
	})

	var h http.Handler = handler
	if cfg.tokenVerifier != nil {
		h = tokenVerifierMiddleware(cfg.tokenVerifier, h)
	}

	return h, nil
}

func tokenVerifierMiddleware(verify TokenVerifier, next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		auth := r.Header.Get("Authorization")
		if !strings.HasPrefix(auth, "Bearer ") {
			http.Error(w, "missing or invalid Authorization header", http.StatusUnauthorized)
			return
		}
		token := strings.TrimPrefix(auth, "Bearer ")
		ctx, err := verify(r.Context(), token)
		if err != nil {
			http.Error(w, "unauthorized", http.StatusUnauthorized)
			return
		}
		next.ServeHTTP(w, r.WithContext(ctx))
	})
}
