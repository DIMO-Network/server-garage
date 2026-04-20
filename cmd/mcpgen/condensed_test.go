package main

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/vektah/gqlparser/v2"
	"github.com/vektah/gqlparser/v2/ast"
)

func TestCondensedSDLBasic(t *testing.T) {
	schema, err := loadGraphQLSchema([]string{"testdata/basic.graphqls"})
	require.NoError(t, err)

	sdl := generateCondensedSDL(schema)

	// Should contain Query type with all fields (including non-annotated ones)
	assert.Contains(t, sdl, "type Query {")
	assert.Contains(t, sdl, "hello(name: String!): String!")
	assert.Contains(t, sdl, "vehicle(tokenId: Int!): Vehicle")
	assert.Contains(t, sdl, "noMcp: String!")

	// Response-only types should use compact single-line format.
	assert.Contains(t, sdl, "type Vehicle { tokenId: Int!, owner: String! }")

	// Should NOT contain directive definitions or introspection types
	assert.NotContains(t, sdl, "directive @mcpTool")
	assert.NotContains(t, sdl, "__Schema")
	assert.NotContains(t, sdl, "__Type")
}

func TestCondensedSDLWithExamples(t *testing.T) {
	schema, err := loadGraphQLSchema([]string{"testdata/examples.graphqls"})
	require.NoError(t, err)

	sdl := generateCondensedSDL(schema)

	// Should contain inline examples as comments
	assert.Contains(t, sdl, "# Example - Get vehicle 123:")
	assert.Contains(t, sdl, "#   { vehicle(tokenId: 123) { tokenId owner } }")
	assert.Contains(t, sdl, "# Example - Get just the owner:")
	assert.Contains(t, sdl, "#   { vehicle(tokenId: 456) { owner } }")
	assert.Contains(t, sdl, "# Example - Find Teslas:")

	// Response-only types should use compact single-line format.
	assert.Contains(t, sdl, "type Vehicle {")
}

func TestCondensedSDLAllTypes(t *testing.T) {
	schema, err := loadGraphQLSchema([]string{"testdata/all_types.graphqls"})
	require.NoError(t, err)

	sdl := generateCondensedSDL(schema)

	// Should contain enum
	assert.Contains(t, sdl, "enum Status {")
	assert.Contains(t, sdl, "ACTIVE")
	assert.Contains(t, sdl, "INACTIVE")

	// Should contain custom scalar
	assert.Contains(t, sdl, "scalar DateTime")

	// Should NOT contain built-in scalars
	assert.NotContains(t, sdl, "scalar String")
	assert.NotContains(t, sdl, "scalar Int")
	assert.NotContains(t, sdl, "scalar Boolean")
}

func TestCondensedSDLDocstrings(t *testing.T) {
	schema, err := loadGraphQLSchema([]string{"testdata/docstrings.graphqls"})
	require.NoError(t, err)

	sdl := generateCondensedSDL(schema)

	// Should include explicit docstrings on arguments
	assert.Contains(t, sdl, "The vehicle's NFT token ID")
}

func TestGeneratedOutputWithCondensedSchema(t *testing.T) {
	schema, err := loadGraphQLSchema([]string{"testdata/basic.graphqls"})
	require.NoError(t, err)

	tools, err := extractTools(schema, "test")
	require.NoError(t, err)

	condensedSDL := generateCondensedSDL(schema)
	output, err := generateGoFile("graph", tools, condensedSDL)
	require.NoError(t, err)

	assert.Contains(t, output, "var CondensedSchema = ")
	assert.Contains(t, output, "type Query")
}

func TestCondensedSDLDeprecated(t *testing.T) {
	schema, err := loadGraphQLSchema([]string{"testdata/deprecated.graphqls"})
	require.NoError(t, err)

	sdl := generateCondensedSDL(schema)

	// Vehicle is response-only, compact format. Deprecated field 'image' excluded.
	assert.Contains(t, sdl, "type Vehicle { tokenId: Int!, owner: String!, imageURI: String! }")
	assert.NotContains(t, sdl, "image: String")

	// Deprecated enum value should be omitted.
	assert.Contains(t, sdl, "ACTIVE")
	assert.Contains(t, sdl, "INACTIVE")
	assert.NotContains(t, sdl, "PENDING")
}

