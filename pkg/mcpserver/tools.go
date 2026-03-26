package mcpserver

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	"github.com/modelcontextprotocol/go-sdk/mcp"
	"github.com/rs/zerolog"
)

// queryInput is the input for the query tool.
type queryInput struct {
	Query     string         `json:"query" jsonschema:"GraphQL query string"`
	Variables map[string]any `json:"variables,omitempty" jsonschema:"GraphQL variables"`
}

// instrumentTool logs and records metrics for a tool call.
func instrumentTool(ctx context.Context, toolName string, fn func() error) {
	start := time.Now()
	err := fn()
	duration := time.Since(start)
	logger := zerolog.Ctx(ctx)
	status := "success"
	if err != nil {
		status = "error"
		logger.Error().Err(err).Str("tool", toolName).Dur("duration", duration).Msg("tool call failed")
	} else {
		logger.Info().Str("tool", toolName).Dur("duration", duration).Msg("tool call succeeded")
	}
	toolCallsTotal.WithLabelValues(toolName, status).Inc()
	toolDurationSeconds.WithLabelValues(toolName).Observe(duration.Seconds())
}

// registerBuiltinTools registers the built-in tools (get_schema, query) on the server.
func registerBuiltinTools(server *mcp.Server, exec GraphQLExecutor, cache *schemaCache, toolPrefix string) {
	schemaToolName := toolPrefix + "_get_schema"
	mcp.AddTool(server, &mcp.Tool{
		Name:        schemaToolName,
		Description: "Returns the GraphQL schema via introspection. Use this to understand available types and fields before constructing queries.",
	}, func(ctx context.Context, req *mcp.CallToolRequest, _ struct{}) (*mcp.CallToolResult, any, error) {
		var schema string
		var toolErr error
		instrumentTool(ctx, schemaToolName, func() error {
			schema, toolErr = cache.getSchema(ctx)
			return toolErr
		})
		if toolErr != nil {
			return nil, nil, toolErr
		}
		return &mcp.CallToolResult{
			Content: []mcp.Content{
				&mcp.TextContent{Text: schema},
			},
		}, nil, nil
	})

	queryToolName := toolPrefix + "_query"
	mcp.AddTool(server, &mcp.Tool{
		Name:        queryToolName,
		Description: "Executes an arbitrary GraphQL query against the API.",
	}, func(ctx context.Context, req *mcp.CallToolRequest, input queryInput) (*mcp.CallToolResult, any, error) {
		var result []byte
		var toolErr error
		instrumentTool(ctx, queryToolName, func() error {
			result, toolErr = exec.Execute(ctx, input.Query, input.Variables)
			return toolErr
		})
		if toolErr != nil {
			return nil, nil, toolErr
		}
		return &mcp.CallToolResult{
			Content: []mcp.Content{
				&mcp.TextContent{Text: string(result)},
			},
		}, nil, nil
	})
}

// buildInputSchema constructs a ToolInputSchema from ArgDefinitions.
func buildInputSchema(args []ArgDefinition) map[string]any {
	properties := make(map[string]any)
	var required []string
	for _, arg := range args {
		properties[arg.Name] = map[string]any{
			"type":        arg.Type,
			"description": arg.Description,
		}
		if arg.Required {
			required = append(required, arg.Name)
		}
	}
	schema := map[string]any{
		"type":       "object",
		"properties": properties,
	}
	if len(required) > 0 {
		schema["required"] = required
	}
	return schema
}

// registerShortcutTools registers shortcut tools derived from ToolDefinitions.
func registerShortcutTools(server *mcp.Server, exec GraphQLExecutor, tools []ToolDefinition) {
	for _, tool := range tools {
		inputSchema := buildInputSchema(tool.Args)

		server.AddTool(&mcp.Tool{
			Name:        tool.Name,
			Description: tool.Description,
			InputSchema: inputSchema,
		}, func(ctx context.Context, req *mcp.CallToolRequest) (*mcp.CallToolResult, error) {
			var args map[string]any
			if req.Params.Arguments != nil {
				if err := json.Unmarshal(req.Params.Arguments, &args); err != nil {
					return nil, fmt.Errorf("unmarshal arguments: %w", err)
				}
			}

			var result []byte
			var toolErr error
			instrumentTool(ctx, tool.Name, func() error {
				result, toolErr = exec.Execute(ctx, tool.Query, args)
				return toolErr
			})
			if toolErr != nil {
				return nil, toolErr
			}
			return &mcp.CallToolResult{
				Content: []mcp.Content{
					&mcp.TextContent{Text: string(result)},
				},
			}, nil
		})
	}
}
