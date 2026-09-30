package schemagen

import (
	"maps"
	"slices"
	"strings"

	"github.com/d0whc3r/protoc-gen-strict/internal/generator/emit"
	"github.com/d0whc3r/protoc-gen-strict/internal/parser"
)

// This file mirrors protoschema-jsonschema v0.6.0 (internal/protoschema/
// jsonschema/jsonschema.go): for each rule, whether its output is exact, and
// which keywords reject values protovalidate accepts and so have to go. It is
// the one place to update when upstream changes; `make verify` asserts every
// fix still finds its keyword.

// The positions under a field's schema that a rule prefix lands on.
const (
	atField  = ""
	atItems  = "items"                // repeated.items.*
	atKeys   = "propertyNames"        // map.keys.*
	atValues = "additionalProperties" // map.values.*
)

// position is one place under a field's schema and the rules that describe it.
type position struct {
	at        string // one of the at* constants
	prefix    string // the rule prefix, e.g. "repeated.items."
	valueType string // ProtoType of the value there, e.g. "string"
}

// jsonFix is one set of keywords to delete at a position, and the rules or
// shapes that made each of them wrong, for the comment in the table.
type jsonFix struct {
	at      string
	drop    []string
	reasons []string
}

// jsonPlan is what the JSON Schema target does with one field: the keywords to
// delete, and the rules its schema does not carry.
type jsonPlan struct {
	fixes   []jsonFix
	runtime []string // rule lines for "Left to runtime validation"
}

// ifZeroKeywords are every rule-derived keyword protoschema-jsonschema writes.
// `ignore: IGNORE_IF_ZERO_VALUE` skips the rules for the zero value, which the
// keywords still reject, so all of them go.
var ifZeroKeywords = []string{
	"enum", "exclusiveMaximum", "exclusiveMinimum", "format", "maxItems", "maxLength",
	"maximum", "minItems", "minLength", "minimum", "pattern",
}

// patternFormats are the StringRules formats upstream writes as a `pattern`
// that rejects valid values: a trailing-dot hostname, a zone id, a URI with
// userinfo, a prefix with host bits in the last octet.
var patternFormats = map[string]bool{
	"hostname": true, "ip": true, "uri": true, "uri_ref": true, "address": true,
	"host_and_port": true, "ip_with_prefixlen": true, "ipv6_with_prefixlen": true,
	"ip_prefix": true, "ipv4_prefix": true, "ipv6_prefix": true,
}

// formatFormats are the formats upstream writes as a `format`, whose grammar is
// the validator's rather than protovalidate's.
var formatFormats = map[string]bool{"email": true, "ipv6": true}