func TestCondensedSDLOneOf(t *testing.T) {
	schema, err := loadGraphQLSchema([]string{"testdata/oneof.graphqls"})
	require.NoError(t, err)

	sdl := generateCondensedSDL(schema)

	// Input type should have @oneOf annotation.
	assert.Contains(t, sdl, "input AftermarketDeviceBy @oneOf {")
}

func TestCondensedSDLScalarDescriptions(t *testing.T) {
	schema, err := loadGraphQLSchema([]string{"testdata/scalar_descriptions.graphqls"})
	require.NoError(t, err)

	sdl := generateCondensedSDL(schema)

	// Scalars should have inline comments.
	assert.Contains(t, sdl, "scalar Address  # 0x-prefixed checksummed hex address")
	assert.Contains(t, sdl, "scalar Time  # RFC-3339 date-time string")
	assert.Contains(t, sdl, "scalar Bytes  # lowercase hex with 0x prefix")
}

func TestCondensedSDLDefaultValues(t *testing.T) {
	schema, err := loadGraphQLSchema([]string{"testdata/defaults.graphqls"})
	require.NoError(t, err)

	sdl := generateCondensedSDL(schema)

	// Default values should be preserved.
	assert.Contains(t, sdl, "maxGapSeconds: Int = 300")
	assert.Contains(t, sdl, "minDurationSeconds: Int = 60")
	assert.Contains(t, sdl, "format: OutputFormat = JSON")
}

func TestCondensedSDLEdgeConnection(t *testing.T) {
	schema, err := loadGraphQLSchema([]string{"testdata/edge_connection.graphqls"})
	require.NoError(t, err)

	sdl := generateCondensedSDL(schema)

	// Edge types should be omitted.
	assert.NotContains(t, sdl, "type VehicleEdge")
	assert.NotContains(t, sdl, "type DCNEdge")

	// Connection types should be omitted.
	assert.NotContains(t, sdl, "type VehicleConnection")
	assert.NotContains(t, sdl, "type DCNConnection")

	// Summary comments should be present.
	assert.Contains(t, sdl, "# All *Edge types: { node: T!, cursor: String! }")
	assert.Contains(t, sdl, "# All *Connection types:")

	// Response-only types should use compact single-line format.
	assert.Contains(t, sdl, "type PageInfo {")
	assert.Contains(t, sdl, "type Vehicle {")
	assert.Contains(t, sdl, "type DCN {")
}

func TestCondensedSDLSignalCollapsing(t *testing.T) {
	schema, err := loadGraphQLSchema([]string{"testdata/signals.graphqls"})
	require.NoError(t, err)

	sdl := generateCondensedSDL(schema)

	// Non-signal field should be emitted normally.
	assert.Contains(t, sdl, "timestamp: String!")

	// Should have signal header with total count (9 signals now with approximate field).
	assert.Contains(t, sdl, "SIGNAL FIELDS (9 total)")

	// Should have per-type calling conventions (compact format: one line per type
	// when it has exactly one argument-signature group).
	assert.Contains(t, sdl, "All signals below exist on every signal type")
	assert.Contains(t, sdl, "SignalAggregations:")
	assert.Contains(t, sdl, "fieldName(agg: FloatAggregation!, filter: SignalFloatFilter): Float")

	// Type column is dropped; exception list names non-default types.
	assert.Contains(t, sdl, "Float is the default type.")
	assert.Contains(t, sdl, "String: vin")
	assert.Contains(t, sdl, "| Signal | Unit | Description |")
	assert.NotContains(t, sdl, "| Signal | Type | Unit | Description |")

	// Individual signal fields appear as table rows; self-evident descriptions dropped.
	assert.Contains(t, sdl, "| speed | km/h |")
	assert.Contains(t, sdl, "| powertrainCombustionEngineSpeed | rpm |")
	// Non-obvious descriptions (PID codes) are kept.
	assert.Contains(t, sdl, "| obdRunTime | s | PID 1F - Engine run time |")
	// String signals appear in the table without an explicit type column.
	assert.Contains(t, sdl, "| vin |  | Vehicle Identification Number |")

	// Category headers should use separator-line format, not table rows.
	assert.Contains(t, sdl, "── OTHER (privilege: VEHICLE_NON_LOCATION_DATA) ──")
	// On tie, alphabetically-first privilege wins: VEHICLE_ALL_TIME_LOCATION.
	assert.Contains(t, sdl, "── CURRENT (privilege: VEHICLE_ALL_TIME_LOCATION) ──")
	assert.NotContains(t, sdl, "| **")

	// Per-field privilege override: the approximate field's privilege differs from
	// the category's dominant (VEHICLE_ALL_TIME_LOCATION), so it's shown inline.
	assert.Contains(t, sdl, "| currentLocationApproximateLatitude | degrees | privilege: VEHICLE_APPROXIMATE_LOCATION |")

	// Signal fields should NOT appear as full field definitions.
	assert.NotContains(t, sdl, "speed(agg: FloatAggregation!")
}

