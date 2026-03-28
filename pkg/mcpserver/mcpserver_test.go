package mcpserver

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
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

func TestLoadSchema(t *testing.T) {
	exec := &mockExecutor{
		fn: func(ctx context.Context, query string, variables map[string]any) ([]byte, error) {
			if strings.Contains(query, "__schema") {
				return []byte(`{"data":{"__schema":{"types":[{"name":"Query"}]}}}`), nil
			}
			return nil, fmt.Errorf("unexpected query")
		},
	}

	schema, err := loadSchema(context.Background(), exec)
	require.NoError(t, err)
	assert.Contains(t, schema, "__schema")
}

func TestLoadSchemaError(t *testing.T) {
	exec := &mockExecutor{
		fn: func(ctx context.Context, query string, variables map[string]any) ([]byte, error) {
			return nil, fmt.Errorf("connection refused")
		},
	}

	_, err := loadSchema(context.Background(), exec)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "introspection query")
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
	registerShortcutTools(mcpServer, exec, []ToolDefinition{toolDef}, nil)

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

	mcpHandler, err := New(context.Background(), mockGQLExecutor(), "Test Server", "0.1.0", "test", WithTools([]ToolDefinition{shortcutTool}))
	require.NoError(t, err)

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
	mcpHandler, err := New(context.Background(), mockGQLExecutor(), "Prefixed Server", "0.1.0", "myprefix")
	require.NoError(t, err)

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

func TestEmptyPrefixRejected(t *testing.T) {
	_, err := New(context.Background(), mockGQLExecutor(), "Test Server", "0.1.0", "")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "toolPrefix must be non-empty")
}

func TestEmptyServerNameRejected(t *testing.T) {
	_, err := New(context.Background(), mockGQLExecutor(), "", "0.1.0", "test")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "serverName must be non-empty")
}

func TestNilExecutorRejected(t *testing.T) {
	_, err := New(context.Background(), nil, "Test Server", "0.1.0", "test")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "executor must not be nil")
}

func TestQueryToolErrorReturnsIsError(t *testing.T) {
	exec := &mockExecutor{
		fn: func(ctx context.Context, query string, variables map[string]any) ([]byte, error) {
			if strings.Contains(query, "__schema") {
				return []byte(`{"data":{"__schema":{"queryType":{"name":"Query"}}}}`), nil
			}
			return nil, fmt.Errorf("field not found: badField")
		},
	}

	mcpServer := mcp.NewServer(&mcp.Implementation{
		Name:    "test-server",
		Version: "0.1.0",
	}, nil)

	registerBuiltinTools(mcpServer, exec, `{"data":{}}`, "test", 65536, nil)

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
		Name: "test_query",
		Arguments: map[string]any{
			"query": "{ badField }",
		},
	})
	// Should NOT be a protocol error — the tool handled it gracefully.
	require.NoError(t, err)
	require.NotNil(t, result)
	assert.True(t, result.IsError, "expected IsError to be true for execution failure")
	require.Len(t, result.Content, 1)

	contentJSON, err := json.Marshal(result.Content[0])
	require.NoError(t, err)
	var textContent struct {
		Text string `json:"text"`
	}
	require.NoError(t, json.Unmarshal(contentJSON, &textContent))
	assert.Contains(t, textContent.Text, "field not found")
}

func ptr[T any](v T) *T { return &v }

