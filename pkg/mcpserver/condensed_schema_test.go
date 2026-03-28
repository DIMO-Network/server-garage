package mcpserver

import (
	"context"
	"encoding/json"
	"net/http/httptest"
	"testing"

	"github.com/modelcontextprotocol/go-sdk/mcp"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestWithCondensedSchema(t *testing.T) {
	condensedSDL := "type Query {\n  vehicle(tokenId: Int!): Vehicle\n}\n\ntype Vehicle {\n  tokenId: Int!\n  owner: String!\n}\n"

	mcpHandler, err := New(context.Background(), mockGQLExecutor(), "Test Server", "0.1.0", "test", WithCondensedSchema(condensedSDL))
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

	schemaResult, err := session.CallTool(ctx, &mcp.CallToolParams{
		Name: "test_get_schema",
	})
	require.NoError(t, err)
	require.NotNil(t, schemaResult)
	require.Len(t, schemaResult.Content, 1)

	contentJSON, err := json.Marshal(schemaResult.Content[0])
	require.NoError(t, err)
	var textContent struct {
		Text string `json:"text"`
	}
	require.NoError(t, json.Unmarshal(contentJSON, &textContent))

	assert.Contains(t, textContent.Text, "type Query")
	assert.Contains(t, textContent.Text, "type Vehicle")
	assert.NotContains(t, textContent.Text, "__schema", "condensed schema should not contain introspection data")
}

func TestWithoutCondensedSchemaFallback(t *testing.T) {
	mcpHandler, err := New(context.Background(), mockGQLExecutor(), "Test Server", "0.1.0", "test")
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

	schemaResult, err := session.CallTool(ctx, &mcp.CallToolParams{
		Name: "test_get_schema",
	})
	require.NoError(t, err)
	require.NotNil(t, schemaResult)
	require.Len(t, schemaResult.Content, 1)

	contentJSON, err := json.Marshal(schemaResult.Content[0])
	require.NoError(t, err)
	var textContent struct {
		Text string `json:"text"`
	}
	require.NoError(t, json.Unmarshal(contentJSON, &textContent))

	assert.Contains(t, textContent.Text, "__schema", "without condensed schema, should return introspection JSON")
}