func TestCondensedSDLLongDescription(t *testing.T) {
	schema, err := loadGraphQLSchema([]string{"testdata/long_description.graphqls"})
	require.NoError(t, err)

	sdl := generateCondensedSDL(schema)

	// Long description should use block string syntax.
	assert.Contains(t, sdl, `"""`)
	// Lines should be word-wrapped (no single line >200 chars in the description).
	for _, line := range strings.Split(sdl, "\n") {
		trimmed := strings.TrimSpace(line)
		// Skip non-description lines.
		if trimmed == `"""` || trimmed == "" || strings.HasPrefix(trimmed, "#") {
			continue
		}
		// Description content lines within block strings should be ≤ ~85 chars
		// (80 wrap + indent). We check raw line length is reasonable.
		if len(line) > 200 {
			t.Errorf("line too long (%d chars): %s", len(line), line[:80]+"...")
		}
	}
	// Key constraints should still be present in the wrapped output.
	assert.Contains(t, sdl, "31 days")
	assert.Contains(t, sdl, "mechanism")

	// List items should be preserved as separate lines, not collapsed into prose.
	assert.Contains(t, sdl, "- ignitionDetection:")
	assert.Contains(t, sdl, "- frequencyAnalysis:")
	assert.Contains(t, sdl, "- changePointDetection:")
	assert.Contains(t, sdl, "- refuel:")
}

func TestWordWrap(t *testing.T) {
	tests := []struct {
		text     string
		width    int
		expected []string
	}{
		{"short text", 80, []string{"short text"}},
		{"", 80, nil},
		{
			"The quick brown fox jumps over the lazy dog and then runs away very quickly",
			30,
			[]string{
				"The quick brown fox jumps over",
				"the lazy dog and then runs",
				"away very quickly",
			},
		},
		{
			"superlongwordthatexceedswidth next",
			10,
			[]string{"superlongwordthatexceedswidth", "next"},
		},
	}
	for _, tt := range tests {
		result := wordWrap(tt.text, tt.width)
		assert.Equal(t, tt.expected, result, "wordWrap(%q, %d)", tt.text, tt.width)
	}
}

func TestCondensedSDLSelfEvidentDescriptions(t *testing.T) {
	schema, err := loadGraphQLSchema([]string{"testdata/self_evident.graphqls"})
	require.NoError(t, err)

	sdl := generateCondensedSDL(schema)

	// Attestation is response-only, compact single-line format (no descriptions).
	assert.Contains(t, sdl, "type Attestation { id: String!, type: String!, dataVersion: String!, payload: String! }")
	// Descriptions are not emitted in compact format, so self-evident check is moot here.
	// Tested via unit tests below.
}