func TestBuiltinToolAnnotations(t *testing.T) {
	exec := mockGQLExecutor()
	mcpHandler, err := New(context.Background(), exec, "Test Server", "0.1.0", "test")
	require.NoError(t, err)

	ts := httptest.NewServer(mcpHandler)
	defer ts.Close()

	ctx := context.Background()
	transport := &mcp.StreamableClientTransport{
		Endpoint:   ts.URL,
		HTTPClient: ts.Client(),
	}

	client := mcp.NewClient(&mcp.Implementation{Name: "test-client", Version: "1.0"}, nil)
	session, err := client.Connect(ctx, transport, nil)
	require.NoError(t, err)
	defer func() { _ = session.Close() }()

	toolsResult, err := session.ListTools(ctx, nil)
	require.NoError(t, err)

	toolMap := make(map[string]*mcp.Tool)
	for _, t := range toolsResult.Tools {
		toolMap[t.Name] = t
	}

	schema := toolMap["test_get_schema"]
	require.NotNil(t, schema)
	require.NotNil(t, schema.Annotations, "get_schema should have annotations")
	assert.True(t, schema.Annotations.ReadOnlyHint)
	assert.Equal(t, ptr(false), schema.Annotations.DestructiveHint)
	assert.Equal(t, ptr(false), schema.Annotations.OpenWorldHint)
	assert.True(t, schema.Annotations.IdempotentHint)

	query := toolMap["test_query"]
	require.NotNil(t, query)
	require.NotNil(t, query.Annotations, "query should have annotations")
	assert.False(t, query.Annotations.ReadOnlyHint)
	assert.Equal(t, ptr(false), query.Annotations.OpenWorldHint)
}

func TestShortcutToolAnnotations(t *testing.T) {
	tool := ToolDefinition{
		Name:        "test_get_vehicle",
		Description: "Get a vehicle",
		Args:        []ArgDefinition{{Name: "id", Type: "integer", Required: true}},
		Query:       `query($id: Int!) { vehicle(id: $id) { id } }`,
		Annotations: &mcp.ToolAnnotations{
			ReadOnlyHint:    true,
			DestructiveHint: boolPtr(false),
			OpenWorldHint:   boolPtr(false),
			IdempotentHint:  true,
		},
	}

	mcpHandler, err := New(context.Background(), mockGQLExecutor(), "Test", "0.1.0", "test", WithTools([]ToolDefinition{tool}))
	require.NoError(t, err)

	ts := httptest.NewServer(mcpHandler)
	defer ts.Close()

	ctx := context.Background()
	transport := &mcp.StreamableClientTransport{Endpoint: ts.URL, HTTPClient: ts.Client()}
	client := mcp.NewClient(&mcp.Implementation{Name: "test-client", Version: "1.0"}, nil)
	session, err := client.Connect(ctx, transport, nil)
	require.NoError(t, err)
	defer func() { _ = session.Close() }()

	toolsResult, err := session.ListTools(ctx, nil)
	require.NoError(t, err)

	var vehicleTool *mcp.Tool
	for _, t := range toolsResult.Tools {
		if t.Name == "test_get_vehicle" {
			vehicleTool = t
			break
		}
	}
	require.NotNil(t, vehicleTool)
	require.NotNil(t, vehicleTool.Annotations)
	assert.True(t, vehicleTool.Annotations.ReadOnlyHint)
	assert.Equal(t, boolPtr(false), vehicleTool.Annotations.DestructiveHint)
}

func TestBuildInputSchemaArrayType(t *testing.T) {
	args := []ArgDefinition{
		{Name: "ids", Type: "array", ItemsType: "integer", Description: "List of IDs", Required: true},
		{Name: "name", Type: "string", Description: "A name", Required: false},
	}
	schema := buildInputSchema(args)

	assert.Equal(t, false, schema["additionalProperties"], "schema should disallow additional properties")

	properties := schema["properties"].(map[string]any)

	idsProp := properties["ids"].(map[string]any)
	assert.Equal(t, "array", idsProp["type"])
	items := idsProp["items"].(map[string]any)
	assert.Equal(t, "integer", items["type"])

	nameProp := properties["name"].(map[string]any)
	_, hasItems := nameProp["items"]
	assert.False(t, hasItems, "non-array type should not have items")
}

