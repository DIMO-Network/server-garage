package mcpserver

import (
	"context"
	"fmt"
	"sync"
)

// schemaCache caches the result of a GraphQL introspection query.
type schemaCache struct {
	exec   GraphQLExecutor
	mu     sync.Mutex
	done   bool
	cached string
}

// getSchema returns the GraphQL schema via introspection. The result is cached
// after the first successful call.
func (s *schemaCache) getSchema(ctx context.Context) (string, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	if s.done {
		return s.cached, nil
	}

	result, err := s.exec.Execute(ctx, introspectionQuery, nil)
	if err != nil {
		return "", fmt.Errorf("introspection query: %w", err)
	}

	s.cached = string(result)
	s.done = true

	return s.cached, nil
}

// introspectionQuery is the full introspection query used to fetch the schema.
const introspectionQuery = `
query IntrospectionQuery {
  __schema {
    queryType { name }
    mutationType { name }
    subscriptionType { name }
    types {
      ...FullType
    }
    directives {
      name
      description
      locations
      args {
        ...InputValue
      }
    }
  }
}

fragment FullType on __Type {
  kind
  name
  description
  fields(includeDeprecated: true) {
    name
    description
    args {
      ...InputValue
    }
    type {
      ...TypeRef
    }
    isDeprecated
    deprecationReason
  }
  inputFields {
    ...InputValue
  }
  interfaces {
    ...TypeRef
  }
  enumValues(includeDeprecated: true) {
    name
    description
    isDeprecated
    deprecationReason
  }
  possibleTypes {
    ...TypeRef
  }
}

fragment InputValue on __InputValue {
  name
  description
  type { ...TypeRef }
  defaultValue
}

fragment TypeRef on __Type {
  kind
  name
  ofType {
    kind
    name
    ofType {
      kind
      name
      ofType {
        kind
        name
        ofType {
          kind
          name
          ofType {
            kind
            name
            ofType {
              kind
              name
            }
          }
        }
      }
    }
  }
}
`