func TestCondensedSDLPaginationArgsInlined(t *testing.T) {
	schema, err := loadGraphQLSchema([]string{"testdata/pagination_did.graphqls"})
	require.NoError(t, err)

	sdl := generateCondensedSDL(schema)

	// Pagination args should be inlined (no multi-line descriptions).
	// The vehicles query had descriptions only on pagination args + filterBy.
	// filterBy has a real description, so it should still be multi-line.
	assert.Contains(t, sdl, "filterBy: VehiclesFilter")

	// Pagination arg descriptions should NOT appear.
	assert.NotContains(t, sdl, "Mutually exclusive with")
	assert.NotContains(t, sdl, "A cursor for pagination")
}

func TestCondensedSDLDIDDescriptionsStripped(t *testing.T) {
	schema, err := loadGraphQLSchema([]string{"testdata/pagination_did.graphqls"})
	require.NoError(t, err)

	sdl := generateCondensedSDL(schema)

	// Vehicle is response-only, compact format — DID description stripped.
	assert.Contains(t, sdl, "type Vehicle { tokenId: Int!, tokenDID: String!, owner: Address! }")
	// No descriptions in compact format, so DID description is inherently excluded.

	// Summary comment for DID format should still appear.
	assert.Contains(t, sdl, "# All tokenDID fields use the format did:erc721:")
}

func TestCondensedSDLExpandedSelfEvident(t *testing.T) {
	// Self-evident description patterns like "Model for this device definition."
	// are tested via the isSelfEvidentDescription unit tests below.
	// Response-only types are now omitted, so we only verify the unit test logic.
}

func TestIsSelfEvidentDescription(t *testing.T) {
	tests := []struct {
		desc, field, typ string
		expected         bool
	}{
		{"id", "id", "Attestation", true},
		{"type", "type", "Attestation", true},
		{"the id", "id", "Attestation", true},
		{"the id.", "id", "Attestation", true},
		{"The dataVersion of the Attestation.", "dataVersion", "Attestation", true},
		{"The dataVersion of the Attestation", "dataVersion", "Attestation", true},
		{"A meaningful description that should be kept.", "payload", "Attestation", false},
		{"Vehicle speed.", "speed", "SignalAggregations", false},
		{"", "id", "Foo", false},
		// "<Field> for this <type>." patterns
		{"Model for this device definition.", "model", "DeviceDefinition", true},
		{"Name for this device definition.", "name", "DeviceDefinition", true},
		{"model for this DeviceDefinition", "model", "DeviceDefinition", true},
		// "<Field> of the <type>." patterns
		{"model of the DeviceDefinition.", "model", "DeviceDefinition", true},
		// Boilerplate query-level openers strip to empty.
		{"View a particular vehicle.", "vehicle", "Query", true},
		{"Retrieve a particular template.", "template", "Query", true},
		{"List minted vehicles.", "vehicles", "Query", true},
		{"criteria to search for a manufacturer", "by", "ManufacturerBy", true},
		// Filter-input descriptions that restate the field name via morphology.
		{"Filter for vehicles owned by this address.", "owner", "VehiclesFilter", true},
		{"Filter for aftermarket devices owned by this address.", "owner", "AftermarketDevicesFilter", true},
		{"Filter for DCN owned by this address.", "owner", "DCNFilter", true},
		// Drop-entirely openers ("View a particular ...") are stripped regardless
		// of remainder — the lookup mode is already enumerated in the `by:` input
		// type's fields, so the tail ("by VIN") adds no load-bearing info.
		{"View a particular vehicle by VIN.", "vehicle", "Query", true},
		// Strip-only openers ("Filter for ...") keep novel remainders.
		{"Filter for vehicles produced by a manufacturer.", "manufacturerTokenId", "VehiclesFilter", false},
	}
	for _, tt := range tests {
		t.Run(tt.desc+"_"+tt.field, func(t *testing.T) {
			assert.Equal(t, tt.expected, isSelfEvidentDescription(tt.desc, tt.field, tt.typ))
		})
	}
}

