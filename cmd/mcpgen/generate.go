package main

import (
	"fmt"
	"go/format"
	"strings"
	"text/template"
	"unicode"

	"github.com/DIMO-Network/server-garage/pkg/mcpserver"
	"github.com/vektah/gqlparser/v2/ast"
)

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

// isTemplatedSelection reports whether a selection string contains Go
// text/template action delimiters. When true, mcpgen emits it as a
// SelectionTemplate rendered per call rather than a static selection set.
func isTemplatedSelection(selection string) bool {
	return strings.Contains(selection, "{{")
}

// validateSelection checks that top-level field names in the selection exist on the type.
func validateSelection(selection string, typeDef *ast.Definition) error {
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

func isIdentChar(r rune) bool {
	return unicode.IsLetter(r) || unicode.IsDigit(r) || r == '_'
}

// extractTopLevelFields extracts top-level field names from a selection string,
// skipping nested { } blocks, fragment spreads, aliases, and directives.
func extractTopLevelFields(selection string) []string {
	var fields []string
	depth := 0
	parenDepth := 0
	skipCount := 0
	tokens := strings.Fields(selection)
	for _, tok := range tokens {
		// Save previous depth state to determine if we were already nested.
		prevDepth := depth
		prevParenDepth := parenDepth

		// Track brace and paren depth from characters in this token.
		for _, ch := range tok {
			switch ch {
			case '{':
				depth++
			case '}':
				depth--
			case '(':
				parenDepth++
			case ')':
				parenDepth--
			}
		}

		// Decrement skipCount before the depth check so it is always
		// consumed, even when the skipped token also changes depth.
		if skipCount > 0 {
			skipCount--
			continue
		}

		// Skip if we were inside a nested block before this token,
		// or if this token opened/is inside a nested block.
		if prevDepth > 0 || prevParenDepth > 0 || depth > 0 || parenDepth > 0 {
			continue
		}

		// Fragment spreads: "..." (followed by "on Type"), "...on Type", or "...FragName".
		if strings.HasPrefix(tok, "...") {
			rest := tok[3:]
			if rest == "" {
				// Bare "..." — skip "on" and type name.
				skipCount = 2
			} else if strings.HasPrefix(rest, "on") && (len(rest) == 2 || !isIdentChar(rune(rest[2]))) {
				// "...on Type" or "...onSomeType" — skip the type name.
				skipCount = 1
			}
			// "...FragmentName" — just skip the token itself (no trailing tokens to skip).
			continue
		}

		// Skip directives (e.g., @skip, @include).
		if strings.HasPrefix(tok, "@") {
			continue
		}

		// Skip aliases — tokens ending with ":".
		if strings.HasSuffix(tok, ":") {
			continue
		}

		// Clean attached braces and parens from field name.
		clean := strings.TrimRight(tok, "{(")
		if clean != "" && clean != "}" {
			fields = append(fields, clean)
		}
	}
	return fields
}

// namedType unwraps list types (Elem) until it reaches a named type.
func namedType(t *ast.Type) string {
	for t.Elem != nil {
		t = t.Elem
	}
	return t.NamedType
}

// mapScalarName maps a GraphQL scalar/type name to a JSON Schema type string.
func mapScalarName(name string, schema *ast.Schema) string {
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

// mapGraphQLType converts a GraphQL type to a JSON Schema type string and optional items type.
// For list types, it returns ("array", elemType).
func mapGraphQLType(t *ast.Type, schema *ast.Schema) (jsonType, itemsType string) {
	if t.Elem != nil {
		elemType, _ := mapGraphQLType(t.Elem, schema)
		return "array", elemType
	}
	return mapScalarName(t.NamedType, schema), ""
}

// enumValues returns the allowed values for an enum type, or nil if not an enum.
// Deprecated enum values are excluded.
func enumValues(t *ast.Type, schema *ast.Schema) []string {
	name := namedType(t)
	def, ok := schema.Types[name]
	if !ok || def.Kind != ast.Enum {
		return nil
	}
	var vals []string
	for _, v := range def.EnumValues {
		if v.Directives.ForName("deprecated") != nil {
			continue
		}
		vals = append(vals, v.Name)
	}
	if len(vals) == 0 {
		return nil
	}
	return vals
}

var goFileTemplate = template.Must(template.New("mcptools").Funcs(template.FuncMap{
	"deref": func(b *bool) bool { return *b },
}).Parse(`// Code generated by mcpgen. DO NOT EDIT.
package {{.Package}}

import (
	"github.com/DIMO-Network/server-garage/pkg/mcpserver"
	"github.com/modelcontextprotocol/go-sdk/mcp"
)

func boolPtr(b bool) *bool { return &b }

var MCPTools = []mcpserver.ToolDefinition{
{{- range .Tools}}
	{
		Name:        {{printf "%q" .Name}},
		Description: {{printf "%q" .Description}},
		Args: []mcpserver.ArgDefinition{
		{{- range .Args}}
			{Name: {{printf "%q" .Name}}, Type: {{printf "%q" .Type}}, Description: {{printf "%q" .Description}}, Required: {{.Required}}, ItemsType: {{printf "%q" .ItemsType}}{{if .EnumValues}}, EnumValues: []string{ {{- range $i, $v := .EnumValues}}{{if $i}}, {{end}}{{printf "%q" $v}}{{end -}} }{{end}}},
		{{- end}}
		},
		Query: {{printf "%q" .Query}},
		{{- if .SelectionTemplate}}
		SelectionTemplate: {{printf "%q" .SelectionTemplate}},
		{{- end}}
		{{- if .Annotations}}
		Annotations: &mcp.ToolAnnotations{
			ReadOnlyHint:    {{.Annotations.ReadOnlyHint}},
			{{- if .Annotations.DestructiveHint}}
			DestructiveHint: boolPtr({{deref .Annotations.DestructiveHint}}),
			{{- end}}
			{{- if .Annotations.OpenWorldHint}}
			OpenWorldHint:   boolPtr({{deref .Annotations.OpenWorldHint}}),
			{{- end}}
			IdempotentHint:  {{.Annotations.IdempotentHint}},
		},
		{{- end}}
	},
{{- end}}
}

var CondensedSchema = {{printf "%q" .CondensedSchema}}
`))

type templateData struct {
	Package         string
	Tools           []mcpserver.ToolDefinition
	CondensedSchema string
}

// generateGoFile renders a Go source file containing tool definitions and condensed schema.
func generateGoFile(pkg string, tools []mcpserver.ToolDefinition, condensedSchema string) (string, error) {
	var sb strings.Builder
	if err := goFileTemplate.Execute(&sb, templateData{
		Package:         pkg,
		Tools:           tools,
		CondensedSchema: condensedSchema,
	}); err != nil {
		return "", fmt.Errorf("executing template: %w", err)
	}
	formatted, err := format.Source([]byte(sb.String()))
	if err != nil {
		return "", fmt.Errorf("formatting generated code: %w", err)
	}
	return string(formatted), nil
}
