package main

import (
	"fmt"
	"regexp"
	"sort"
	"strings"

	"github.com/vektah/gqlparser/v2/ast"
)

var unitRegexp = regexp.MustCompile(`(?i)unit:\s*'([^']+)'`)

var privilegeRegexp = regexp.MustCompile(`(?i)required privileges:\s*\[([^\]]+)\]`)

// signalGroup groups signal fields by their return type and argument signature.
type signalGroup struct {
	returnType string
	argSig     string // e.g. "(agg: FloatAggregation!, filter: SignalFloatFilter)"
	fields     []*ast.FieldDefinition
}

// typeArgInfo associates a signal type name with its grouped signal fields.
type typeArgInfo struct {
	typeName string
	groups   []signalGroup
}

// signalCategoryInfo holds a group of signals under a category label.
type signalCategoryInfo struct {
	label  string
	prefix string
	fields []*ast.FieldDefinition
}

// writeSignalReferenceTable emits a standalone signal reference section before the signal types.
// This is shared by all signal types (e.g., SignalAggregations and SignalCollection).
func writeSignalReferenceTable(sb *strings.Builder, schema *ast.Schema, analysis *schemaAnalysis) {
	// Use the first signal type (alphabetically) as the source for signal fields.
	var signalTypeNames []string
	for name := range analysis.signalTypes {
		signalTypeNames = append(signalTypeNames, name)
	}
	sort.Strings(signalTypeNames)
	if len(signalTypeNames) == 0 {
		return
	}

	// Collect signals and arg signatures from ALL signal types.
	var signals []*ast.FieldDefinition
	var typeInfos []typeArgInfo

	for _, typeName := range signalTypeNames {
		def := schema.Types[typeName]
		var typeSignals []*ast.FieldDefinition
		for _, f := range def.Fields {
			if shouldOmitField(f.Directives) || f.Directives.ForName("isSignal") == nil {
				continue
			}
			typeSignals = append(typeSignals, f)
		}
		if len(signals) == 0 {
			signals = typeSignals // use first type's signals for the reference table
		}
		typeInfos = append(typeInfos, typeArgInfo{typeName: typeName, groups: groupSignalFields(typeSignals)})
	}

	if len(signals) == 0 {
		return
	}

	sb.WriteString("\n")
	fmt.Fprintf(sb, "# ═══ SIGNAL FIELDS (%d total) ═══\n", len(signals))

	// Emit per-type calling conventions so the LLM knows how to query each
	// signal type. The args depend on the parent type, not the individual signal.
	sb.WriteString("# All signals below exist on every signal type. Calling convention per type:\n")
	for _, ti := range typeInfos {
		if len(ti.groups) == 1 {
			g := ti.groups[0]
			fmt.Fprintf(sb, "#   %s: fieldName%s: %s\n", ti.typeName, g.argSig, g.returnType)
			continue
		}
		fmt.Fprintf(sb, "#   %s:\n", ti.typeName)
		for _, g := range ti.groups {
			fmt.Fprintf(sb, "#     fieldName%s: %s\n", g.argSig, g.returnType)
		}
	}

	// Emit type-exception list so the table can drop its Type column.
	// Default is Float (dominant case in VSS); call out String / Location / other.
	buckets := map[string][]string{}
	bucketOrder := []string{}
	for _, f := range signals {
		rt := baseSignalType(f.Type)
		if _, ok := buckets[rt]; !ok {
			bucketOrder = append(bucketOrder, rt)
		}
		buckets[rt] = append(buckets[rt], f.Name)
	}
	defaultType := "Float"
	if _, ok := buckets[defaultType]; !ok {
		// Fall back to whichever bucket is largest.
		maxN := 0
		for t, names := range buckets {
			if len(names) > maxN {
				defaultType = t
				maxN = len(names)
			}
		}
	}
	fmt.Fprintf(sb, "# %s is the default type.", defaultType)
	for _, t := range bucketOrder {
		if t == defaultType {
			continue
		}
		fmt.Fprintf(sb, " %s: %s.", t, strings.Join(buckets[t], ", "))
	}
	sb.WriteString("\n")

	// Emit markdown table (Unit | Description; Type is omitted — see exception list above).
	sb.WriteString("# | Signal | Unit | Description |\n")
	sb.WriteString("# |--------|------|-------------|\n")

	// Global shared descriptions: descriptions repeated verbatim ≥3 times across
	// the entire signal list (doors "Is item open or closed?", belts "Is the belt
	// engaged", wheel "Rotational speed of a vehicle's wheel", etc.). Emit each
	// once in a legend above the table; rows whose desc matches are blanked so
	// the prose isn't repeated across every door/belt/wheel row.
	globalShared := sharedDescriptions(signals, 3)
	if len(globalShared) > 0 {
		sb.WriteString("# Shared descriptions (blank rows below use these):\n")
		for _, d := range globalShared {
			fmt.Fprintf(sb, "#   - %s\n", d)
		}
	}
	globalSharedSet := make(map[string]bool, len(globalShared))
	for _, d := range globalShared {
		globalSharedSet[d] = true
	}

	categories := categorizeSignals(signals)
	for _, cat := range categories {
		priv := dominantPrivilege(cat.fields)
		// Category-local shared descriptions (≥2 reps not already in the global
		// set) — catches smaller clusters the global threshold misses.
		localShared := sharedDescriptions(cat.fields, 2)
		localSharedSet := make(map[string]bool, len(localShared))
		var localUnique []string
		for _, d := range localShared {
			if globalSharedSet[d] {
				continue
			}
			localSharedSet[d] = true
			localUnique = append(localUnique, d)
		}
		header := fmt.Sprintf("# ── %s ──", cat.label)
		if priv != "" {
			header = fmt.Sprintf("# ── %s (privilege: %s) ──", cat.label, priv)
		}
		sb.WriteString(header)
		sb.WriteString("\n")
		for _, d := range localUnique {
			fmt.Fprintf(sb, "# shared: %s\n", d)
		}
		for _, f := range cat.fields {
			writeSignalTableRow(sb, f, priv, globalSharedSet, localSharedSet)
		}
	}
}

