package mcpserver

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"

	"github.com/99designs/gqlgen/graphql"
	"github.com/99designs/gqlgen/graphql/executor"
)

// gqlgenExecutor implements GraphQLExecutor using a gqlgen ExecutableSchema.
type gqlgenExecutor struct {
	exec *executor.Executor
}

// NewGQLGenExecutor returns a GraphQLExecutor backed by a gqlgen ExecutableSchema.
func NewGQLGenExecutor(es graphql.ExecutableSchema) GraphQLExecutor {
	return &gqlgenExecutor{
		exec: executor.New(es),
	}
}

// Execute runs a GraphQL query against the gqlgen schema.
func (g *gqlgenExecutor) Execute(ctx context.Context, query string, variables map[string]any) ([]byte, error) {
	params := &graphql.RawParams{
		Query:     query,
		Variables: variables,
	}

	ctx = graphql.StartOperationTrace(ctx)
	opCtx, errs := g.exec.CreateOperationContext(ctx, params)
	if errs != nil {
		msgs := make([]string, len(errs))
		for i, e := range errs {
			msgs[i] = e.Message
		}
		return nil, fmt.Errorf("graphql operation error: %s", strings.Join(msgs, "; "))
	}

	ctx = graphql.WithOperationContext(ctx, opCtx)
	handler, ctx := g.exec.DispatchOperation(ctx, opCtx)
	if handler == nil {
		return nil, fmt.Errorf("graphql operation aborted by middleware")
	}
	resp := handler(ctx)
	if resp == nil {
		return nil, fmt.Errorf("graphql operation returned nil response")
	}

	return json.Marshal(resp)
}
