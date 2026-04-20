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
// Also includes type-like nouns ("address", "integer") whose presence is already conveyed by
// the field's GraphQL type and adds no signal in a docstring.
var stopWords = map[string]bool{
	"the": true, "of": true, "for": true, "this": true, "a": true, "an": true,
	"is": true, "in": true, "to": true, "and": true, "or": true, "by": true,
	"on": true, "at": true, "from": true, "with": true, "its": true, "that": true,
	"true": true, "false": true, "any": true, "which": true,
	"particular": true, "specific": true, "specified": true, "given": true,
	"based": true, "criteria": true, "matching": true, "these": true,
	"returns": true, "return": true, "contains": true,
	"address": true, "integer": true, "string": true, "boolean": true, "bytes": true,
}

// wordStems maps morphological variants to a canonical stem so descriptions like
// "Filter for vehicles owned by this address" match field names like "owner".
// Both sides of each entry reduce to the key. Intentionally small — we only add
// pairs observed producing false-negative self-evident matches across DIMO schemas.
var wordStems = map[string]string{
	"owned": "owner", "owns": "owner", "owning": "owner",
	"minted": "mint", "minting": "mint", "mints": "mint",
	"paired": "pair", "pairing": "pair", "pairs": "pair",
	"created": "create", "creating": "create", "creates": "create", "creator": "create",
	"filtered": "filter", "filtering": "filter", "filters": "filter",
	"connected": "connect", "connecting": "connect", "connects": "connect", "connection": "connect",
	"vehicles": "vehicle", "devices": "device",
	"addresses": "address",
}

// stemOf returns the canonical stem for a word, or the word itself if no stem is known.
func stemOf(w string) string {
	if s, ok := wordStems[w]; ok {
		return s
	}
	return w
}

// dropEntirelyPrefixes are verb+article openers where the remainder of the
// description always just names or explains the entity the field returns.
// Descriptions starting with any of these are treated as fully self-evident,
// regardless of remainder content (sort-order trivia, alias notes, etc.).
var dropEntirelyPrefixes = []string{
	"view a particular ", "retrieve a particular ", "get a particular ",
	"fetch a particular ", "look up a particular ", "look up ",
	"retrieves information about an ", "retrieves information about a ",
	"retrieves information about ",
	"list minted ", "lists minted ", "list ", "lists ",
	"view ", "retrieve ", "retrieves ", "get ", "fetch ", "fetches ",
}

// stripOnlyPrefixes are openers stripped off so the remainder is checked
// against field/type names. Used for qualifiers like "Filter for X ..." where
// the tail carries the load-bearing content.
var stripOnlyPrefixes = []string{
	"criteria to search for a ", "criteria to search for an ", "criteria to search for ",
	"filters the ", "filter the ", "filters for ", "filter for ",
	"filters by ", "filter by ", "filter on ", "filters on ",
	"filter based on ", "filters based on ",
}

// hasDropEntirelyPrefix reports whether desc opens with a verb+article prefix
// that renders the whole description self-evident.
func hasDropEntirelyPrefix(desc string) bool {
	lower := strings.ToLower(strings.TrimSpace(desc))
	for _, p := range dropEntirelyPrefixes {
		if strings.HasPrefix(lower, p) {
			return true
		}
	}
	return false
}

// stripBoilerplatePrefix removes a known opener from desc. Returns the trimmed
// remainder (empty when the prefix consumed the entire description). Handles
// both drop-entirely and strip-only prefixes; callers pair this with
// hasDropEntirelyPrefix when they want to bail out early.
func stripBoilerplatePrefix(desc string) string {
	lower := strings.ToLower(strings.TrimSpace(desc))
	for _, p := range dropEntirelyPrefixes {
		if strings.HasPrefix(lower, p) {
			return strings.TrimSpace(desc[len(p):])
		}
	}
	for _, p := range stripOnlyPrefixes {
		if strings.HasPrefix(lower, p) {
			return strings.TrimSpace(desc[len(p):])
		}
	}
	return desc
}