func TestQuerySizeLimitRejected(t *testing.T) {
	exec := mockGQLExecutor()
	mcpServer := mcp.NewServer(&mcp.Implementation{Name: "test", Version: "0.1.0"}, nil)
	registerBuiltinTools(mcpServer, exec, `{"data":{}}`, "test", 50, nil)

	serverTransport, clientTransport := mcp.NewInMemoryTransports()
	ctx := context.Background()
	go func() { _ = mcpServer.Run(ctx, serverTransport) }()

	client := mcp.NewClient(&mcp.Implementation{Name: "test-client", Version: "0.1.0"}, nil)
	session, err := client.Connect(ctx, clientTransport, nil)
	require.NoError(t, err)

	bigQuery := strings.Repeat("x", 51)
	result, err := session.CallTool(ctx, &mcp.CallToolParams{
		Name:      "test_query",
		Arguments: map[string]any{"query": bigQuery},
	})
	require.NoError(t, err)
	assert.True(t, result.IsError)

	contentJSON, _ := json.Marshal(result.Content[0])
	var tc struct{ Text string }
	json.Unmarshal(contentJSON, &tc)
	assert.Contains(t, tc.Text, "exceeds maximum size")
}

func TestBuildInputSchemaEnumValues(t *testing.T) {
	args := []ArgDefinition{
		{
			Name:        "status",
			Type:        "string",
			Description: "Filter by status",
			Required:    true,
			EnumValues:  []string{"ACTIVE", "INACTIVE"},
		},
		{
			Name:        "name",
			Type:        "string",
			Description: "A name",
			Required:    false,
		},
	}
	schema := buildInputSchema(args)
	properties := schema["properties"].(map[string]any)

	statusProp := properties["status"].(map[string]any)
	assert.Equal(t, "string", statusProp["type"])
	assert.Equal(t, []string{"ACTIVE", "INACTIVE"}, statusProp["enum"])

	nameProp := properties["name"].(map[string]any)
	_, hasEnum := nameProp["enum"]
	assert.False(t, hasEnum, "non-enum type should not have enum field")
}

type authCtxKey struct{}

// headerRoundTripper wraps an http.RoundTripper and adds custom headers.
type headerRoundTripper struct {
	base    http.RoundTripper
	headers http.Header
}

func (h *headerRoundTripper) RoundTrip(req *http.Request) (*http.Response, error) {
	for k, v := range h.headers {
		req.Header[k] = v
	}
	return h.base.RoundTrip(req)
}

func TestTokenVerifierAccepts(t *testing.T) {
	verifier := func(ctx context.Context, token string) (context.Context, error) {
		if token == "valid-token" {
			return context.WithValue(ctx, authCtxKey{}, "authenticated"), nil
		}
		return ctx, fmt.Errorf("invalid token")
	}

	mcpHandler, err := New(context.Background(), mockGQLExecutor(), "Test", "0.1.0", "test",
		WithTokenVerifier(verifier),
	)
	require.NoError(t, err)

	ts := httptest.NewServer(mcpHandler)
	defer ts.Close()

	ctx := context.Background()
	httpClient := ts.Client()
	httpClient.Transport = &headerRoundTripper{
		base:    httpClient.Transport,
		headers: http.Header{"Authorization": []string{"Bearer valid-token"}},
	}
	transport := &mcp.StreamableClientTransport{
		Endpoint:   ts.URL,
		HTTPClient: httpClient,
	}
	client := mcp.NewClient(&mcp.Implementation{Name: "test-client", Version: "1.0"}, nil)
	session, err := client.Connect(ctx, transport, nil)
	require.NoError(t, err)
	defer func() { _ = session.Close() }()

	tools, err := session.ListTools(ctx, nil)
	require.NoError(t, err)
	assert.NotEmpty(t, tools.Tools)
}

func TestTokenVerifierRejects(t *testing.T) {
	verifier := func(ctx context.Context, token string) (context.Context, error) {
		return ctx, fmt.Errorf("invalid")
	}

	mcpHandler, err := New(context.Background(), mockGQLExecutor(), "Test", "0.1.0", "test",
		WithTokenVerifier(verifier),
	)
	require.NoError(t, err)

	ts := httptest.NewServer(mcpHandler)
	defer ts.Close()

	resp, err := http.Post(ts.URL, "application/json", strings.NewReader(`{}`))
	require.NoError(t, err)
	defer resp.Body.Close()
	assert.Equal(t, http.StatusUnauthorized, resp.StatusCode)
}

