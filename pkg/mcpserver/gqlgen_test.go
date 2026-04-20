package mcpserver

import (
	"context"
	"testing"

	"github.com/99designs/gqlgen/graphql"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/vektah/gqlparser/v2"
	"github.com/vektah/gqlparser/v2/ast"
)

func testSchema() *ast.Schema {
	return gqlparser.MustLoadSchema(
		&ast.Source{
			Name:  "test.graphqls",
			Input: `type Query { hello: String! }`,
		},
	)
}

func TestGQLGenExecutorSuccess(t *testing.T) {
	es := &graphql.ExecutableSchemaMock{
		SchemaFunc: testSchema,
		ComplexityFunc: func(ctx context.Context, typeName, fieldName string, childComplexity int, args map[string]any) (int, bool) {
			return 0, false
		},
		ExecFunc: func(ctx context.Context) graphql.ResponseHandler {
			return func(ctx context.Context) *graphql.Response {
				return &graphql.Response{
					Data: []byte(`{"hello":"world"}`),
				}
			}
		},
	}

	exec := NewGQLGenExecutor(es)
	result, err := exec.Execute(context.Background(), `{ hello }`, nil)
	require.NoError(t, err)
	assert.Contains(t, string(result), `"hello"`)
}

func TestGQLGenExecutorInvalidQuery(t *testing.T) {
	es := &graphql.ExecutableSchemaMock{
		SchemaFunc: testSchema,
		ComplexityFunc: func(ctx context.Context, typeName, fieldName string, childComplexity int, args map[string]any) (int, bool) {
			return 0, false
		},
		ExecFunc: func(ctx context.Context) graphql.ResponseHandler {
			return nil
		},
	}

	exec := NewGQLGenExecutor(es)
	_, err := exec.Execute(context.Background(), `{ nonExistent }`, nil)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "graphql operation error")
}

func TestGQLGenExecutorNilResponse(t *testing.T) {
	es := &graphql.ExecutableSchemaMock{
		SchemaFunc: testSchema,
		ComplexityFunc: func(ctx context.Context, typeName, fieldName string, childComplexity int, args map[string]any) (int, bool) {
			return 0, false
		},
		ExecFunc: func(ctx context.Context) graphql.ResponseHandler {
			return func(ctx context.Context) *graphql.Response {
				return nil
			}
		},
	}

	exec := NewGQLGenExecutor(es)
	_, err := exec.Execute(context.Background(), `{ hello }`, nil)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "nil response")
}
