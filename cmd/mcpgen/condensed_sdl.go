package main

import (
	"strings"

	"github.com/vektah/gqlparser/v2/ast"
)

// writeOperationType writes a Query, Mutation, or Subscription type in SDL format,
// including inline @mcpExample comments after annotated fields.
// Deprecated fields are omitted.
func writeOperationType(sb *strings.Builder, def *ast.Definition) {
	sb.WriteString("type ")
	sb.WriteString(def.Name)
	sb.WriteString(" {\n")

	prevHadExtra := false
	for _, field := range def.Fields {
		if strings.HasPrefix(field.Name, "__") || shouldOmitField(field.Directives) {
			continue
		}

		desc := filterFieldDescription(field.Description, field.Name, def.Name)
		hasExtra := desc != ""

		if prevHadExtra {
			sb.WriteString("\n")
		}

		if desc != "" {
			writeDescription(sb, desc, "  ")
		}

		sb.WriteString("  ")
		sb.WriteString(field.Name)
		writeFieldArguments(sb, field.Arguments)
		sb.WriteString(": ")
		sb.WriteString(field.Type.String())
		sb.WriteString("\n")

		for _, dir := range field.Directives {
			if dir.Name != "mcpExample" {
				continue
			}
			descArg := dir.Arguments.ForName("description")
			queryArg := dir.Arguments.ForName("query")
			if descArg != nil && queryArg != nil {
				sb.WriteString("  # Example - ")
				sb.WriteString(descArg.Value.Raw)
				sb.WriteString(":\n")
				sb.WriteString("  #   ")
				sb.WriteString(queryArg.Value.Raw)
				sb.WriteString("\n")
				hasExtra = true
			}
		}

		prevHadExtra = hasExtra
	}

	sb.WriteString("}\n")
}

// writeTypeDefinition writes a non-operation type in SDL format.
// It skips deprecated fields, preserves @oneOf on input types, adds scalar descriptions,
// and collapses @isSignal fields into compact tables.
// Response-only types and types without useful descriptions use compact single-line format.
func writeTypeDefinition(sb *strings.Builder, def *ast.Definition, analysis *schemaAnalysis) {
	switch def.Kind {
	case ast.Object, ast.InputObject:
		isResponseOnly := def.Kind == ast.Object && !analysis.queryReachable[def.Name] && !analysis.signalTypes[def.Name]

		// Signal types always get the categorized table treatment.
		// Response-only types and types without useful descriptions use single-line format.
		if !analysis.signalTypes[def.Name] && (isResponseOnly || !hasUsefulFieldDescriptions(def)) {
			writeCompactInputOrType(sb, def)
			return
		}

		writeTypeHeader(sb, def)
		sb.WriteString(" {\n")

		if analysis.signalTypes[def.Name] {
			writeFieldsWithSignalCollapsing(sb, def)
		} else {
			for _, field := range def.Fields {
				if shouldOmitField(field.Directives) {
					continue
				}
				desc := filterFieldDescription(field.Description, field.Name, def.Name)
				if desc != "" {
					writeDescription(sb, desc, "  ")
				}
				sb.WriteString("  ")
				sb.WriteString(field.Name)
				writeFieldArguments(sb, field.Arguments)
				sb.WriteString(": ")
				sb.WriteString(field.Type.String())
				writeFieldDefaultValue(sb, field)
				sb.WriteString("\n")
			}
		}
		sb.WriteString("}\n")

	case ast.Enum:
		// Auto-compact enums with no useful value descriptions.
		if !hasUsefulEnumDescriptions(def) {
			writeCompactEnum(sb, def)
			return
		}
		sb.WriteString("enum ")
		sb.WriteString(def.Name)
		sb.WriteString(" {\n")
		for _, val := range def.EnumValues {
			if shouldOmitField(val.Directives) {
				continue
			}
			if val.Description != "" {
				writeDescription(sb, val.Description, "  ")
			}
			sb.WriteString("  ")
			sb.WriteString(val.Name)
			sb.WriteString("\n")
		}
		sb.WriteString("}\n")

	case ast.Interface:
		// Use compact format for interfaces.
		sb.WriteString("interface ")
		sb.WriteString(def.Name)
		sb.WriteString(" { ")
		first := true
		for _, field := range def.Fields {
			if shouldOmitField(field.Directives) {
				continue
			}
			if !first {
				sb.WriteString(", ")
			}
			sb.WriteString(field.Name)
			sb.WriteString(": ")
			sb.WriteString(field.Type.String())
			first = false
		}
		sb.WriteString(" }\n")

	case ast.Union:
		sb.WriteString("union ")
		sb.WriteString(def.Name)
		sb.WriteString(" = ")
		for i, t := range def.Types {
			if i > 0 {
				sb.WriteString(" | ")
			}
			sb.WriteString(t)
		}
		sb.WriteString("\n")

	case ast.Scalar:
		sb.WriteString("scalar ")
		sb.WriteString(def.Name)
		if def.Description != "" {
			sb.WriteString("  # ")
			// Join multi-line descriptions into a single inline comment.
			desc := strings.Join(strings.Fields(def.Description), " ")
			sb.WriteString(desc)
		}
		sb.WriteString("\n")
	}
}

