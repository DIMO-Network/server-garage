package main

import (
	"testing"

	"github.com/DIMO-Network/server-garage/pkg/mcpserver"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/vektah/gqlparser/v2/ast"
)

// loadTools is a test helper that loads schema files and extracts tools.
func loadTools(t *testing.T, paths []string, prefix string) []mcpserver.ToolDefinition {
	t.Helper()
	schema, err := loadGraphQLSchema(paths)
	require.NoError(t, err)
	tools, err := extractTools(schema, prefix)
	require.NoError(t, err)
	return tools
}

func TestParseDirective(t *testing.T) {
	tools := loadTools(t, []string{"testdata/basic.graphqls"}, "")
	require.Len(t, tools, 2)

	// hello tool — no selection, one arg
	assert.Equal(t, "hello", tools[0].Name)
	assert.Equal(t, "Say hello to someone", tools[0].Description)
	require.Len(t, tools[0].Args, 1)
	assert.Equal(t, "name", tools[0].Args[0].Name)
	assert.Equal(t, "string", tools[0].Args[0].Type)
	assert.True(t, tools[0].Args[0].Required)
	assert.Equal(t, `query($name: String!) { hello(name: $name) }`, tools[0].Query)

	// vehicle_info tool — with selection set
	assert.Equal(t, "vehicle_info", tools[1].Name)
	assert.Equal(t, "Get vehicle info", tools[1].Description)
	require.Len(t, tools[1].Args, 1)
	assert.Equal(t, "tokenId", tools[1].Args[0].Name)
	assert.Equal(t, "integer", tools[1].Args[0].Type)
	assert.True(t, tools[1].Args[0].Required)
	assert.Equal(t, `query($tokenId: Int!) { vehicle(tokenId: $tokenId) { tokenId owner } }`, tools[1].Query)
}

func TestPrefixApplied(t *testing.T) {
	tools := loadTools(t, []string{"testdata/basic.graphqls"}, "telemetry")
	require.Len(t, tools, 2)

	assert.Equal(t, "telemetry_hello", tools[0].Name)
	assert.Equal(t, "telemetry_vehicle_info", tools[1].Name)
}

func TestArgDescriptionFromDocString(t *testing.T) {
	tools := loadTools(t, []string{"testdata/docstrings.graphqls"}, "")
	require.Len(t, tools, 1)
	require.Len(t, tools[0].Args, 1)

	assert.Equal(t, "The vehicle's NFT token ID", tools[0].Args[0].Description)
}

func TestArgDescriptionFallback(t *testing.T) {
	tools := loadTools(t, []string{"testdata/basic.graphqls"}, "")
	require.Len(t, tools, 2)

	assert.Equal(t, "name (String!, required)", tools[0].Args[0].Description)
	assert.Equal(t, "tokenId (Int!, required)", tools[1].Args[0].Description)
}

func TestInvalidSelection(t *testing.T) {
	schema, err := loadGraphQLSchema([]string{"testdata/invalid_selection.graphqls"})
	require.NoError(t, err)
	_, err = extractTools(schema, "")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "nonExistentField")
}

func TestGeneratedOutput(t *testing.T) {
	tools := loadTools(t, []string{"testdata/basic.graphqls"}, "test")

	output, err := generateGoFile("graph", tools, "")
	require.NoError(t, err)

	assert.Contains(t, output, "package graph")
	assert.Contains(t, output, `"github.com/DIMO-Network/server-garage/pkg/mcpserver"`)
	assert.Contains(t, output, "DO NOT EDIT")
	assert.Contains(t, output, "test_hello")
	assert.Contains(t, output, "test_vehicle_info")
}

func TestListReturnTypeWithSelection(t *testing.T) {
	tools := loadTools(t, []string{"testdata/list_types.graphqls"}, "")
	require.Len(t, tools, 1)

	tool := tools[0]
	assert.Equal(t, "list_vehicles", tool.Name)
	require.Len(t, tool.Args, 1)
	assert.Equal(t, "array", tool.Args[0].Type)
	assert.Equal(t, "integer", tool.Args[0].ItemsType)
}

func TestExtractTopLevelFields(t *testing.T) {
	tests := []struct {
		name     string
		input    string
		expected []string
	}{
		{
			name:     "simple fields",
			input:    "id name",
			expected: []string{"id", "name"},
		},
		{
			name:     "nested braces",
			input:    "id vehicles { make model }",
			expected: []string{"id", "vehicles"},
		},
		{
			name:     "inline fragment with space",
			input:    "... on Vehicle { id } name",
			expected: []string{"name"},
		},
		{
			name:     "inline fragment no space",
			input:    "...on Vehicle { id } name",
			expected: []string{"name"},
		},
		{
			name:     "alias",
			input:    "alias: name id",
			expected: []string{"name", "id"},
		},
		{
			name:     "directive",
			input:    "name @skip(if: true) id",
			expected: []string{"name", "id"},
		},
		{
			name:     "named fragment spread",
			input:    "id ...VehicleFields name",
			expected: []string{"id", "name"},
		},
		{
			name:     "fragment with attached brace",
			input:    "... on Vehicle{ id } name",
			expected: []string{"name"},
		},
		{
			name:     "empty",
			input:    "",
			expected: nil,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := extractTopLevelFields(tt.input)
			assert.Equal(t, tt.expected, result)
		})
	}
}

