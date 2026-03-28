package main

import (
	"regexp"
	"strings"

	"github.com/vektah/gqlparser/v2/ast"
)

// isDeprecated returns true if the directive list includes @deprecated.
func isDeprecated(dirs ast.DirectiveList) bool {
	return dirs.ForName("deprecated") != nil
}

// isHidden returns true if the directive list includes @mcpHide.
func isHidden(dirs ast.DirectiveList) bool {
	return dirs.ForName("mcpHide") != nil
}

// shouldOmitField returns true if the field/value should be excluded from condensed output.
func shouldOmitField(dirs ast.DirectiveList) bool {
	return isDeprecated(dirs) || isHidden(dirs)
}

// selfEvidentFields are field names whose semantics are universally understood
// (comparison operators, standard filter fields) and never need descriptions.
var selfEvidentFields = map[string]bool{
	"eq": true, "neq": true, "gt": true, "lt": true, "gte": true, "lte": true,
	"in": true, "notIn": true, "or": true, "and": true, "not": true,
	"containsAll": true, "containsAny": true,
}

// stopWords are filler words ignored when comparing descriptions against field/type names.
var stopWords = map[string]bool{
	"the": true, "of": true, "for": true, "this": true, "a": true, "an": true,
	"is": true, "in": true, "to": true, "and": true, "or": true, "by": true,
	"on": true, "at": true, "from": true, "with": true, "its": true, "that": true,
	"true": true, "false": true,
}

// isSelfEvidentDescription returns true if a description trivially restates the field name.
// Uses word-overlap: if every non-stop-word in the description also appears in the
// field name or type name (split on camelCase boundaries), the description adds nothing.
func isSelfEvidentDescription(desc, fieldName, typeName string) bool {
	if desc == "" {
		return false
	}
	if selfEvidentFields[fieldName] {
		return true
	}

	// Build the set of "known" words from field name + type name.
	// Include both the camelCase-split words and the raw lowercase forms,
	// since descriptions may use either "dataVersion" or "data version".
	knownWords := make(map[string]bool)
	knownWords[strings.ToLower(fieldName)] = true
	knownWords[strings.ToLower(typeName)] = true
	for _, w := range strings.Fields(camelToSpaced(fieldName)) {
		knownWords[w] = true
	}
	for _, w := range strings.Fields(camelToSpaced(typeName)) {
		knownWords[w] = true
	}

	// Tokenize description, strip punctuation, check if all meaningful words are known.
	for _, w := range strings.Fields(strings.ToLower(desc)) {
		w = strings.Trim(w, ".,;:!?'\"()-")
		if w == "" || len(w) <= 1 || stopWords[w] {
			continue
		}
		if !knownWords[w] {
			return false
		}
	}
	return true
}

// camelToSpaced converts PascalCase/camelCase to lowercase space-separated words.
// e.g., "DeviceDefinition" → "device definition"
func camelToSpaced(s string) string {
	var sb strings.Builder
	for i, r := range s {
		if i > 0 && r >= 'A' && r <= 'Z' {
			sb.WriteByte(' ')
		}
		sb.WriteRune(r)
	}
	return strings.ToLower(sb.String())
}

// paginationArgs are standard Relay pagination argument names whose descriptions
// are well-known and add no value for LLMs.
var paginationArgs = map[string]bool{
	"first": true, "after": true, "last": true, "before": true,
}

// isPaginationArg returns true if the argument is a standard Relay pagination argument.
func isPaginationArg(name string) bool {
	return paginationArgs[name]
}

// didFormatPattern matches the common DID format description that appears on many tokenDID fields.
var didFormatPattern = regexp.MustCompile(`(?i)did:erc721:<chainID>:<contractAddress>:<tokenId>`)

// containsDIDFormat returns true if a description contains the DID format string.
func containsDIDFormat(desc string) bool {
	return didFormatPattern.MatchString(desc)
}

// filterFieldDescription returns the description to use for a field, or "" if it should be omitted.
// It strips self-evident descriptions and DID format descriptions.
func filterFieldDescription(desc, fieldName, typeName string) string {
	if desc == "" {
		return ""
	}
	if isSelfEvidentDescription(desc, fieldName, typeName) {
		return ""
	}
	if containsDIDFormat(desc) {
		return ""
	}
	return desc
}

// filterArgDescription returns the description to use for an argument, or "" if it should be omitted.
// Pagination arguments and self-evident descriptions are stripped.
func filterArgDescription(desc, argName string) string {
	if desc == "" || isPaginationArg(argName) {
		return ""
	}

	knownWords := make(map[string]bool)
	knownWords[strings.ToLower(argName)] = true
	for _, w := range strings.Fields(camelToSpaced(argName)) {
		knownWords[w] = true
	}

	for _, w := range strings.Fields(strings.ToLower(desc)) {
		w = strings.Trim(w, ".,;:!?'\"()-")
		if w == "" || len(w) <= 1 || stopWords[w] {
			continue
		}
		if !knownWords[w] {
			return desc
		}
	}
	return ""
}
