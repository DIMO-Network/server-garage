package mcpserver

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"

	"github.com/modelcontextprotocol/go-sdk/mcp"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// mockExecutor implements GraphQLExecutor for testing.
type mockExecutor struct {
	fn func(ctx context.Context, query string, variables map[string]any) ([]byte, error)
}

func (m *mockExecutor) Execute(ctx context.Context, query string, variables map[string]any) ([]byte, error) {
	return m.fn(ctx, query, variables)
}

func TestExecutorQuery(t *testing.T) {
	expected := `{"data":{"vehicle":{"id":"123"}}}`
	exec := &mockExecutor{
		fn: func(ctx context.Context, query string, variables map[string]any) ([]byte, error) {
			return []byte(expected), nil
		},
	}

	result, err := exec.Execute(context.Background(), `{ vehicle { id } }`, nil)
	require.NoError(t, err)
	assert.JSONEq(t, expected, string(result))
}

type ctxKey string

func TestExecutorContextPropagation(t *testing.T) {
	key := ctxKey("user_id")
	exec := &mockExecutor{
		fn: func(ctx context.Context, query string, variables map[string]any) ([]byte, error) {
			val := ctx.Value(key)
			if val == nil {
				return nil, fmt.Errorf("context value not propagated")
			}
			if val != "user-42" {
				return nil, fmt.Errorf("unexpected context value: %v", val)
			}
			return []byte(`{"data":{}}`), nil
		},
	}

	ctx := context.WithValue(context.Background(), key, "user-42")
	_, err := exec.Execute(ctx, `{ me { id } }`, nil)
	require.NoError(t, err)
}

func TestExecutorError(t *testing.T) {
	exec := &mockExecutor{
		fn: func(ctx context.Context, query string, variables map[string]any) ([]byte, error) {
			return nil, fmt.Errorf("execution failed")
		},
	}

	_, err := exec.Execute(context.Background(), `{ fail }`, nil)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "execution failed")
}

func TestGetSchema(t *testing.T) {
	exec := &mockExecutor{
		fn: func(ctx context.Context, query string, variables map[string]any) ([]byte, error) {
			if strings.Contains(query, "__schema") {
				return []byte(`{"data":{"__schema":{"types":[{"name":"Query"}]}}}`), nil
			}
			return nil, fmt.Errorf("unexpected query")
		},
	}

	cache := &schemaCache{exec: exec}
	schema, err := cache.getSchema(context.Background())
	require.NoError(t, err)
	assert.Contains(t, schema, "__schema")
}

func TestGetSchemaCaching(t *testing.T) {
	var callCount atomic.Int32
	exec := &mockExecutor{
		fn: func(ctx context.Context, query string, variables map[string]any) ([]byte, error) {
			callCount.Add(1)
			return []byte(`{"data":{"__schema":{"types":[]}}}`), nil
		},
	}

	cache := &schemaCache{exec: exec}

	schema1, err := cache.getSchema(context.Background())
	require.NoError(t, err)

	schema2, err := cache.getSchema(context.Background())
	require.NoError(t, err)

	assert.Equal(t, schema1, schema2)
	assert.Equal(t, int32(1), callCount.Load(), "executor should only be called once due to caching")
}

// mockGQLExecutor returns a GraphQLExecutor that handles introspection and echoes regular queries.
func mockGQLExecutor() GraphQLExecutor {
	return &mockExecutor{
		fn: func(ctx context.Context, query string, variables map[string]any) ([]byte, error) {
			if strings.Contains(query, "__schema") {
				return []byte(`{"data":{"__schema":{"queryType":{"name":"Query"},"types":[{"name":"Query"},{"name":"Vehicle"}]}}}`), nil
			}
			resp := map[string]any{
				"data": map[string]any{
					"echoQuery":     query,
					"echoVariables": variables,
				},
			}
			return json.Marshal(resp)
		},
	}
}

