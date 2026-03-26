package mcpserver

import (
	"context"
	"encoding/json"

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

	opCtx, errs := g.exec.CreateOperationContext(ctx, params)
	if errs != nil {
		resp := g.exec.DispatchError(graphql.WithOperationContext(ctx, opCtx), errs)
		return json.Marshal(resp)
	}

	ctx = graphql.WithOperationContext(ctx, opCtx)
	handler, ctx := g.exec.DispatchOperation(ctx, opCtx)
	resp := handler(ctx)

	return json.Marshal(resp)
}