func TestTokenVerifierMissingHeader(t *testing.T) {
	verifier := func(ctx context.Context, token string) (context.Context, error) {
		return ctx, nil
	}

	mcpHandler, err := New(context.Background(), mockGQLExecutor(), "Test", "0.1.0", "test",
		WithTokenVerifier(verifier),
	)
	require.NoError(t, err)

	ts := httptest.NewServer(mcpHandler)
	defer ts.Close()

	resp, err := http.Post(ts.URL, "application/json", strings.NewReader(`{}`))
	require.NoError(t, err)
	defer resp.Body.Close()
	assert.Equal(t, http.StatusUnauthorized, resp.StatusCode)
}

func TestShortcutToolUnexpectedArgs(t *testing.T) {
	tool := ToolDefinition{
		Name:        "test_get_vehicle",
		Description: "Get a vehicle",
		Args: []ArgDefinition{
			{Name: "tokenId", Type: "integer", Description: "The vehicle token ID", Required: true},
		},
		Query: `query($tokenId: Int!) { vehicle(tokenId: $tokenId) { id } }`,
	}

	mcpServer := mcp.NewServer(&mcp.Implementation{Name: "test", Version: "0.1.0"}, nil)
	registerShortcutTools(mcpServer, mockGQLExecutor(), []ToolDefinition{tool}, nil)

	serverTransport, clientTransport := mcp.NewInMemoryTransports()
	ctx := context.Background()
	go func() { _ = mcpServer.Run(ctx, serverTransport) }()

	client := mcp.NewClient(&mcp.Implementation{Name: "test-client", Version: "0.1.0"}, nil)
	session, err := client.Connect(ctx, clientTransport, nil)
	require.NoError(t, err)

	// The InputSchema sets additionalProperties: false, and the MCP SDK enforces
	// JSON Schema validation server-side. An unexpected argument should be
	// rejected at the protocol level before reaching the tool handler.
	_, err = session.CallTool(ctx, &mcp.CallToolParams{
		Name: "test_get_vehicle",
		Arguments: map[string]any{
			"tokenId":     123,
			"notARealArg": "should not reach executor",
		},
	})
	require.Error(t, err, "SDK should reject unexpected arguments via additionalProperties: false")
	assert.Contains(t, err.Error(), "notARealArg")
}

func TestMultipleShortcutToolsDispatch(t *testing.T) {
	tools := []ToolDefinition{
		{
			Name:        "get_vehicle",
			Description: "Get vehicle",
			Args:        []ArgDefinition{{Name: "id", Type: "integer", Required: true}},
			Query:       `query($id: Int!) { vehicle(id: $id) { id } }`,
		},
		{
			Name:        "get_user",
			Description: "Get user",
			Args:        []ArgDefinition{{Name: "name", Type: "string", Required: true}},
			Query:       `query($name: String!) { user(name: $name) { name } }`,
		},
		{
			Name:        "get_status",
			Description: "Get status",
			Args:        nil,
			Query:       `{ status }`,
		},
	}

	mcpServer := mcp.NewServer(&mcp.Implementation{Name: "test", Version: "0.1.0"}, nil)
	exec := mockGQLExecutor()
	registerShortcutTools(mcpServer, exec, tools, nil)

	serverTransport, clientTransport := mcp.NewInMemoryTransports()
	ctx := context.Background()
	go func() { _ = mcpServer.Run(ctx, serverTransport) }()

	client := mcp.NewClient(&mcp.Implementation{Name: "test-client", Version: "0.1.0"}, nil)
	session, err := client.Connect(ctx, clientTransport, nil)
	require.NoError(t, err)

	for _, tool := range tools {
		t.Run(tool.Name, func(t *testing.T) {
			args := map[string]any{}
			for _, a := range tool.Args {
				switch a.Type {
				case "integer":
					args[a.Name] = 1
				case "string":
					args[a.Name] = "test"
				}
			}

			result, err := session.CallTool(ctx, &mcp.CallToolParams{
				Name:      tool.Name,
				Arguments: args,
			})
			require.NoError(t, err)
			require.NotNil(t, result)
			require.Len(t, result.Content, 1)

			contentJSON, err := json.Marshal(result.Content[0])
			require.NoError(t, err)
			var tc struct{ Text string }
			require.NoError(t, json.Unmarshal(contentJSON, &tc))

			var resp map[string]any
			require.NoError(t, json.Unmarshal([]byte(tc.Text), &resp))
			data := resp["data"].(map[string]any)
			assert.Equal(t, tool.Query, data["echoQuery"], "tool %s dispatched wrong query", tool.Name)
		})
	}
}