// hasUsefulFieldDescriptions returns true if any non-deprecated field on the type has a
// description that survives filtering (not self-evident, not DID format).
func hasUsefulFieldDescriptions(def *ast.Definition) bool {
	for _, field := range def.Fields {
		if shouldOmitField(field.Directives) {
			continue
		}
		if filterFieldDescription(field.Description, field.Name, def.Name) != "" {
			return true
		}
	}
	return false
}

// hasUsefulEnumDescriptions returns true if any enum value has a non-empty description.
func hasUsefulEnumDescriptions(def *ast.Definition) bool {
	for _, val := range def.EnumValues {
		if shouldOmitField(val.Directives) {
			continue
		}
		if val.Description != "" {
			return true
		}
	}
	return false
}

// writeTypeHeader writes the shared prefix for input/type definitions:
// keyword, name, @oneOf (if applicable), and implements clause.
func writeTypeHeader(sb *strings.Builder, def *ast.Definition) {
	if def.Kind == ast.InputObject {
		sb.WriteString("input ")
	} else {
		sb.WriteString("type ")
	}
	sb.WriteString(def.Name)
	if def.Kind == ast.InputObject && def.Directives.ForName("oneOf") != nil {
		sb.WriteString(" @oneOf")
	}
	if len(def.Interfaces) > 0 {
		sb.WriteString(" implements ")
		for i, iface := range def.Interfaces {
			if i > 0 {
				sb.WriteString(" & ")
			}
			sb.WriteString(iface)
		}
	}
}

// writeCompactInputOrType emits an input or type with no useful descriptions as a single line.
func writeCompactInputOrType(sb *strings.Builder, def *ast.Definition) {
	writeTypeHeader(sb, def)
	sb.WriteString(" { ")
	first := true
	for _, field := range def.Fields {
		if shouldOmitField(field.Directives) {
			continue
		}
		if !first {
			sb.WriteString(", ")
		}
		sb.WriteString(field.Name)
		writeInlineArgs(sb, field.Arguments)
		sb.WriteString(": ")
		sb.WriteString(field.Type.String())
		writeFieldDefaultValue(sb, field)
		first = false
	}
	sb.WriteString(" }\n")
}

// writeCompactEnum emits an enum on a single line: enum Foo { A, B, C }
func writeCompactEnum(sb *strings.Builder, def *ast.Definition) {
	sb.WriteString("enum ")
	sb.WriteString(def.Name)
	sb.WriteString(" { ")
	first := true
	for _, val := range def.EnumValues {
		if shouldOmitField(val.Directives) {
			continue
		}
		if !first {
			sb.WriteString(", ")
		}
		sb.WriteString(val.Name)
		first = false
	}
	sb.WriteString(" }\n")
}

// writeInlineArgs writes arguments in compact inline format: (name: Type, name2: Type2)
// with no descriptions. Used in compact mode and for compact type fields.
func writeInlineArgs(sb *strings.Builder, args ast.ArgumentDefinitionList) {
	if len(args) == 0 {
		return
	}
	sb.WriteString("(")
	for i, arg := range args {
		if i > 0 {
			sb.WriteString(", ")
		}
		sb.WriteString(arg.Name)
		sb.WriteString(": ")
		sb.WriteString(arg.Type.String())
		writeDefaultValue(sb, arg)
	}
	sb.WriteString(")")
}