// isSelfEvidentDescription returns true if a description trivially restates the field name.
// Uses word-overlap: if every non-stop-word in the description also appears in the
// field name or type name (split on camelCase boundaries), the description adds nothing.
// Leading boilerplate openers (e.g. "View a particular", "Filter for") are stripped first
// so descriptions that would otherwise register as novel resolve to empty.
func isSelfEvidentDescription(desc, fieldName, typeName string) bool {
	if desc == "" {
		return false
	}
	if selfEvidentFields[fieldName] {
		return true
	}

	if hasDropEntirelyPrefix(desc) {
		return true
	}
	desc = stripBoilerplatePrefix(desc)
	if desc == "" {
		return true
	}

	// Build the set of "known" words from field name + type name.
	// Include both the camelCase-split words and the raw lowercase forms,
	// since descriptions may use either "dataVersion" or "data version".
	knownWords := make(map[string]bool)
	addKnown := func(w string) {
		knownWords[w] = true
		knownWords[stemOf(w)] = true
	}
	addKnown(strings.ToLower(fieldName))
	addKnown(strings.ToLower(typeName))
	for _, w := range strings.Fields(camelToSpaced(fieldName)) {
		addKnown(w)
	}
	for _, w := range strings.Fields(camelToSpaced(typeName)) {
		addKnown(w)
	}

	// Tokenize description, strip punctuation, check if all meaningful words are known.
	for _, w := range strings.Fields(strings.ToLower(desc)) {
		w = strings.Trim(w, ".,;:!?'\"()-")
		if w == "" || len(w) <= 1 || stopWords[w] {
			continue
		}
		if knownWords[w] || knownWords[stemOf(w)] {
			continue
		}
		return false
	}
	return true
}

// camelToSpaced converts PascalCase/camelCase to lowercase space-separated words,
// treating runs of uppercase letters as a single acronym.
// e.g., "DeviceDefinition" → "device definition", "DCNFilter" → "dcn filter",
// "tokenDID" → "token did".
func camelToSpaced(s string) string {
	runes := []rune(s)
	var sb strings.Builder
	isUpper := func(r rune) bool { return r >= 'A' && r <= 'Z' }
	isLower := func(r rune) bool { return r >= 'a' && r <= 'z' }
	for i, r := range runes {
		if i > 0 && isUpper(r) {
			prev := runes[i-1]
			// camelCase boundary: lower → upper (fooBar → foo Bar).
			// Acronym boundary: upper → upper where next is lower (DCNFilter → DCN Filter).
			if isLower(prev) || (isUpper(prev) && i+1 < len(runes) && isLower(runes[i+1])) {
				sb.WriteByte(' ')
			}
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

// filterScalarDescription returns the inline description to emit after a scalar
// declaration, or "" if none is worth showing. Self-evident descriptions (that
// merely restate the scalar name) are dropped. Multi-sentence descriptions are
// truncated to the first sentence to strip analogies and spec footnotes.
func filterScalarDescription(desc, scalarName string) string {
	if desc == "" {
		return ""
	}
	collapsed := strings.Join(strings.Fields(desc), " ")
	if first := firstSentence(collapsed); first != "" {
		collapsed = first
	}
	if isSelfEvidentDescription(collapsed, "", scalarName) {
		return ""
	}
	return collapsed
}

// firstSentence returns the first sentence of s, including its terminating period,
// or the whole string if no sentence terminator is found.
func firstSentence(s string) string {
	if idx := strings.Index(s, ". "); idx >= 0 {
		return s[:idx+1]
	}
	return s
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
// parentField is the name of the field the argument belongs to (e.g. "vehicle" for
// vehicle(tokenId:)); its tokens join knownWords so "The token ID of the vehicle"
// resolves as self-evident.
func filterArgDescription(desc, argName, parentField string) string {
	if desc == "" || isPaginationArg(argName) {
		return ""
	}

	desc = stripBoilerplatePrefix(desc)
	if desc == "" {
		return ""
	}

	knownWords := make(map[string]bool)
	addKnown := func(w string) {
		knownWords[w] = true
		knownWords[stemOf(w)] = true
	}
	addKnown(strings.ToLower(argName))
	for _, w := range strings.Fields(camelToSpaced(argName)) {
		addKnown(w)
	}
	if parentField != "" {
		addKnown(strings.ToLower(parentField))
		for _, w := range strings.Fields(camelToSpaced(parentField)) {
			addKnown(w)
		}
	}

	for _, w := range strings.Fields(strings.ToLower(desc)) {
		w = strings.Trim(w, ".,;:!?'\"()-")
		if w == "" || len(w) <= 1 || stopWords[w] {
			continue
		}
		if knownWords[w] || knownWords[stemOf(w)] {
			continue
		}
		return desc
	}
	return ""
}