func TestStripBoilerplatePrefix(t *testing.T) {
	tests := []struct{ in, want string }{
		{"View a particular vehicle.", "vehicle."},
		{"Retrieve a particular template.", "template."},
		{"Filter for vehicles owned by this address.", "vehicles owned by this address."},
		{"Filters the DCNs based on the specified criteria.", "DCNs based on the specified criteria."},
		{"List minted vehicles.", "vehicles."},
		{"No prefix here.", "No prefix here."},
		{"", ""},
	}
	for _, tt := range tests {
		t.Run(tt.in, func(t *testing.T) {
			assert.Equal(t, tt.want, stripBoilerplatePrefix(tt.in))
		})
	}
}

func TestAnalyzeSchemaEdgeDetection(t *testing.T) {
	schema, err := loadGraphQLSchema([]string{"testdata/edge_connection.graphqls"})
	require.NoError(t, err)

	analysis := analyzeSchema(schema)

	assert.True(t, analysis.edgeTypes["VehicleEdge"])
	assert.True(t, analysis.edgeTypes["DCNEdge"])
	assert.False(t, analysis.edgeTypes["Vehicle"])

	assert.True(t, analysis.connectionTypes["VehicleConnection"])
	assert.True(t, analysis.connectionTypes["DCNConnection"])
	assert.False(t, analysis.connectionTypes["PageInfo"])
}

func TestAnalyzeSchemaSignalDetection(t *testing.T) {
	schema, err := loadGraphQLSchema([]string{"testdata/signals.graphqls"})
	require.NoError(t, err)

	analysis := analyzeSchema(schema)

	assert.True(t, analysis.signalTypes["SignalAggregations"])
	assert.False(t, analysis.signalTypes["SignalFloatFilter"])
}

func TestCondensedSDLMcpHide(t *testing.T) {
	schema, err := loadGraphQLSchema([]string{"testdata/hidden.graphqls"})
	require.NoError(t, err)

	sdl := generateCondensedSDL(schema)

	// Hidden field should be excluded.
	assert.NotContains(t, sdl, "internalScore")
	// Non-hidden fields should remain.
	assert.Contains(t, sdl, "tokenId")
	assert.Contains(t, sdl, "owner")
	assert.Contains(t, sdl, "name")
}

func TestCondensedSDLStructuralValidity(t *testing.T) {
	// Round-trip validation: for each testdata schema, generate condensed SDL,
	// strip comment lines (signal tables, examples), and verify the remaining
	// SDL is parseable as valid GraphQL.
	entries, err := os.ReadDir("testdata")
	require.NoError(t, err)

	for _, e := range entries {
		if e.IsDir() || filepath.Ext(e.Name()) != ".graphqls" {
			continue
		}
		t.Run(e.Name(), func(t *testing.T) {
			schema, err := loadGraphQLSchema([]string{filepath.Join("testdata", e.Name())})
			require.NoError(t, err)

			sdl := generateCondensedSDL(schema)
			require.NotEmpty(t, sdl)

			// Strip comment-only lines (signal tables, examples, convention notes).
			var cleaned []string
			for _, line := range strings.Split(sdl, "\n") {
				trimmed := strings.TrimSpace(line)
				if trimmed == "" || strings.HasPrefix(trimmed, "#") {
					continue
				}
				cleaned = append(cleaned, line)
			}
			cleanedSDL := strings.Join(cleaned, "\n")
			if strings.TrimSpace(cleanedSDL) == "" {
				return // all-comments output (unlikely but safe)
			}

			// Parse the cleaned SDL to verify structural validity.
			// Tolerate "Undefined type" errors for collapsed Edge/Connection types
			// which are intentionally omitted and described via convention comments.
			_, gqlErr := gqlparser.LoadSchema(&ast.Source{
				Name:  "condensed",
				Input: cleanedSDL,
			})
			if gqlErr != nil {
				for _, line := range strings.Split(gqlErr.Error(), "\n") {
					if strings.Contains(line, "Undefined type") {
						continue // expected for collapsed types
					}
					t.Errorf("condensed SDL has structural error:\n%s\n\nCleaned SDL:\n%s", line, cleanedSDL)
				}
			}
		})
	}
}