// sharedDescriptions returns the set of non-obvious signal descriptions that
// repeat verbatim at least `min` times across fields, preserving first-observed
// order for stable output.
func sharedDescriptions(fields []*ast.FieldDefinition, min int) []string {
	counts := map[string]int{}
	order := []string{}
	for _, f := range fields {
		d := extractShortDescription(f.Description)
		if d == "" || !isNonObviousSignalDesc(d, f.Name) {
			continue
		}
		if _, seen := counts[d]; !seen {
			order = append(order, d)
		}
		counts[d]++
	}
	var out []string
	for _, d := range order {
		if counts[d] >= min {
			out = append(out, d)
		}
	}
	return out
}

// writeFieldsWithSignalCollapsing writes fields for types that contain @isSignal fields.
// Non-signal fields are emitted normally. Signal fields reference the standalone table.
func writeFieldsWithSignalCollapsing(sb *strings.Builder, def *ast.Definition) {
	var nonSignal []*ast.FieldDefinition
	signalCount := 0

	for _, f := range def.Fields {
		if shouldOmitField(f.Directives) {
			continue
		}
		if f.Directives.ForName("isSignal") != nil {
			signalCount++
		} else {
			nonSignal = append(nonSignal, f)
		}
	}

	// Emit non-signal fields normally.
	for _, field := range nonSignal {
		desc := filterFieldDescription(field.Description, field.Name, def.Name)
		if desc != "" {
			writeDescription(sb, desc, "  ")
		}
		sb.WriteString("  ")
		sb.WriteString(field.Name)
		writeFieldArguments(sb, field.Arguments, field.Name)
		sb.WriteString(": ")
		sb.WriteString(field.Type.String())
		sb.WriteString("\n")
	}

	if signalCount > 0 {
		fmt.Fprintf(sb, "  # + %d signal fields (see SIGNAL FIELDS table above)\n", signalCount)
	}
}

