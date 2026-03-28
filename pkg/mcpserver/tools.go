package mcpserver

import (
	"context"
	"fmt"
	"log/slog"
	"time"

	"github.com/modelcontextprotocol/go-sdk/mcp"
	"github.com/rs/zerolog"
)

func boolPtr(b bool) *bool { return &b }

// queryInput is the input for the query tool.
type queryInput struct {
	Query     string         `json:"query" jsonschema:"A GraphQL query or mutation string. Use $-prefixed variable placeholders for dynamic values."`
	Variables map[string]any `json:"variables,omitempty" jsonschema:"A JSON object mapping variable names to values. Keys must match the $-prefixed placeholders declared in the query."`
}

// executeTool runs a GraphQL query, instruments the call, and returns an MCP result.
// Execution errors are returned as tool-level errors (IsError: true), not protocol errors.
//
// Logging is dual-path: zerolog (via context) for infrastructure observability, and
// slog (via explicit logger) for the MCP SDK logging convention. zerolog is always
// attempted; slog fires only when WithLogger is configured.
func executeTool(ctx context.Context, toolName string, exec GraphQLExecutor, query string, variables map[string]any, logger *slog.Logger) (*mcp.CallToolResult, any, error) {
	start := time.Now()
	result, err := exec.Execute(ctx, query, variables)
	duration := time.Since(start)

	zerologger := zerolog.Ctx(ctx)
	status := "success"
	if err != nil {
		status = "error"
		zerologger.Error().Err(err).Str("tool", toolName).Dur("duration", duration).Msg("tool call failed")
		if logger != nil {
			logger.ErrorContext(ctx, "tool call failed", "tool", toolName, "error", err, "duration", duration)
		}
	} else {
		zerologger.Info().Str("tool", toolName).Dur("duration", duration).Msg("tool call succeeded")
		if logger != nil {
			logger.InfoContext(ctx, "tool call succeeded", "tool", toolName, "duration", duration)
		}
	}
	toolCallsTotal.WithLabelValues(toolName, status).Inc()
	toolDurationSeconds.WithLabelValues(toolName).Observe(duration.Seconds())

	if err != nil {
		return &mcp.CallToolResult{
			Content: []mcp.Content{
				&mcp.TextContent{Text: err.Error()},
			},
			IsError: true,
		}, nil, nil
	}
	return &mcp.CallToolResult{
		Content: []mcp.Content{
			&mcp.TextContent{Text: string(result)},
		},
	}, nil, nil
}

// registerBuiltinTools registers the built-in tools (get_schema, query) on the server.
func registerBuiltinTools(server *mcp.Server, exec GraphQLExecutor, cachedSchema string, toolPrefix string, maxQuerySize int, logger *slog.Logger) {
	schemaToolName := toolPrefix + "_get_schema"
	mcp.AddTool(server, &mcp.Tool{
		Name: schemaToolName,
		Description: "Returns the GraphQL schema describing all available types, fields, arguments, and their relationships. " +
			"Call this first to understand the API before constructing any query.",
		Annotations: &mcp.ToolAnnotations{
			ReadOnlyHint:    true,
			DestructiveHint: boolPtr(false),
			OpenWorldHint:   boolPtr(false),
			IdempotentHint:  true,
		},
	}, func(ctx context.Context, req *mcp.CallToolRequest, _ struct{}) (*mcp.CallToolResult, any, error) {
		start := time.Now()
		defer func() {
			d := time.Since(start)
			zerolog.Ctx(ctx).Info().Str("tool", schemaToolName).Dur("duration", d).Msg("tool call succeeded")
			toolCallsTotal.WithLabelValues(schemaToolName, "success").Inc()
			toolDurationSeconds.WithLabelValues(schemaToolName).Observe(d.Seconds())
		}()
		if cachedSchema == "" {
			return &mcp.CallToolResult{
				Content: []mcp.Content{
					&mcp.TextContent{Text: "schema is unavailable"},
				},
				IsError: true,
			}, nil, nil
		}
		return &mcp.CallToolResult{
			Content: []mcp.Content{
				&mcp.TextContent{Text: cachedSchema},
			},
		}, nil, nil
	})

	queryToolName := toolPrefix + "_query"
	mcp.AddTool(server, &mcp.Tool{
		Name: queryToolName,
		Description: "Executes a GraphQL query or mutation and returns the result as JSON " +
			"in the standard GraphQL response format ({\"data\": ...} on success, {\"errors\": [...]} on failure). " +
			"Always call " + schemaToolName + " first to discover the schema. " +
			"Prefer purpose-specific tools when available and only use this tool for operations they do not cover. " +
			"Pass dynamic values using the 'variables' parameter with $-prefixed placeholders in the query " +
			"instead of interpolating values directly into the query string.",
		Annotations: &mcp.ToolAnnotations{
			OpenWorldHint: boolPtr(false),
		},
	}, func(ctx context.Context, req *mcp.CallToolRequest, input queryInput) (*mcp.CallToolResult, any, error) {
		if len(input.Query) > maxQuerySize {
			return &mcp.CallToolResult{
				Content: []mcp.Content{
					&mcp.TextContent{Text: fmt.Sprintf("query exceeds maximum size of %d bytes", maxQuerySize)},
				},
				IsError: true,
			}, nil, nil
		}
		return executeTool(ctx, queryToolName, exec, input.Query, input.Variables, logger)
	})
}

// buildInputSchema constructs a ToolInputSchema from ArgDefinitions.
func buildInputSchema(args []ArgDefinition) map[string]any {
	properties := make(map[string]any)
	var required []string
	for _, arg := range args {
		prop := map[string]any{
			"type":        arg.Type,
			"description": arg.Description,
		}
		if arg.Type == "array" && arg.ItemsType != "" {
			prop["items"] = map[string]any{"type": arg.ItemsType}
		}
		if len(arg.EnumValues) > 0 {
			prop["enum"] = arg.EnumValues
		}
		properties[arg.Name] = prop
		if arg.Required {
			required = append(required, arg.Name)
		}
	}
	schema := map[string]any{
		"type":                 "object",
		"properties":           properties,
		"additionalProperties": false,
	}
	if len(required) > 0 {
		schema["required"] = required
	}
	return schema
}

// registerShortcutTools registers shortcut tools derived from ToolDefinitions.
func registerShortcutTools(server *mcp.Server, exec GraphQLExecutor, tools []ToolDefinition, logger *slog.Logger) {
	for _, tool := range tools {
		inputSchema := buildInputSchema(tool.Args)

		mcp.AddTool(server, &mcp.Tool{
			Name:        tool.Name,
			Description: tool.Description,
			InputSchema: inputSchema,
			Annotations: tool.Annotations,
		}, func(ctx context.Context, req *mcp.CallToolRequest, args map[string]any) (*mcp.CallToolResult, any, error) {
			return executeTool(ctx, tool.Name, exec, tool.Query, args, logger)
		})
	}
}
