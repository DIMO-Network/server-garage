package main

import (
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/DIMO-Network/server-garage/pkg/mcpserver"
	"github.com/modelcontextprotocol/go-sdk/mcp"
	"github.com/vektah/gqlparser/v2"
	"github.com/vektah/gqlparser/v2/ast"
)

// schemaAnalysis holds pre-computed metadata used to produce a more compact condensed SDL.
type schemaAnalysis struct {
	edgeTypes       map[string]bool // types matching the Relay Edge pattern
	connectionTypes map[string]bool // types matching the Relay Connection pattern
	signalTypes     map[string]bool // types that contain @isSignal fields
	hasDIDFields    bool            // true if any field description contains the DID format string
	queryReachable  map[string]bool // types reachable from query args / input types
}

// analyzeSchema pre-scans the schema to detect patterns that can be collapsed in the condensed output.
func analyzeSchema(schema *ast.Schema) schemaAnalysis {
	a := schemaAnalysis{
		edgeTypes:       make(map[string]bool),
		connectionTypes: make(map[string]bool),
		signalTypes:     make(map[string]bool),
		queryReachable:  computeQueryReachable(schema),
	}
	for name, def := range schema.Types {
		if def.Kind != ast.Object {
			continue
		}
		if isEdgeType(def) {
			a.edgeTypes[name] = true
		}
		if isConnectionType(def) {
			a.connectionTypes[name] = true
		}
		if hasSignalFields(def) {
			a.signalTypes[name] = true
		}
		if !a.hasDIDFields {
			for _, f := range def.Fields {
				if containsDIDFormat(f.Description) {
					a.hasDIDFields = true
					break
				}
			}
		}
	}

	return a
}

// computeQueryReachable finds all types that are reachable from query/mutation arguments
// or are input types, enums, scalars, interfaces, or unions. Response-only object types
// (leaf types that only appear in field return values) are excluded.
func computeQueryReachable(schema *ast.Schema) map[string]bool {
	reachable := make(map[string]bool)

	// Seed: all input objects, enums, scalars, interfaces, and unions are always relevant.
	for name, def := range schema.Types {
		switch def.Kind {
		case ast.InputObject, ast.Enum, ast.Scalar, ast.Interface, ast.Union:
			reachable[name] = true
		}
	}

	// Seed: types referenced by query/mutation/subscription field arguments.
	for _, opType := range []*ast.Definition{schema.Query, schema.Mutation, schema.Subscription} {
		if opType == nil {
			continue
		}
		for _, field := range opType.Fields {
			for _, arg := range field.Arguments {
				collectReferencedTypes(arg.Type, schema, reachable)
			}
		}
	}

	// Walk: transitively include types referenced by fields of already-reachable input objects.
	// (e.g., if an input type references another input type)
	changed := true
	for changed {
		changed = false
		for name := range reachable {
			def := schema.Types[name]
			if def == nil || def.Kind != ast.InputObject {
				continue
			}
			for _, f := range def.Fields {
				typeName := namedType(f.Type)
				if !reachable[typeName] {
					reachable[typeName] = true
					changed = true
				}
			}
		}
	}

	return reachable
}

// collectReferencedTypes adds the named type from a GraphQL type reference to the set.
func collectReferencedTypes(t *ast.Type, schema *ast.Schema, set map[string]bool) {
	name := namedType(t)
	set[name] = true
	// If this is an input object, also include its field types.
	if def, ok := schema.Types[name]; ok && def.Kind == ast.InputObject {
		for _, f := range def.Fields {
			childName := namedType(f.Type)
			if !set[childName] {
				collectReferencedTypes(f.Type, schema, set)
			}
		}
	}
}

// isEdgeType returns true if a type contains the Relay Edge fields: node: T! and cursor: String!.
func isEdgeType(def *ast.Definition) bool {
	hasNode, hasCursor := false, false
	for _, f := range def.Fields {
		if strings.HasPrefix(f.Name, "__") {
			continue
		}
		if f.Name == "node" && f.Type.NonNull {
			hasNode = true
		}
		if f.Name == "cursor" && f.Type.NonNull && f.Type.NamedType == "String" {
			hasCursor = true
		}
	}
	return hasNode && hasCursor
}

// isConnectionType returns true if a type contains the Relay Connection fields:
// totalCount, edges, nodes, pageInfo.
func isConnectionType(def *ast.Definition) bool {
	required := map[string]bool{"totalCount": false, "edges": false, "nodes": false, "pageInfo": false}
	for _, f := range def.Fields {
		if _, ok := required[f.Name]; ok {
			required[f.Name] = true
		}
	}
	for _, found := range required {
		if !found {
			return false
		}
	}
	return true
}

// hasSignalFields returns true if any field on the type has the @isSignal directive.
func hasSignalFields(def *ast.Definition) bool {
	for _, f := range def.Fields {
		if f.Directives.ForName("isSignal") != nil {
			return true
		}
	}
	return false
}