// writeSignalTableRow writes a signal as a markdown table row.
// categoryPrivilege is the dominant privilege for the category; if the field's
// privilege differs, it is shown inline in the description column.
// globalShared and localShared are sets of description texts emitted as legends
// above the table or category; rows whose own description matches any of them
// are blanked so the table doesn't repeat identical prose.
func writeSignalTableRow(sb *strings.Builder, f *ast.FieldDefinition, categoryPrivilege string, globalShared, localShared map[string]bool) {
	unit := extractUnit(f.Description)
	shortDesc := extractShortDescription(f.Description)
	// Drop descriptions that just restate the signal name + unit.
	if !isNonObviousSignalDesc(shortDesc, f.Name) {
		shortDesc = ""
	}
	if globalShared[shortDesc] || localShared[shortDesc] {
		shortDesc = ""
	}

	// Show privilege inline if it differs from the category's dominant privilege.
	fieldPriv := extractPrivilege(f.Description)
	if fieldPriv != "" && fieldPriv != categoryPrivilege {
		if shortDesc != "" {
			shortDesc += " (privilege: " + fieldPriv + ")"
		} else {
			shortDesc = "privilege: " + fieldPriv
		}
	}

	fmt.Fprintf(sb, "# | %s | %s | %s |\n", f.Name, unit, shortDesc)
}

// baseSignalType returns the base type name for a signal field's return type.
// For wrapper types like SignalFloat, SignalString, SignalLocation it strips the
// "Signal" prefix. For plain types (Float, String, Location) it returns as-is.
func baseSignalType(t *ast.Type) string {
	name := t.NamedType
	if t.Elem != nil {
		name = t.Elem.NamedType
	}
	if after, ok := strings.CutPrefix(name, "Signal"); ok && after != "" {
		return after
	}
	return name
}

// isNonObviousSignalDesc returns true if a signal description adds value beyond the name+unit.
// Compares description words against the camelCase-split field name. If fewer than 2 words
// in the description are novel (not in the field name and not stop words), it's redundant.
func isNonObviousSignalDesc(desc, fieldName string) bool {
	if desc == "" {
		return false
	}
	nameWords := strings.Fields(strings.ToLower(camelToSpaced(fieldName)))
	nameSet := make(map[string]bool, len(nameWords))
	for _, w := range nameWords {
		nameSet[w] = true
	}
	descWords := strings.Fields(strings.ToLower(desc))
	novelWords := 0
	for _, w := range descWords {
		w = strings.Trim(w, ".,;:!?'\"()-")
		if w == "" || stopWords[w] {
			continue
		}
		if !nameSet[w] {
			novelWords++
		}
	}
	return novelWords >= 2
}

// categorizeSignals groups signal fields by their first camelCase word prefix.
// Prefixes with fewer than 2 signals are grouped into "other".
func categorizeSignals(signals []*ast.FieldDefinition) []signalCategoryInfo {
	// Count signals per depth-1 prefix.
	prefixCount := map[string]int{}
	for _, f := range signals {
		if p := firstCamelPrefix(f.Name); p != "" {
			prefixCount[p]++
		}
	}

	// Assign each signal to its prefix (if count >= 2) or "other".
	type assignment struct {
		field  *ast.FieldDefinition
		prefix string
	}
	var assigns []assignment
	for _, f := range signals {
		p := firstCamelPrefix(f.Name)
		if p != "" && prefixCount[p] >= 2 {
			assigns = append(assigns, assignment{field: f, prefix: p})
		} else {
			assigns = append(assigns, assignment{field: f, prefix: "other"})
		}
	}

	// Group by prefix, maintaining first-seen order.
	var prefixOrder []string
	groupMap := map[string]*signalCategoryInfo{}
	for _, a := range assigns {
		if g, ok := groupMap[a.prefix]; ok {
			g.fields = append(g.fields, a.field)
		} else {
			label := prefixToLabel(a.prefix)
			g := &signalCategoryInfo{label: label, prefix: a.prefix, fields: []*ast.FieldDefinition{a.field}}
			groupMap[a.prefix] = g
			prefixOrder = append(prefixOrder, a.prefix)
		}
	}

	categories := make([]signalCategoryInfo, 0, len(prefixOrder))
	for _, p := range prefixOrder {
		categories = append(categories, *groupMap[p])
	}
	return categories
}