// planJSON classifies every rule on one field.
func planJSON(field parser.FieldMetadata) jsonPlan {
	var p jsonPlan
	drops := map[string]map[string]bool{}
	reasons := map[string][]string{}
	drop := func(at, reason string, keywords ...string) {
		if drops[at] == nil {
			drops[at] = map[string]bool{}
		}
		for _, k := range keywords {
			drops[at][k] = true
		}
		if !slices.Contains(reasons[at], reason) {
			reasons[at] = append(reasons[at], reason)
		}
	}
	// A nested message's def is reached through a $ref, and carries its rules.
	dropRef := func(pos position, reason string) {
		if kind, target, ok := referenceTarget(pos.valueType); ok && kind == "message" && !isWKT(target) {
			drop(pos.at, reason, "$ref")
		}
	}

	ignore := field.Ignore
	if line := emit.ImpliedIgnore(field); line != "" {
		p.runtime = append(p.runtime, line)
	}
	// Upstream writes a singular message field as a bare $ref, which carries
	// none of the field's own rules. A list of messages keeps its item bounds.
	message := strings.HasPrefix(field.ProtoType, "message:") && !field.IsMap && !field.Repeated

	for _, pos := range positions(field) {
		if pos.valueType == "bytes" {
			drop(pos.at, "bytes: base64 pattern rejects URL-safe", "pattern")
		}
		if pos.at == atKeys && field.MapKey == "bool" {
			drop(atField, "map<bool, _>: JSON keys are strings", atKeys)
		}

		rules := rulesAt(field, pos)
		// A field-level IGNORE_ALWAYS makes upstream drop every rule under the
		// field. protovalidate never evaluates a nested message then either,
		// though the $ref still applies the def's keywords.
		if ignore == emit.IgnoreAlways {
			dropRef(pos, "ignore = "+emit.IgnoreAlways+": protovalidate never evaluates the message")
			p.runtime = append(p.runtime, ruleLines(rules)...)
			continue
		}
		// Upstream reads `ignore` on the field only: under repeated.items or a
		// map it writes the keywords all the same. IGNORE_IF_ZERO_VALUE exempts
		// the zero value, which the keywords still reject.
		posIgnore := ignore
		if pos.at != atField {
			posIgnore = ignoreValue(field, pos.prefix)
		}
		if posIgnore != "" {
			if len(rules) > 0 {
				drop(pos.at, pos.prefix+"ignore = "+posIgnore+": upstream writes the keywords anyway", ifZeroKeywords...)
			}
			if posIgnore == emit.IgnoreAlways {
				dropRef(pos, pos.prefix+"ignore = "+emit.IgnoreAlways+": protovalidate never evaluates the message")
			}
			p.runtime = append(p.runtime, ruleLines(rules)...)
			continue
		}

		siblings := map[string]parser.Rule{}
		for _, rule := range rules {
			siblings[strings.TrimPrefix(rule.Kind, pos.prefix)] = rule
		}
		for _, rule := range rules {
			if message {
				p.runtime = append(p.runtime, emit.RuleLine(rule)) // upstream writes a bare $ref
				continue
			}
			carried, keywords := classifyJSON(strings.TrimPrefix(rule.Kind, pos.prefix), rule, siblings)
			if len(keywords) > 0 {
				drop(pos.at, rule.Kind, keywords...)
			}
			if !carried {
				p.runtime = append(p.runtime, emit.RuleLine(rule))
			}
		}
	}

	if field.Required {
		carried, keywords := requiredJSON(field, ignore)
		if len(keywords) > 0 {
			drop(atField, "required: a set field may hold \"\"", keywords...)
		}
		if !carried {
			p.runtime = append([]string{emit.RequiredRule}, p.runtime...)
		}
	}
	p.runtime = append(p.runtime, emit.CELComments(field.CEL)...)

	for _, at := range []string{atField, atItems, atKeys, atValues} {
		if len(drops[at]) > 0 {
			p.fixes = append(p.fixes, jsonFix{at: at, drop: slices.Sorted(maps.Keys(drops[at])), reasons: reasons[at]})
		}
	}
	return p
}

// positions lists where a field's rules land, in the order they print.
func positions(field parser.FieldMetadata) []position {
	switch {
	case field.IsMap:
		return []position{
			{at: atField},
			{at: atKeys, prefix: "map.keys.", valueType: field.MapKey},
			{at: atValues, prefix: "map.values.", valueType: field.MapValue},
		}
	case field.Repeated:
		return []position{
			{at: atField},
			{at: atItems, prefix: emit.RepeatedItems, valueType: field.ProtoType},
		}
	default:
		return []position{{at: atField, valueType: field.ProtoType}}
	}
}

// rulesAt returns the rules describing one position: the ones under its prefix,
// or, for the field itself, the ones under no nested prefix. `ignore` and the
// no-op rules are left out; they constrain nothing.
func rulesAt(field parser.FieldMetadata, pos position) []parser.Rule {
	var out []parser.Rule
	for _, rule := range field.Rules {
		if isNoop(rule) || leafOf(rule.Kind) == emit.IgnoreRule {
			continue
		}
		nested := strings.HasPrefix(rule.Kind, emit.RepeatedItems) ||
			strings.HasPrefix(rule.Kind, "map.keys.") || strings.HasPrefix(rule.Kind, "map.values.")
		switch {
		case pos.prefix == "" && !nested:
			out = append(out, rule)
		case pos.prefix != "" && strings.HasPrefix(rule.Kind, pos.prefix):
			out = append(out, rule)
		}
	}
	return out
}

// classifyJSON reports whether upstream's output for one rule is exact, and the
// keywords that reject valid values. kind has the position prefix removed, e.g.
// "string.min_len"; siblings holds the other rules at the same position, keyed
// the same way.
func classifyJSON(kind string, rule parser.Rule, siblings map[string]parser.Rule) (bool, []string) {
	typ, leaf, _ := strings.Cut(kind, ".")
	has := func(leaf string) bool { _, ok := siblings[typ+"."+leaf]; return ok }

	switch {
	case typ == "string":
		return classifyStringJSON(leaf, rule, siblings)
	case int32Kinds[typ]:
		// Upstream bounds the number branch only. Its string branch,
		// "^-?[0-9]+$", takes any integer, and protobuf-es reads "0" as 0.
		return false, nil
	case typ == "float":
		// Upstream writes a float32 bound as its shortest decimal (0.1), while
		// protovalidate compares the float32 widened to a double
		// (0.10000000149011612), which the decimal rejects.
		switch leaf {
		case "gt", "gte", "lt", "lte", "const", "in":
			return false, []string{"enum", "exclusiveMaximum", "exclusiveMinimum", "maximum", "minimum"}
		}
	case typ == "bool" && leaf == "const":
		return true, nil
	case typ == "enum":
		switch leaf {
		case "const", "in", "defined_only":
			return true, nil
		case "not_in":
			// Upstream narrows the numbers only under one of the other three.
			return has("const") || has("in") || has("defined_only"), nil
		}
	case typ == "repeated":
		switch leaf {
		case "min_items", "max_items":
			return true, nil
		}
	}
	return false, nil
}

