package tsgen

import (
	"slices"
	"strconv"
	"strings"

	"github.com/d0whc3r/protoc-gen-strict/internal/generator/emit"
	"github.com/d0whc3r/protoc-gen-strict/internal/parser"
)

// ignoreAlways is the protovalidate mode that skips a field entirely, nested
// message included.
const ignoreAlways = "IGNORE_ALWAYS"

// narrowing is what one field contributes to its message's strict type. The
// zero value means the field is left exactly as protoc-gen-es declared it.
type narrowing struct {
	Brands   []string // nominal string types from strict/types, intersected
	Extract  string   // enum.in or enum.const, as a union of numeric literals
	Exclude  string   // enum.not_in, likewise
	List     bool     // repeated.min_items of at least one
	Required bool     // (buf.validate.field).required on a field declared `?:`
	Target   string   // fq name of a message-typed field whose target narrows

	// The rule kinds that produced the narrowing, so the rest can be reported
	// as left to runtime validation.
	consumed map[string]bool
}

func (n narrowing) isZero() bool {
	return len(n.Brands) == 0 && n.Extract == "" && n.Exclude == "" &&
		!n.List && !n.Required && n.Target == ""
}

// fieldNarrowing resolves everything but the message-typed target, which waits
// on the fixed point in narrowingOf.
func (c *Context) fieldNarrowing(field parser.FieldMetadata) narrowing {
	n := narrowing{consumed: map[string]bool{}}

	// `ignore` means the rules do not always apply, so none describes the type.
	// IGNORE_ALWAYS drops them; IGNORE_IF_ZERO_VALUE admits the zero value too,
	// which leaves min_len and min_items saying nothing and turns the rest into
	// a union with the zero. Dropped and reported instead, since under-narrowing
	// is the safe direction; widen per rule if a schema needs it.
	if _, ok := emit.RuleValue(field, emit.IgnoreRule); ok {
		return n
	}

	// Brands describe one string; a rule under repeated.items describes the
	// element, which this plugin does not narrow.
	if field.ProtoType == "string" && !field.Repeated {
		if value, ok := emit.RuleValue(field, "string.uuid"); ok && value == "true" {
			n.addBrand("Uuid")
			n.consumed["string.uuid"] = true
		}
		if value, ok := emit.RuleValue(field, "string.email"); ok && value == "true" {
			n.addBrand("Email")
			n.consumed["string.email"] = true
		}
		// Only "at least one" has a type equivalent; the bound stays runtime.
		if intRule(field, "string.min_len") >= 1 {
			n.addBrand("NonEmpty")
			n.consumed["string.min_len"] = true
		}
	}

	// A numeric TS enum member is a subtype of its literal, and protovalidate
	// stores these values as plain int32, so the union filters the generated
	// enum without this plugin learning a member name.
	if strings.HasPrefix(field.ProtoType, "enum:") && !field.Repeated {
		for _, kind := range []string{"enum.const", "enum.in"} {
			if members, ok := enumMembers(field, kind); ok {
				n.Extract, n.consumed[kind] = members, true
				break
			}
		}
		if n.Extract == "" {
			if members, ok := enumMembers(field, "enum.not_in"); ok {
				n.Exclude, n.consumed["enum.not_in"] = members, true
			}
		}
	}

	if field.Repeated && intRule(field, "repeated.min_items") >= 1 {
		n.List, n.consumed["repeated.min_items"] = true, true
	}

	// `required` forbids the zero value. Where presence is tracked that is just
	// the Require wrapper; protovalidate counts an empty string as set there.
	// Where it is not, the zero is a value, and ruling it out narrows the type.
	if field.Required {
		n.consumed["required"] = true
		switch {
		case tsOptionalInGenerated(field):
			n.Required = true
		case field.Repeated:
			n.List = true
		case field.ProtoType == "string":
			n.addBrand("NonEmpty")
		case strings.HasPrefix(field.ProtoType, "enum:") && n.Extract == "" && n.Exclude == "":
			n.Exclude = "0"
		default:
			// A numeric or bytes zero has no type to exclude it from, and a
			// map's is an index signature. Left to runtime validation.
			delete(n.consumed, "required")
		}
	}
	return n
}

// addBrand keeps the brand list free of duplicates and of redundancy: a Uuid or
// an Email is never empty, so NonEmpty alongside one would only cost the caller
// a second constructor.
func (n *narrowing) addBrand(brand string) {
	if slices.Contains(n.Brands, brand) {
		return
	}
	if brand == "NonEmpty" && (slices.Contains(n.Brands, "Uuid") || slices.Contains(n.Brands, "Email")) {
		return
	}
	if brand == "Uuid" || brand == "Email" {
		n.Brands = slices.DeleteFunc(n.Brands, func(existing string) bool { return existing == "NonEmpty" })
	}
	n.Brands = append(n.Brands, brand)
}

// narrowingOf is fieldNarrowing plus the message-typed target, known only once
// the fixed point has run.
func (c *Context) narrowingOf(field parser.FieldMetadata) narrowing {
	n := c.fieldNarrowing(field)
	// IGNORE_ALWAYS switches off the recursive validation of the message the
	// field carries, not only the rules written beside it, so the target's
	// strict type does not describe this field either. Requiring it would
	// reject a message protovalidate accepts.
	if value, _ := emit.RuleValue(field, emit.IgnoreRule); value == ignoreAlways {
		return n
	}
	if target, ok := messageTarget(field); ok && c.needsStrict[target] {
		n.Target = target
	}
	return n
}

// tsOptionalInGenerated reports whether protoc-gen-es declared the field `?:`.
// Lists and maps are always present; a singular message or a proto3 `optional`
// is not.
func tsOptionalInGenerated(field parser.FieldMetadata) bool {
	if field.Repeated || field.IsMap {
		return false
	}
	return field.Optional || strings.HasPrefix(field.ProtoType, "message:")
}

// enumMembers renders an enum rule's value as a union of numeric literals.
func enumMembers(field parser.FieldMetadata, kind string) (string, bool) {
	value, ok := emit.RuleValue(field, kind)
	if !ok {
		return "", false
	}
	parts := strings.FieldsFunc(strings.Trim(value, "[]"), func(r rune) bool {
		return r == ',' || r == ' '
	})
	if len(parts) == 0 {
		return "", false
	}
	for _, part := range parts {
		if _, err := strconv.Atoi(part); err != nil {
			return "", false // an unexpected rendering; report it instead
		}
	}
	return strings.Join(parts, " | "), true
}

// intRule reads a numeric rule such as string.min_len, returning 0 when it is
// absent or rendered as something other than an integer.
func intRule(field parser.FieldMetadata, kind string) int {
	value, ok := emit.RuleValue(field, kind)
	if !ok {
		return 0
	}
	n, err := strconv.Atoi(value)
	if err != nil {
		return 0
	}
	return n
}
