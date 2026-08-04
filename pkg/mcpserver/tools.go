package mcpserver

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"strings"
	"text/template"
	"time"

	"github.com/modelcontextprotocol/go-sdk/mcp"
	"github.com/rs/zerolog"
)

func boolPtr(b bool) *bool { return &b }

// jsonNumberArgs is a map[string]any whose JSON numbers decode as json.Number
// instead of float64, at every nesting depth. gqlgen's scalar unmarshallers
// accept json.Number for Int and Float (its own HTTP transport decodes request
// bodies with UseNumber) but reject float64 for Int, and float64 loses
// precision above 2^53.
type jsonNumberArgs map[string]any

func (a *jsonNumberArgs) UnmarshalJSON(data []byte) error {
	dec := json.NewDecoder(bytes.NewReader(data))
	dec.UseNumber()
	return dec.Decode((*map[string]any)(a))
}

// decodeRawArgs unmarshals the raw wire-format arguments into dst. The typed
// argument the SDK hands tool handlers has been round-tripped through a plain
// map[string]any for schema validation (applySchema), which collapses all
// numbers to float64 before any custom unmarshaller runs. Decoding
// req.Params.Arguments directly is the only path that preserves numeric
// fidelity end to end.
func decodeRawArgs(raw json.RawMessage, dst any) error {
	if len(raw) == 0 {
		return nil
	}
	return json.Unmarshal(raw, dst)
}

// queryInput is the input for the query tool.
type queryInput struct {
	Query     string         `json:"query" jsonschema:"A GraphQL query or mutation string. Use $-prefixed variable placeholders for dynamic values."`
	Variables jsonNumberArgs `json:"variables,omitempty" jsonschema:"A JSON object mapping variable names to values. Keys must match the $-prefixed placeholders declared in the query."`
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
	}, func(ctx context.Context, req *mcp.CallToolRequest, _ queryInput) (*mcp.CallToolResult, any, error) {
		var input queryInput
		if err := decodeRawArgs(req.Params.Arguments, &input); err != nil {
			return &mcp.CallToolResult{
				Content: []mcp.Content{&mcp.TextContent{Text: fmt.Sprintf("failed to decode arguments: %s", err.Error())}},
				IsError: true,
			}, nil, nil
		}
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
// Returns an error if any tool's SelectionTemplate fails to parse.
func registerShortcutTools(server *mcp.Server, exec GraphQLExecutor, tools []ToolDefinition, logger *slog.Logger) error {
	for _, tool := range tools {
		inputSchema := buildInputSchema(tool.Args)
		toolOnlyArgs := make(map[string]bool, len(tool.Args))
		for _, a := range tool.Args {
			if a.ToolOnly {
				toolOnlyArgs[a.Name] = true
			}
		}

		var selTmpl *template.Template
		if tool.SelectionTemplate != "" {
			// missingkey=error: a caller omitting a key the template references
			// (e.g. a signalRequests entry without "agg") gets a clear render
			// error instead of "<no value>" spliced into the GraphQL query.
			tmpl, err := template.New(tool.Name).Option("missingkey=error").Parse(tool.SelectionTemplate)
			if err != nil {
				return fmt.Errorf("mcpserver: parse SelectionTemplate for tool %q: %w", tool.Name, err)
			}
			if !strings.Contains(tool.Query, SelectionPlaceholder) {
				return fmt.Errorf("mcpserver: tool %q has SelectionTemplate but Query is missing placeholder %q", tool.Name, SelectionPlaceholder)
			}
			selTmpl = tmpl
		}

		mcp.AddTool(server, &mcp.Tool{
			Name:        tool.Name,
			Description: tool.Description,
			InputSchema: inputSchema,
			Annotations: tool.Annotations,
		}, func(ctx context.Context, req *mcp.CallToolRequest, _ jsonNumberArgs) (*mcp.CallToolResult, any, error) {
			args := jsonNumberArgs{}
			if err := decodeRawArgs(req.Params.Arguments, &args); err != nil {
				return &mcp.CallToolResult{
					Content: []mcp.Content{&mcp.TextContent{Text: fmt.Sprintf("failed to decode arguments: %s", err.Error())}},
					IsError: true,
				}, nil, nil
			}
			query := tool.Query
			if selTmpl != nil {
				var buf strings.Builder
				if err := selTmpl.Execute(&buf, args); err != nil {
					return &mcp.CallToolResult{
						Content: []mcp.Content{&mcp.TextContent{Text: fmt.Sprintf("failed to render selection template: %s", err.Error())}},
						IsError: true,
					}, nil, nil
				}
				query = strings.Replace(tool.Query, SelectionPlaceholder, buf.String(), 1)
			}
			gqlArgs := map[string]any(args)
			if len(toolOnlyArgs) > 0 {
				gqlArgs = make(map[string]any, len(args))
				for k, v := range args {
					if toolOnlyArgs[k] {
						continue
					}
					gqlArgs[k] = v
				}
			}
			return executeTool(ctx, tool.Name, exec, query, gqlArgs, logger)
		})
	}
	return nil
}