func TestExtractTopLevelFieldsInlineFragment(t *testing.T) {
	fields := extractTopLevelFields("...onVehicle { id } name")
	assert.Equal(t, []string{"name"}, fields)
}

func TestEnumValuesExtracted(t *testing.T) {
	tools := loadTools(t, []string{"testdata/all_types.graphqls"}, "")
	require.Len(t, tools, 1)

	statusArg := tools[0].Args[5]
	assert.Equal(t, "status", statusArg.Name)
	assert.Equal(t, "string", statusArg.Type)
	assert.Equal(t, []string{"ACTIVE", "INACTIVE"}, statusArg.EnumValues)

	nameArg := tools[0].Args[0]
	assert.Equal(t, "name", nameArg.Name)
	assert.Nil(t, nameArg.EnumValues)
}

func TestDeprecatedEnumValuesExcluded(t *testing.T) {
	schema, err := loadGraphQLSchema([]string{"testdata/deprecated.graphqls"})
	require.NoError(t, err)

	// The Status enum has ACTIVE, INACTIVE, and PENDING @deprecated.
	def := schema.Types["Status"]
	require.NotNil(t, def)

	vals := enumValues(&ast.Type{NamedType: "Status"}, schema)
	assert.Equal(t, []string{"ACTIVE", "INACTIVE"}, vals)
	assert.NotContains(t, vals, "PENDING")
}

func TestGeneratedCodeIncludesAnnotations(t *testing.T) {
	schema, err := loadGraphQLSchema([]string{"testdata/basic.graphqls"})
	require.NoError(t, err)

	tools, err := extractTools(schema, "test")
	require.NoError(t, err)
	require.Len(t, tools, 2)
	require.NotNil(t, tools[0].Annotations, "tool should have annotations")
	assert.True(t, tools[0].Annotations.ReadOnlyHint)
}

func TestGeneratedCodeWithAnnotationsCompiles(t *testing.T) {
	schema, err := loadGraphQLSchema([]string{"testdata/basic.graphqls"})
	require.NoError(t, err)

	tools, err := extractTools(schema, "test")
	require.NoError(t, err)

	condensed := generateCondensedSDL(schema)
	output, err := generateGoFile("testpkg", tools, condensed)
	require.NoError(t, err)
	assert.Contains(t, output, "mcp.ToolAnnotations")
	assert.Contains(t, output, "ReadOnlyHint")
	assert.Contains(t, output, "boolPtr")
}

func TestTemplatedSelection(t *testing.T) {
	tools := loadTools(t, []string{"testdata/templated_selection.graphqls"}, "")
	require.Len(t, tools, 1)

	tool := tools[0]
	assert.Equal(t, "get_signals", tool.Name)
	assert.Equal(t, "timestamp {{range .signalRequests}} {{.name}}(agg: {{.agg}}) {{end}}", tool.SelectionTemplate)
	assert.Contains(t, tool.Query, "__MCPGEN_SELECTION__", "Query should carry the selection placeholder")
	assert.NotContains(t, tool.Query, "{{", "Query should not contain the raw template")
}

func TestTemplatedSelectionGeneratedOutput(t *testing.T) {
	tools := loadTools(t, []string{"testdata/templated_selection.graphqls"}, "test")
	output, err := generateGoFile("graph", tools, "")
	require.NoError(t, err)
	assert.Contains(t, output, "SelectionTemplate:")
	assert.Contains(t, output, "__MCPGEN_SELECTION__")
}

func TestMapGraphQLTypeAllScalars(t *testing.T) {
	tools := loadTools(t, []string{"testdata/all_types.graphqls"}, "")
	require.Len(t, tools, 1)

	args := tools[0].Args
	require.Len(t, args, 7)

	expected := []struct {
		name      string
		typ       string
		itemsType string
	}{
		{"name", "string", ""},
		{"limit", "integer", ""},
		{"score", "number", ""},
		{"active", "boolean", ""},
		{"since", "string", ""},
		{"status", "string", ""},
		{"ids", "array", "integer"},
	}

	for i, exp := range expected {
		assert.Equal(t, exp.name, args[i].Name, "arg %d name", i)
		assert.Equal(t, exp.typ, args[i].Type, "arg %d type", i)
		assert.Equal(t, exp.itemsType, args[i].ItemsType, "arg %d itemsType", i)
	}
}