// writeFieldArguments writes field arguments in SDL format.
// Uses inline format when no arguments have non-trivial descriptions, multi-line otherwise.
// Pagination args and self-evident arg descriptions are stripped.
// Preserves default values when present.
func writeFieldArguments(sb *strings.Builder, args ast.ArgumentDefinitionList) {
	if len(args) == 0 {
		return
	}

	// Check if any arg has a meaningful description after filtering.
	hasDescriptions := false
	for _, arg := range args {
		if filterArgDescription(arg.Description, arg.Name) != "" {
			hasDescriptions = true
			break
		}
	}

	if hasDescriptions {
		sb.WriteString("(\n")
		for _, arg := range args {
			desc := filterArgDescription(arg.Description, arg.Name)
			if desc != "" {
				writeDescription(sb, desc, "    ")
			}
			sb.WriteString("    ")
			sb.WriteString(arg.Name)
			sb.WriteString(": ")
			sb.WriteString(arg.Type.String())
			writeDefaultValue(sb, arg)
			sb.WriteString("\n")
		}
		sb.WriteString("  )")
	} else {
		sb.WriteString("(")
		for i, arg := range args {
			if i > 0 {
				sb.WriteString(", ")
			}
			sb.WriteString(arg.Name)
			sb.WriteString(": ")
			sb.WriteString(arg.Type.String())
			writeDefaultValue(sb, arg)
		}
		sb.WriteString(")")
	}
}

// writeRawDefault appends " = <value>" to the builder for a default value.
func writeRawDefault(sb *strings.Builder, val *ast.Value) {
	if val == nil {
		return
	}
	sb.WriteString(" = ")
	if val.Kind == ast.StringValue {
		sb.WriteString("\"")
		sb.WriteString(val.Raw)
		sb.WriteString("\"")
	} else {
		sb.WriteString(val.Raw)
	}
}

// writeDefaultValue appends " = <value>" to the builder if the argument has a default value.
func writeDefaultValue(sb *strings.Builder, arg *ast.ArgumentDefinition) {
	writeRawDefault(sb, arg.DefaultValue)
}

// writeFieldDefaultValue appends " = <value>" for input object fields with default values.
func writeFieldDefaultValue(sb *strings.Builder, field *ast.FieldDefinition) {
	writeRawDefault(sb, field.DefaultValue)
}

// writeDescription writes a GraphQL description string with the given indentation.
// Short descriptions use inline "..." syntax. Long descriptions (>200 chars) or those
// containing quotes use block string syntax with word-wrapping for readability.
// Structural elements (list items starting with "- ", blank-line paragraph breaks)
// are preserved so LLMs can parse constraints and options.
func writeDescription(sb *strings.Builder, desc, indent string) {
	// Collapse multi-line descriptions to single line for compactness.
	collapsed := strings.Join(strings.Fields(desc), " ")

	needsBlock := strings.Contains(collapsed, "\"") || len(collapsed) > 200
	if !needsBlock {
		sb.WriteString(indent)
		sb.WriteString("\"")
		sb.WriteString(collapsed)
		sb.WriteString("\"\n")
		return
	}

	// Block string with structure-aware wrapping.
	paragraphs := splitDescriptionParagraphs(desc)
	sb.WriteString(indent)
	sb.WriteString("\"\"\"\n")
	for _, para := range paragraphs {
		if para == "" {
			// Blank line between paragraphs.
			sb.WriteString("\n")
			continue
		}
		for _, line := range wordWrap(para, 80) {
			sb.WriteString(indent)
			sb.WriteString(line)
			sb.WriteString("\n")
		}
	}
	sb.WriteString(indent)
	sb.WriteString("\"\"\"\n")
}

// splitDescriptionParagraphs splits a description into logical paragraphs.
// It treats blank lines as paragraph separators and lines starting with "- "
// as individual list items (each becomes its own paragraph prefixed with "- ").
func splitDescriptionParagraphs(desc string) []string {
	rawLines := strings.Split(desc, "\n")
	var paragraphs []string
	var current []string

	flush := func() {
		if len(current) > 0 {
			paragraphs = append(paragraphs, strings.Join(strings.Fields(strings.Join(current, " ")), " "))
			current = nil
		}
	}

	for _, line := range rawLines {
		trimmed := strings.TrimSpace(line)
		if trimmed == "" {
			flush()
			continue
		}
		if strings.HasPrefix(trimmed, "- ") {
			flush()
			// Keep the list item as its own paragraph with the "- " prefix.
			paragraphs = append(paragraphs, "- "+strings.Join(strings.Fields(trimmed[2:]), " "))
			continue
		}
		current = append(current, trimmed)
	}
	flush()
	return paragraphs
}

// wordWrap splits text into lines of at most maxWidth characters, breaking at word boundaries.
func wordWrap(text string, maxWidth int) []string {
	words := strings.Fields(text)
	if len(words) == 0 {
		return nil
	}
	var lines []string
	current := words[0]
	for _, w := range words[1:] {
		if len(current)+1+len(w) > maxWidth {
			lines = append(lines, current)
			current = w
		} else {
			current += " " + w
		}
	}
	lines = append(lines, current)
	return lines
}