// firstCamelPrefix returns the first word of a camelCase name (before the first uppercase letter),
// or "" if the name has no camelCase boundary.
func firstCamelPrefix(name string) string {
	for i, r := range name {
		if i > 0 && r >= 'A' && r <= 'Z' {
			return name[:i]
		}
	}
	return ""
}

// prefixToLabel converts a depth-1 camelCase prefix to an uppercase category label.
func prefixToLabel(prefix string) string {
	return strings.ToUpper(prefix)
}

// dominantPrivilege finds the most common privilege string among a group of fields.
// On ties, the alphabetically-first privilege wins for determinism.
func dominantPrivilege(fields []*ast.FieldDefinition) string {
	counts := map[string]int{}
	for _, f := range fields {
		m := privilegeRegexp.FindStringSubmatch(f.Description)
		if len(m) >= 2 {
			counts[m[1]]++
		}
	}
	best := ""
	bestCount := 0
	for p, c := range counts {
		if c > bestCount || (c == bestCount && (best == "" || p < best)) {
			best = p
			bestCount = c
		}
	}
	return best
}

// extractPrivilege returns the privilege string from a single field's description.
func extractPrivilege(desc string) string {
	m := privilegeRegexp.FindStringSubmatch(desc)
	if len(m) >= 2 {
		return m[1]
	}
	return ""
}

// groupSignalFields groups fields by their return type and argument signature.
func groupSignalFields(fields []*ast.FieldDefinition) []signalGroup {
	type groupKey struct {
		returnType string
		argSig     string
	}
	keyOrder := []groupKey{}
	groupMap := map[groupKey]*signalGroup{}

	for _, f := range fields {
		rt := f.Type.String()
		sig := buildArgSignature(f.Arguments)
		key := groupKey{returnType: rt, argSig: sig}
		if g, ok := groupMap[key]; ok {
			g.fields = append(g.fields, f)
		} else {
			g := &signalGroup{returnType: rt, argSig: sig, fields: []*ast.FieldDefinition{f}}
			groupMap[key] = g
			keyOrder = append(keyOrder, key)
		}
	}

	groups := make([]signalGroup, 0, len(keyOrder))
	for _, k := range keyOrder {
		groups = append(groups, *groupMap[k])
	}
	return groups
}

// buildArgSignature creates a compact string representation of a field's argument list.
func buildArgSignature(args ast.ArgumentDefinitionList) string {
	if len(args) == 0 {
		return "()"
	}
	var parts []string
	for _, a := range args {
		parts = append(parts, a.Name+": "+a.Type.String())
	}
	return "(" + strings.Join(parts, ", ") + ")"
}

// extractUnit pulls the unit string from a signal field description, e.g. "Unit: 'km/h'" → "km/h".
func extractUnit(desc string) string {
	m := unitRegexp.FindStringSubmatch(desc)
	if len(m) >= 2 {
		return m[1]
	}
	return ""
}

// extractShortDescription returns the first sentence of a description, stripping unit/privilege info.
func extractShortDescription(desc string) string {
	// Take first line or sentence.
	s := desc
	if idx := strings.Index(s, "\n"); idx >= 0 {
		s = s[:idx]
	}
	if idx := strings.Index(s, ". "); idx >= 0 {
		s = s[:idx]
	}
	s = strings.TrimSuffix(s, ".")
	return strings.TrimSpace(s)
}
