package main

import (
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestParseDirective(t *testing.T) {
	tools, err := parseSchema([]string{"testdata/basic.graphqls"}, "")
	require.NoError(t, err)
	require.Len(t, tools, 2)

	// hello tool
	assert.Equal(t, "hello", tools[0].Name)
	assert.Equal(t, "Say hello to someone", tools[0].Description)
	require.Len(t, tools[0].Args, 1)
	assert.Equal(t, "name", tools[0].Args[0].Name)
	assert.Equal(t, "string", tools[0].Args[0].Type)
	assert.True(t, tools[0].Args[0].Required)

	// vehicle_info tool
	assert.Equal(t, "vehicle_info", tools[1].Name)
	assert.Equal(t, "Get vehicle info", tools[1].Description)
	require.Len(t, tools[1].Args, 1)
	assert.Equal(t, "tokenId", tools[1].Args[0].Name)
	assert.Equal(t, "integer", tools[1].Args[0].Type)
	assert.True(t, tools[1].Args[0].Required)
}

func TestPrefixApplied(t *testing.T) {
	tools, err := parseSchema([]string{"testdata/basic.graphqls"}, "telemetry")
	require.NoError(t, err)
	require.Len(t, tools, 2)

	assert.Equal(t, "telemetry_hello", tools[0].Name)
	assert.Equal(t, "telemetry_vehicle_info", tools[1].Name)
}

func TestArgDescriptionFromDocString(t *testing.T) {
	tools, err := parseSchema([]string{"testdata/docstrings.graphqls"}, "")
	require.NoError(t, err)
	require.Len(t, tools, 1)
	require.Len(t, tools[0].Args, 1)

	assert.Equal(t, "The vehicle's NFT token ID", tools[0].Args[0].Description)
}

func TestArgDescriptionFallback(t *testing.T) {
	tools, err := parseSchema([]string{"testdata/basic.graphqls"}, "")
	require.NoError(t, err)
	require.Len(t, tools, 2)

	// hello tool's "name" arg has no doc string
	assert.Equal(t, "name argument", tools[0].Args[0].Description)
}

func TestInvalidSelection(t *testing.T) {
	_, err := parseSchema([]string{"testdata/invalid_selection.graphqls"}, "")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "nonExistentField")
}

func TestGeneratedOutput(t *testing.T) {
	tools, err := parseSchema([]string{"testdata/basic.graphqls"}, "test")
	require.NoError(t, err)

	output, err := generateGoFile("graph", tools)
	require.NoError(t, err)

	assert.True(t, strings.Contains(output, "package graph"))
	assert.True(t, strings.Contains(output, `"github.com/DIMO-Network/server-garage/pkg/mcpserver"`))
	assert.True(t, strings.Contains(output, "DO NOT EDIT"))
	assert.True(t, strings.Contains(output, "test_hello"))
	assert.True(t, strings.Contains(output, "test_vehicle_info"))
}
