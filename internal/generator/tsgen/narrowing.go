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
	Brands   []string // string shape types from strict/types, intersected
	Extract  string   // enum.in or enum.const, as a union of numeric literals
	Exclude  string   // enum.not_in, likewise
	Enum     string   // fq name of an enum-typed field whose strict alias applies
	List     bool     // repeated.min_items of at least one
	Required bool     // (buf.validate.field).required on a field declared `?:`
	Target   string   // fq name of a message-typed field whose target narrows

	// The rule kinds that produced the narrowing, so the rest can be reported
	// as left to runtime validation.
	consumed map[string]bool
}

func (n narrowing) isZero() bool {
	return len(n.Brands) == 0 && n.Extract == "" && n.Exclude == "" &&
		n.Enum == "" && !n.List && !n.Required && n.Target == ""
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
	//
	// string.min_len is not among them: a template literal type has no shape
	// that excludes the empty string, and expressing it would take a nominal
	// type no caller can produce without a constructor. Left to runtime.
	if field.ProtoType == "string" && !field.Repeated {
		if value, ok := emit.RuleValue(field, "string.uuid"); ok && value == "true" {
			n.Brands = append(n.Brands, "Uuid")
			n.consumed["string.uuid"] = true
		}
		if value, ok := emit.RuleValue(field, "string.email"); ok && value == "true" {
			n.Brands = append(n.Brands, "Email")
			n.consumed["string.email"] = true
		}
	}

	// A numeric TS enum member is a subtype of its literal, and protovalidate
	// stores these values as plain int32, so the union filters the generated
	// enum without this plugin learning a member name.
	if name, ok := strings.CutPrefix(field.ProtoType, "enum:"); ok {
		if !field.Repeated {
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

		// Every enum field also drops the member protobuf numbers 0, rule or no
		// rule. That is a naming convention — buf lint's ENUM_ZERO_VALUE_SUFFIX
		// calls it <ENUM>_UNSPECIFIED, "unset" rather than a value — so the
		// enum's strict alias is the base an enum rule then narrows further.
		// See docs/rule-coverage-typescript.md.
		if c.strictEnums[name] {
			n.Enum = name
			n.Exclude = withoutZero(n.Exclude)
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
		case n.Enum != "":
			// The enum's strict alias already rules the zero member out.
		case strings.HasPrefix(field.ProtoType, "enum:") && n.Extract == "" && n.Exclude == "":
			n.Exclude = "0"
		default:
			// A string, numeric or bytes zero has no type to exclude it from,
			// and a map's is an index signature. Left to runtime validation.
			delete(n.consumed, "required")
		}
	}
	return n
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

// withoutZero drops the zero from a union of enum members. The enum's strict
// alias already excludes it, so a rule naming it too would only repeat the
// exclusion — and an enum.not_in of the zero alone would leave nothing to say.
func withoutZero(members string) string {
	if members == "" {
		return ""
	}
	kept := slices.DeleteFunc(strings.Split(members, " | "), func(m string) bool { return m == "0" })
	return strings.Join(kept, " | ")
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