func TestTokenVerifierContextPropagation(t *testing.T) {
	verifier := func(ctx context.Context, token string) (context.Context, error) {
		return context.WithValue(ctx, authCtxKey{}, "user-from-token"), nil
	}

	exec := &mockExecutor{
		fn: func(ctx context.Context, query string, variables map[string]any) ([]byte, error) {
			if strings.Contains(query, "__schema") {
				return []byte(`{"data":{"__schema":{"queryType":{"name":"Query"}}}}`), nil
			}
			val, ok := ctx.Value(authCtxKey{}).(string)
			if !ok || val != "user-from-token" {
				return nil, fmt.Errorf("expected context value 'user-from-token', got %v", ctx.Value(authCtxKey{}))
			}
			return []byte(`{"data":{"authenticated":true}}`), nil
		},
	}

	mcpHandler, err := New(context.Background(), exec, "Test", "0.1.0", "test",
		WithTokenVerifier(verifier),
	)
	require.NoError(t, err)

	ts := httptest.NewServer(mcpHandler)
	defer ts.Close()

	ctx := context.Background()
	httpClient := ts.Client()
	httpClient.Transport = &headerRoundTripper{
		base:    httpClient.Transport,
		headers: http.Header{"Authorization": []string{"Bearer any-token"}},
	}
	transport := &mcp.StreamableClientTransport{Endpoint: ts.URL, HTTPClient: httpClient}
	client := mcp.NewClient(&mcp.Implementation{Name: "test-client", Version: "1.0"}, nil)
	session, err := client.Connect(ctx, transport, nil)
	require.NoError(t, err)
	defer func() { _ = session.Close() }()

	result, err := session.CallTool(ctx, &mcp.CallToolParams{
		Name:      "test_query",
		Arguments: map[string]any{"query": `{ me { id } }`},
	})
	require.NoError(t, err)
	require.NotNil(t, result)
	assert.False(t, result.IsError, "tool should succeed with propagated context")
}

func TestWithLoggerOption(t *testing.T) {
	var buf bytes.Buffer
	handler := slog.NewJSONHandler(&buf, &slog.HandlerOptions{Level: slog.LevelInfo})
	logger := slog.New(handler)

	mcpHandler, err := New(context.Background(), mockGQLExecutor(), "Test", "0.1.0", "test",
		WithLogger(logger),
	)
	require.NoError(t, err)

	ts := httptest.NewServer(mcpHandler)
	defer ts.Close()

	ctx := context.Background()
	transport := &mcp.StreamableClientTransport{Endpoint: ts.URL, HTTPClient: ts.Client()}
	client := mcp.NewClient(&mcp.Implementation{Name: "test-client", Version: "1.0"}, nil)
	session, err := client.Connect(ctx, transport, nil)
	require.NoError(t, err)
	defer func() { _ = session.Close() }()

	_, err = session.CallTool(ctx, &mcp.CallToolParams{
		Name:      "test_query",
		Arguments: map[string]any{"query": `{ __typename }`},
	})
	require.NoError(t, err)
	assert.Contains(t, buf.String(), "tool call succeeded")
}