// loadGraphQLSchema loads .graphqls files and parses them into an ast.Schema.
func loadGraphQLSchema(paths []string) (*ast.Schema, error) {
	var sources []*ast.Source
	for _, p := range paths {
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
	return schema, nil
}

// extractTools finds Query fields annotated with @mcpTool and returns tool definitions.
// This is the same logic as parseSchema but accepts a pre-parsed *ast.Schema.
func extractTools(schema *ast.Schema, prefix string) ([]mcpserver.ToolDefinition, error) {
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
			return nil, fmt.Errorf("field %s: @mcpTool directive missing required argument(s)", field.Name)
		}

		toolName := nameArg.Value.Raw
		if prefix != "" {
			toolName = prefix + "_" + toolName
		}
		description := descArg.Value.Raw
		selection := selArg.Value.Raw
		isTemplated := isTemplatedSelection(selection)

		readOnly := true // default
		if readOnlyArg := dir.Arguments.ForName("readOnly"); readOnlyArg != nil {
			readOnly = readOnlyArg.Value.Raw == "true"
		}

		// Only validate selections that are plain text. Templated selections
		// can't be checked here because the rendered field names depend on
		// per-call argument values; a bad template is rejected by mcpserver
		// at registration time and bad rendered GraphQL is rejected by the
		// executor at call time.
		if selection != "" && !isTemplated {
			returnTypeName := namedType(field.Type)
			returnType := schema.Types[returnTypeName]
			if returnType == nil {
				return nil, fmt.Errorf("return type %s not found", returnTypeName)
			}
			if err := validateSelection(selection, returnType); err != nil {
				return nil, fmt.Errorf("field %s: %w", field.Name, err)
			}
		}

		var args []mcpserver.ArgDefinition
		for _, a := range field.Arguments {
			argDesc := a.Description
			if argDesc == "" {
				if a.Type.NonNull {
					argDesc = fmt.Sprintf("%s (%s, required)", a.Name, a.Type.String())
				} else {
					argDesc = fmt.Sprintf("%s (%s, optional)", a.Name, a.Type.String())
				}
			}
			jsonType, itemsType := mapGraphQLType(a.Type, schema)
			args = append(args, mcpserver.ArgDefinition{
				Name:        a.Name,
				Type:        jsonType,
				Description: argDesc,
				Required:    a.Type.NonNull,
				ItemsType:   itemsType,
				EnumValues:  enumValues(a.Type, schema),
			})
		}

		selectionTemplate := ""
		querySelection := selection
		if isTemplated {
			selectionTemplate = selection
			querySelection = mcpserver.SelectionPlaceholder
		}
		query := buildQueryString(field, querySelection)

		var annotations *mcp.ToolAnnotations
		if readOnly {
			f := false
			annotations = &mcp.ToolAnnotations{
				ReadOnlyHint:    true,
				DestructiveHint: &f,
				OpenWorldHint:   &f,
				IdempotentHint:  true,
			}
		}

		tools = append(tools, mcpserver.ToolDefinition{
			Name:              toolName,
			Description:       description,
			Args:              args,
			Query:             query,
			SelectionTemplate: selectionTemplate,
			Annotations:       annotations,
		})
	}

	return tools, nil
}

// generateCondensedSDL produces a compact GraphQL SDL representation of the schema.
// It strips directive definitions and introspection built-ins, omits deprecated fields,
// collapses Edge/Connection types, collapses @isSignal fields into compact tables,
// preserves @oneOf and default values, and inlines @mcpExample directives as comments.
func generateCondensedSDL(schema *ast.Schema) string {
	analysis := analyzeSchema(schema)
	var sb strings.Builder

	builtinTypes := map[string]bool{
		"String": true, "Int": true, "Float": true, "Boolean": true, "ID": true,
	}

	operationTypes := map[string]bool{}
	if schema.Query != nil {
		operationTypes[schema.Query.Name] = true
	}
	if schema.Mutation != nil {
		operationTypes[schema.Mutation.Name] = true
	}
	if schema.Subscription != nil {
		operationTypes[schema.Subscription.Name] = true
	}

	// Collect non-builtin type names, split into scalars and the rest.
	var scalarNames []string
	var typeNames []string
	for name := range schema.Types {
		if builtinTypes[name] || strings.HasPrefix(name, "__") || operationTypes[name] {
			continue
		}
		if analysis.edgeTypes[name] || analysis.connectionTypes[name] {
			continue
		}
		if schema.Types[name].Kind == ast.Scalar {
			scalarNames = append(scalarNames, name)
		} else {
			typeNames = append(typeNames, name)
		}
	}
	sort.Strings(scalarNames)
	sort.Strings(typeNames)

	// Emit scalars and convention notes first so LLMs have type context before seeing fields.
	for _, name := range scalarNames {
		writeTypeDefinition(&sb, schema.Types[name], &analysis)
	}

	// Convention notes for collapsed patterns.
	var notes []string
	if analysis.hasDIDFields {
		notes = append(notes, "# All tokenDID fields use the format did:erc721:<chainID>:<contractAddress>:<tokenId>")
	}
	if len(analysis.edgeTypes) > 0 {
		notes = append(notes, "# All *Edge types: { node: T!, cursor: String! }")
	}
	if len(analysis.connectionTypes) > 0 {
		notes = append(notes, "# All *Connection types: { totalCount: Int!, edges: [TEdge!]!, nodes: [T!]!, pageInfo: PageInfo! }")
	}
	if len(notes) > 0 {
		if sb.Len() > 0 {
			sb.WriteString("\n")
		}
		sb.WriteString(strings.Join(notes, "\n"))
		sb.WriteString("\n")
	}

	// Signal reference table next (before queries so LLMs see the signal catalog first).
	if len(analysis.signalTypes) > 0 {
		writeSignalReferenceTable(&sb, schema, &analysis)
	}

	// Then operations.
	if schema.Query != nil {
		if sb.Len() > 0 {
			sb.WriteString("\n")
		}
		writeOperationType(&sb, schema.Query)
	}
	if schema.Mutation != nil {
		if sb.Len() > 0 {
			sb.WriteString("\n")
		}
		writeOperationType(&sb, schema.Mutation)
	}
	if schema.Subscription != nil {
		if sb.Len() > 0 {
			sb.WriteString("\n")
		}
		writeOperationType(&sb, schema.Subscription)
	}

	// Then all other types.
	for _, name := range typeNames {
		if sb.Len() > 0 {
			sb.WriteString("\n")
		}
		writeTypeDefinition(&sb, schema.Types[name], &analysis)
	}

	return sb.String()
}
