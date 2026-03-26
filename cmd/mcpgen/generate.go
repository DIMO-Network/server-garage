package main

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"text/template"

	"github.com/DIMO-Network/server-garage/pkg/mcpserver"
	"github.com/vektah/gqlparser/v2"
	"github.com/vektah/gqlparser/v2/ast"
)

// parseSchema reads .graphqls files, finds Query fields annotated with @mcpTool,
// and returns a slice of ToolDefinition.
func parseSchema(schemaPaths []string, prefix string) ([]mcpserver.ToolDefinition, error) {
	var sources []*ast.Source
	for _, p := range schemaPaths {
		data, err := os.ReadFile(p)
		if err != nil {
			return nil, fmt.Errorf("reading %s: %w", p, err)
		}
		sources = append(sources, &ast.Source{
			Name:  filepath.Base(p),
			Input: string(data),
		})
	}

	schema, gqlErr := gqlparser.LoadSchema(sources...)
	if gqlErr != nil {
		return nil, fmt.Errorf("parsing schema: %s", gqlErr.Error())
	}

	queryType := schema.Query
	if queryType == nil {
		return nil, fmt.Errorf("no Query type found in schema")
	}

	var tools []mcpserver.ToolDefinition
	for _, field := range queryType.Fields {
		dir := field.Directives.ForName("mcpTool")
		if dir == nil {
			continue
		}

		nameArg := dir.Arguments.ForName("name")
		descArg := dir.Arguments.ForName("description")
		selArg := dir.Arguments.ForName("selection")
		if nameArg == nil || descArg == nil || selArg == nil {
			continue
		}

		toolName := nameArg.Value.Raw
		if prefix != "" {
			toolName = prefix + "_" + toolName
		}
		description := descArg.Value.Raw
		selection := selArg.Value.Raw

		// Validate selection against return type
		if selection != "" {
			returnType := schema.Types[field.Type.Name()]
			if returnType == nil {
				return nil, fmt.Errorf("return type %s not found", field.Type.Name())
			}
			if err := validateSelection(selection, returnType, schema); err != nil {
				return nil, fmt.Errorf("field %s: %w", field.Name, err)
			}
		}

		// Build args
		var args []mcpserver.ArgDefinition
		for _, a := range field.Arguments {
			argDesc := a.Description
			if argDesc == "" {
				argDesc = a.Name + " argument"
			}
			args = append(args, mcpserver.ArgDefinition{
				Name:        a.Name,
				Type:        mapGraphQLType(a.Type, schema),
				Description: argDesc,
				Required:    a.Type.NonNull,
			})
		}

		// Build query string
		query := buildQueryString(field, selection)

		tools = append(tools, mcpserver.ToolDefinition{
			Name:        toolName,
			Description: description,
			Args:        args,
			Query:       query,
		})
	}

	return tools, nil
}

// buildQueryString constructs a GraphQL query string from a field definition.
func buildQueryString(field *ast.FieldDefinition, selection string) string {
	var sb strings.Builder

	// Build variable declarations
	if len(field.Arguments) > 0 {
		sb.WriteString("query(")
		for i, a := range field.Arguments {
			if i > 0 {
				sb.WriteString(", ")
			}
			sb.WriteString("$")
			sb.WriteString(a.Name)
			sb.WriteString(": ")
			sb.WriteString(a.Type.String())
		}
		sb.WriteString(") { ")
	} else {
		sb.WriteString("{ ")
	}

	// Build field call
	sb.WriteString(field.Name)
	if len(field.Arguments) > 0 {
		sb.WriteString("(")
		for i, a := range field.Arguments {
			if i > 0 {
				sb.WriteString(", ")
			}
			sb.WriteString(a.Name)
			sb.WriteString(": $")
			sb.WriteString(a.Name)
		}
		sb.WriteString(")")
	}

	// Add selection set
	if selection != "" {
		sb.WriteString(" { ")
		sb.WriteString(selection)
		sb.WriteString(" }")
	}

	sb.WriteString(" }")
	return sb.String()
}

// validateSelection checks that top-level field names in the selection exist on the type.
func validateSelection(selection string, typeDef *ast.Definition, schema *ast.Schema) error {
	fields := extractTopLevelFields(selection)
	for _, f := range fields {
		found := false
		for _, tf := range typeDef.Fields {
			if tf.Name == f {
				found = true
				break
			}
		}
		if !found {
			return fmt.Errorf("field %q not found on type %s", f, typeDef.Name)
		}
	}
	return nil
}

// extractTopLevelFields extracts top-level field names from a selection string,
// skipping nested { } blocks.
func extractTopLevelFields(selection string) []string {
	var fields []string
	depth := 0
	tokens := strings.Fields(selection)
	for _, tok := range tokens {
		if tok == "{" {
			depth++
			continue
		}
		if tok == "}" {
			depth--
			continue
		}
		if depth == 0 {
			// Strip any trailing { that might be attached
			clean := strings.TrimSuffix(tok, "{")
			if clean != "" {
				fields = append(fields, clean)
			}
		}
	}
	return fields
}

// mapGraphQLType converts a GraphQL type to a JSON Schema type string.
// Custom scalars (Time, Address, etc.) and enums are mapped to "string"
// since they serialize as strings in JSON.
func mapGraphQLType(t *ast.Type, schema *ast.Schema) string {
	name := t.Name()
	switch name {
	case "Int":
		return "integer"
	case "Float":
		return "number"
	case "Boolean":
		return "boolean"
	case "String", "ID":
		return "string"
	default:
		if def, ok := schema.Types[name]; ok {
			if def.Kind == ast.Scalar || def.Kind == ast.Enum {
				return "string"
			}
		}
		return "object"
	}
}

var goFileTemplate = template.Must(template.New("mcptools").Parse(`// Code generated by mcpgen. DO NOT EDIT.
package {{.Package}}

import "github.com/DIMO-Network/server-garage/pkg/mcpserver"

var MCPTools = []mcpserver.ToolDefinition{
{{- range .Tools}}
	{
		Name:        {{printf "%q" .Name}},
		Description: {{printf "%q" .Description}},
		Args: []mcpserver.ArgDefinition{
		{{- range .Args}}
			{Name: {{printf "%q" .Name}}, Type: {{printf "%q" .Type}}, Description: {{printf "%q" .Description}}, Required: {{.Required}}},
		{{- end}}
		},
		Query: {{printf "%q" .Query}},
	},
{{- end}}
}
`))

type templateData struct {
	Package string
	Tools   []mcpserver.ToolDefinition
}

// generateGoFile renders a Go source file containing the tool definitions.
func generateGoFile(pkg string, tools []mcpserver.ToolDefinition) (string, error) {
	var sb strings.Builder
	if err := goFileTemplate.Execute(&sb, templateData{
		Package: pkg,
		Tools:   tools,
	}); err != nil {
		return "", fmt.Errorf("executing template: %w", err)
	}
	return sb.String(), nil
}