func TestShortcutTool(t *testing.T) {
	toolDef := ToolDefinition{
		Name:        "get_vehicle",
		Description: "Fetches a vehicle by token ID.",
		Args: []ArgDefinition{
			{Name: "tokenId", Type: "integer", Description: "The vehicle token ID", Required: true},
		},
		Query: `query GetVehicle($tokenId: Int!) { vehicle(tokenId: $tokenId) { id make model } }`,
	}

	mcpServer := mcp.NewServer(&mcp.Implementation{
		Name:    "test-server",
		Version: "0.1.0",
	}, nil)

	exec := mockGQLExecutor()
	registerShortcutTools(mcpServer, exec, []ToolDefinition{toolDef})

	serverTransport, clientTransport := mcp.NewInMemoryTransports()

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	go func() {
		_ = mcpServer.Run(ctx, serverTransport)
	}()

	client := mcp.NewClient(&mcp.Implementation{
		Name:    "test-client",
		Version: "0.1.0",
	}, nil)

	session, err := client.Connect(ctx, clientTransport, nil)
	require.NoError(t, err)

	result, err := session.CallTool(ctx, &mcp.CallToolParams{
		Name: "get_vehicle",
		Arguments: map[string]any{
			"tokenId": 42,
		},
	})
	require.NoError(t, err)
	require.NotNil(t, result)
	require.Len(t, result.Content, 1)

	contentJSON, err := json.Marshal(result.Content[0])
	require.NoError(t, err)
	var textContent struct {
		Type string `json:"type"`
		Text string `json:"text"`
	}
	require.NoError(t, json.Unmarshal(contentJSON, &textContent))
	assert.Equal(t, "text", textContent.Type)

	var resp map[string]any
	require.NoError(t, json.Unmarshal([]byte(textContent.Text), &resp))
	data := resp["data"].(map[string]any)
	assert.Equal(t, toolDef.Query, data["echoQuery"])

	vars := data["echoVariables"].(map[string]any)
	assert.Equal(t, float64(42), vars["tokenId"])
}

func TestMCPHandlerEndToEnd(t *testing.T) {
	shortcutTool := ToolDefinition{
		Name:        "get_vehicle",
		Description: "Fetches a vehicle by token ID.",
		Args: []ArgDefinition{
			{Name: "tokenId", Type: "integer", Description: "The vehicle token ID", Required: true},
		},
		Query: `query GetVehicle($tokenId: Int!) { vehicle(tokenId: $tokenId) { id } }`,
	}

	mcpHandler := New(mockGQLExecutor(), "Test Server", "test", WithTools([]ToolDefinition{shortcutTool}))

	ts := httptest.NewServer(mcpHandler)
	defer ts.Close()

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	transport := &mcp.StreamableClientTransport{
		Endpoint:   ts.URL,
		HTTPClient: ts.Client(),
	}

	client := mcp.NewClient(&mcp.Implementation{
		Name:    "test-client",
		Version: "1.0",
	}, nil)

	session, err := client.Connect(ctx, transport, nil)
	require.NoError(t, err)
	defer func() { _ = session.Close() }()

	result, err := session.ListTools(ctx, &mcp.ListToolsParams{})
	require.NoError(t, err)
	require.NotNil(t, result)

	toolNames := make(map[string]bool)
	for _, tool := range result.Tools {
		toolNames[tool.Name] = true
	}

	assert.True(t, toolNames["test_get_schema"], "expected test_get_schema tool")
	assert.True(t, toolNames["test_query"], "expected test_query tool")
	assert.True(t, toolNames["get_vehicle"], "expected get_vehicle shortcut tool")

	schemaResult, err := session.CallTool(ctx, &mcp.CallToolParams{
		Name: "test_get_schema",
	})
	require.NoError(t, err)
	require.NotNil(t, schemaResult)
	require.Len(t, schemaResult.Content, 1)

	contentJSON, err := json.Marshal(schemaResult.Content[0])
	require.NoError(t, err)
	var textContent struct {
		Type string `json:"type"`
		Text string `json:"text"`
	}
	require.NoError(t, json.Unmarshal(contentJSON, &textContent))
	assert.Equal(t, "text", textContent.Type)
	assert.Contains(t, textContent.Text, "__schema")

	queryResult, err := session.CallTool(ctx, &mcp.CallToolParams{
		Name: "test_query",
		Arguments: map[string]any{
			"query": "{ vehicles { id } }",
		},
	})
	require.NoError(t, err)
	require.NotNil(t, queryResult)
	require.Len(t, queryResult.Content, 1)
}

func TestToolPrefixing(t *testing.T) {
	mcpHandler := New(mockGQLExecutor(), "Prefixed Server", "myprefix")

	ts := httptest.NewServer(mcpHandler)
	defer ts.Close()

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	transport := &mcp.StreamableClientTransport{
		Endpoint:   ts.URL,
		HTTPClient: ts.Client(),
	}

	client := mcp.NewClient(&mcp.Implementation{
		Name:    "test-client",
		Version: "1.0",
	}, nil)

	session, err := client.Connect(ctx, transport, nil)
	require.NoError(t, err)
	defer func() { _ = session.Close() }()

	result, err := session.ListTools(ctx, &mcp.ListToolsParams{})
	require.NoError(t, err)
	require.NotNil(t, result)

	toolNames := make(map[string]bool)
	for _, tool := range result.Tools {
		toolNames[tool.Name] = true
	}

	assert.True(t, toolNames["myprefix_get_schema"], "expected myprefix_get_schema tool")
	assert.True(t, toolNames["myprefix_query"], "expected myprefix_query tool")
	assert.False(t, toolNames["test_get_schema"], "should not have test prefix")
	assert.False(t, toolNames["test_query"], "should not have test prefix")
}