func classifyStringJSON(leaf string, rule parser.Rule, siblings map[string]parser.Rule) (bool, []string) {
	has := func(leaf string) bool { _, ok := siblings["string."+leaf]; return ok }
	switch {
	case leaf == "min_len" || leaf == "max_len" || leaf == "len" || leaf == "const":
		return true, nil
	case leaf == "in":
		return !has("const"), nil
	case leaf == "uuid" || leaf == "tuuid":
		// The pattern is protovalidate's own, unless a pattern of the other
		// rules overwrote it upstream.
		return !has("pattern") && !has("prefix") && !has("suffix") && !has("contains"), nil
	case leaf == "len_bytes":
		// Upstream sets maxLength to max_bytes, which is 0 when unset; only
		// len or max_len overwrite it.
		if !has("max_bytes") && !has("len") && !has("max_len") {
			return false, []string{"maxLength"}
		}
	case leaf == "pattern":
		return false, []string{"pattern"} // RE2 source in an ECMA-262 keyword
	case leaf == "prefix" || leaf == "suffix" || leaf == "contains":
		if !has("pattern") {
			return false, []string{"pattern"} // synthesized without escaping
		}
	case patternFormats[leaf]:
		return false, []string{"pattern"}
	case formatFormats[leaf]:
		return false, []string{"format"}
	case leaf == "well_known_regex":
		// Upstream writes the header-name pattern only for an explicit
		// `strict: true`, with a `\'` escape the u flag rejects.
		if rule.Value == "KNOWN_REGEX_HTTP_HEADER_NAME" && siblings["string.strict"].Value == "true" {
			return false, []string{"pattern"}
		}
	}
	return false, nil
}

// requiredJSON classifies `required`. Upstream lists the field in the parent's
// `required`, which is exact for presence. An implicit field also needs a
// non-zero value: upstream spells that for bools and enums, and for strings
// and bytes only through `minLength: 1`, which it writes only when the field
// carries another rule of its type. On a field with presence that same
// `minLength: 1` is wrong, since a set field may hold "".
func requiredJSON(field parser.FieldMetadata, ignore string) (bool, []string) {
	if ignore != "" {
		return false, nil
	}
	if !field.Presence {
		switch {
		case field.Repeated || field.IsMap:
			return false, nil
		case field.ProtoType == "bool" || strings.HasPrefix(field.ProtoType, "enum:"):
			return true, nil
		case field.ProtoType == "string" || field.ProtoType == "bytes":
			return typedRules(field), nil
		}
		return false, nil
	}
	if minLengthFromRequired(field) {
		return true, []string{"minLength"}
	}
	return true, nil
}

// minLengthFromRequired reports whether upstream wrote `minLength: 1` because of
// `required` rather than because of a length rule.
func minLengthFromRequired(field parser.FieldMetadata) bool {
	if !typedRules(field) {
		return false // upstream returns before the length keywords
	}
	switch field.ProtoType {
	case "string":
		if _, ok := emit.RuleValue(field, "string.len"); ok {
			return false
		}
		minLen, ok := emit.RuleValue(field, "string.min_len")
		return !ok || minLen == "0"
	case "bytes":
		_, hasLen := emit.RuleValue(field, "bytes.len")
		_, hasMin := emit.RuleValue(field, "bytes.min_len")
		return !hasLen && !hasMin
	}
	return false
}

// typedRules reports whether a string or bytes field carries a rule of its own
// type, e.g. "string.email", the condition under which upstream writes any
// length keyword at all.
func typedRules(field parser.FieldMetadata) bool {
	return slices.ContainsFunc(field.Rules, func(rule parser.Rule) bool {
		// A bare `string: {}` block is reported as the rule "string" itself.
		return rule.Kind == field.ProtoType || strings.HasPrefix(rule.Kind, field.ProtoType+".")
	})
}

func ruleLines(rules []parser.Rule) []string {
	lines := make([]string, 0, len(rules))
	for _, rule := range rules {
		lines = append(lines, emit.RuleLine(rule))
	}
	return lines
}
