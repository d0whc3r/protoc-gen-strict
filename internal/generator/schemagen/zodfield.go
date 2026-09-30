package schemagen

import (
	"cmp"
	"slices"
	"strconv"
	"strings"

	"github.com/d0whc3r/protoc-gen-strict/internal/generator/emit"
	"github.com/d0whc3r/protoc-gen-strict/internal/parser"
)

// A rule is a native Zod check only when a Zod built-in does exactly what
// protovalidate does, with the same API in both majors (or per major where
// they differ). Everything else goes through field(), which runs protovalidate
// itself. See "Native checks and protovalidate" in docs/rule-coverage-schema.md.

// fixedPatterns are the formats protovalidate-es evaluates with a constant
// platform RegExp (`fixedPattern` in native/string.ts, `isEmail` in lib.ts),
// copied verbatim. The same engine and source make them exact.
var fixedPatterns = map[string]string{
	"uuid":             `/^[0-9a-fA-F]{8}-[0-9a-fA-F]{4}-[0-9a-fA-F]{4}-[0-9a-fA-F]{4}-[0-9a-fA-F]{12}$/`,
	"tuuid":            `/^[0-9a-fA-F]{32}$/`,
	"ulid":             `/^[0-7][0-9A-HJKMNP-TV-Za-hjkmnp-tv-z]{25}$/`,
	"protobuf_fqn":     `/^[A-Za-z_][A-Za-z_0-9]*(\.[A-Za-z_][A-Za-z_0-9]*)*$/`,
	"protobuf_dot_fqn": `/^\.[A-Za-z_][A-Za-z_0-9]*(\.[A-Za-z_][A-Za-z_0-9]*)*$/`,
	"email":            "/^[a-zA-Z0-9.!#$%&'*+/=?^_`{|}~-]+@[a-zA-Z0-9](?:[a-zA-Z0-9-]{0,61}[a-zA-Z0-9])?(?:\\.[a-zA-Z0-9](?:[a-zA-Z0-9-]{0,61}[a-zA-Z0-9])?)*$/",
}

// stringChecks are the StringRules with a Zod method of the same meaning, and
// the major that has it: Zod counts code points, like protovalidate, since 4.5.
var stringChecks = map[string]struct {
	method string
	since  ZodMajor
}{
	"min_len":  {".min", Zod4},
	"max_len":  {".max", Zod4},
	"len":      {".length", Zod4},
	"prefix":   {".startsWith", Zod3},
	"suffix":   {".endsWith", Zod3},
	"contains": {".includes", Zod3},
}

// boundChecks are the numeric bounds, each a Zod number method.
var boundChecks = map[string]string{"gt": ".gt", "gte": ".gte", "lt": ".lt", "lte": ".lte"}

// zodField is one property of a <message>ZodObject.
type zodField struct {
	expr      string   // the schema, without field() and the trailing .optional()
	typ       string   // its TypeScript type, e.g. "z.ZodString", for a .d.ts
	native    []string // rule kinds its Zod checks enforce, in rule order
	checked   []string // rule kinds and CEL labels left to protovalidate
	required  bool     // the key is mandatory
	recursive bool     // it names a message of the same cycle
	opaque    bool     // z.unknown() for a message: field() still decodes it, as protojson does
	note      string   // why the schema is looser than the rules, if it is
}

// refined reports whether the field's schema carries field().
func (zf zodField) refined() bool { return len(zf.checked) > 0 || zf.opaque }

// valueSpec is what one value's schema is built from: the field itself, or
// the elements of a repeated field.
type valueSpec struct {
	protoType string
	prefix    string        // rule prefix, "" or "repeated.items."
	rules     []parser.Rule // the rules under prefix, full kinds
	nativeOK  bool          // false under `ignore`, where protovalidate decides alone
	nonZero   bool          // `required` on an implicit field: the zero value is out
	unchecked bool          // IGNORE_ALWAYS: protovalidate never evaluates a nested message
}

// value is a built schema and the rule kinds it enforces.
type value struct {
	expr    string
	typ     string // the TypeScript type of expr, e.g. "z.ZodString"
	handled []string
	nonZero bool   // it enforces valueSpec.nonZero
	ref     string // the message it names, when it names one
	opaque  bool   // z.unknown() for a message
	note    string // why it is looser than the rules, when it is
}

// buildField chooses the schema and checks of one field.
func (w *zodWriter) buildField(field parser.FieldMetadata) zodField {
	var zf zodField
	ignore := field.Ignore != ""
	always := field.Ignore == emit.IgnoreAlways
	root, items := splitRules(field)
	presence := field.Presence
	nonZero := field.Required && !ignore && !presence

	var handled []string
	switch {
	case field.IsMap:
		// Keys are always strings in JSON; key and value rules go to protovalidate.
		v := w.valueSchema(valueSpec{protoType: field.MapValue, unchecked: uncheckedValue(field)})
		zf.expr = w.z() + ".record(" + w.z() + ".string(), " + v.expr + ")"
		zf.typ = w.zt("ZodRecord") + "<" + w.zt("ZodString") + ", " + v.typ + ">"
		zf.recursive, zf.opaque = w.inCycle(v.ref), v.opaque
		zf.note = cmp.Or(v.note, unknownNote(field.MapValue, v))
	case field.Repeated:
		itemsIgnore := ignoreValue(field, emit.RepeatedItems)
		v := w.valueSchema(valueSpec{
			protoType: field.ProtoType, prefix: emit.RepeatedItems, rules: items,
			nativeOK: !ignore && itemsIgnore == "", unchecked: uncheckedValue(field),
		})
		handled = append(handled, v.handled...)
		zf.expr = w.z() + ".array(" + v.expr + ")"
		zf.typ = w.zt("ZodArray") + "<" + v.typ + ">"
		zf.recursive, zf.opaque = w.inCycle(v.ref), v.opaque
		zf.note = cmp.Or(v.note, unknownNote(field.ProtoType, v))
		if !ignore {
			for _, rule := range root {
				switch rule.Kind {
				case "repeated.min_items":
					zf.expr += ".min(" + rule.Value + ")"
					handled = append(handled, rule.Kind)
				case "repeated.max_items":
					zf.expr += ".max(" + rule.Value + ")"
					handled = append(handled, rule.Kind)
				}
			}
			if nonZero {
				zf.expr += ".min(1)" // required on a list: not empty
				handled = append(handled, emit.RequiredRule)
			}
		}
	default:
		v := w.valueSchema(valueSpec{protoType: field.ProtoType, rules: root, nativeOK: !ignore, nonZero: nonZero, unchecked: always})
		handled = append(handled, v.handled...)
		zf.expr, zf.typ = v.expr, v.typ
		zf.recursive, zf.opaque = w.inCycle(v.ref), v.opaque
		if v.nonZero {
			handled = append(handled, emit.RequiredRule)
		}
		zf.note = cmp.Or(v.note, unknownNote(field.ProtoType, v))
	}

	// protovalidate keeps `required` under every ignore but IGNORE_ALWAYS.
	if field.Required && !always {
		zf.required = true
		if presence {
			handled = append(handled, emit.RequiredRule) // the mandatory key is the whole rule
		}
	}

	// Whatever the Zod checks do not enforce, protovalidate does.
	for _, rule := range field.Rules {
		if isNoop(rule) || leafOf(rule.Kind) == emit.IgnoreRule {
			continue
		}
		if slices.Contains(handled, rule.Kind) {
			zf.native = append(zf.native, rule.Kind)
			continue
		}
		zf.checked = append(zf.checked, rule.Kind)
	}
	if field.Required {
		if slices.Contains(handled, emit.RequiredRule) {
			zf.native = append([]string{emit.RequiredRule}, zf.native...)
		} else {
			zf.checked = append([]string{emit.RequiredRule}, zf.checked...)
		}
	}
	for _, rule := range field.CEL {
		zf.checked = append(zf.checked, "cel["+emit.CELLabel(rule)+"]")
	}
	return zf
}

// unknownNote explains a z.unknown(): the message comes from a file this run
// writes no module for.
func unknownNote(protoType string, v value) string {
	kind, target, ok := referenceTarget(protoType)
	if !ok || kind != "message" || v.ref != "" {
		return ""
	}
	return target + " is not generated in this run, so the field takes any value that decodes as one."
}

// uncheckedValue reports whether protovalidate never evaluates what a field
// holds: IGNORE_ALWAYS on the field, or on its list items or map values.
func uncheckedValue(field parser.FieldMetadata) bool {
	switch {
	case field.Ignore == emit.IgnoreAlways:
		return true
	case field.IsMap:
		return ignoreValue(field, "map.values.") == emit.IgnoreAlways
	case field.Repeated:
		return ignoreValue(field, emit.RepeatedItems) == emit.IgnoreAlways
	}
	return false
}

// splitRules separates the rules on a field from those on its elements.
// `ignore` and the no-op rules stay in both lists: valueSchema skips them.
func splitRules(field parser.FieldMetadata) (root, items []parser.Rule) {
	for _, rule := range field.Rules {
		if strings.HasPrefix(rule.Kind, emit.RepeatedItems) {
			items = append(items, rule)
			continue
		}
		root = append(root, rule)
	}
	return root, items
}

// valueSchema builds the schema of one value and its native checks.
func (w *zodWriter) valueSchema(spec valueSpec) value {
	rule := func(leaf string) (parser.Rule, bool) {
		for _, r := range spec.rules {
			if r.Kind == spec.prefix+leaf {
				return r, true
			}
		}
		return parser.Rule{}, false
	}
	kind, target, isRef := referenceTarget(spec.protoType)
	typ := spec.protoType

	switch {
	case isRef && kind == "message" && spec.unchecked && !isWKT(target):
		// The nested message's own schema would apply rules protovalidate skips.
		return value{
			expr: w.z() + ".unknown()", typ: w.zt("ZodUnknown"), opaque: true,
			note: "protovalidate never evaluates the " + target + " inside, so the field takes any value that decodes as one.",
		}
	case isRef && kind == "message":
		expr, typ, ref := w.messageRef(target)
		return value{expr: expr, typ: typ, ref: ref, opaque: ref == ""}
	case isRef && kind == "enum":
		return w.enumSchema(target, spec, rule)
	case typ == "string":
		return w.stringSchema(spec, rule)
	case typ == "bytes":
		return value{expr: w.z() + ".string()", typ: w.zt("ZodString")}
	case typ == "bool":
		c, hasConst := rule("bool.const")
		switch {
		case spec.nativeOK && hasConst:
			return value{
				expr: w.z() + ".literal(" + c.Value + ")", typ: w.zt("ZodLiteral") + "<" + c.Value + ">",
				handled: []string{c.Kind}, nonZero: spec.nonZero && c.Value == "true",
			}
		case spec.nativeOK && spec.nonZero:
			// required: not false
			return value{expr: w.z() + ".literal(true)", typ: w.zt("ZodLiteral") + "<true>", nonZero: true}
		}
		return value{expr: w.z() + ".boolean()", typ: w.zt("ZodBoolean")}
	case int32Kinds[typ]:
		return w.int32Schema(spec, rule)
	case floatKinds[typ]:
		special := []string{"NaN", "Infinity", "-Infinity"}
		return value{
			expr: w.z() + ".union([" + w.z() + ".number(), " + w.enumExpr(special) + "])",
			typ:  w.unionType([]string{w.zt("ZodNumber"), w.enumType(special)}),
		}
	}
	if signed, ok := int64Kinds[typ]; ok {
		if signed {
			return value{expr: w.z() + ".string().regex(/^-?[0-9]+$/)", typ: w.zt("ZodString")}
		}
		return value{expr: w.z() + ".string().regex(/^[0-9]+$/)", typ: w.zt("ZodString")}
	}
	return value{expr: w.z() + ".unknown()", typ: w.zt("ZodUnknown")} // unreachable for a proto3 field type
}

func (w *zodWriter) stringSchema(spec valueSpec, rule func(string) (parser.Rule, bool)) value {
	if spec.nativeOK {
		if c, ok := rule("string.const"); ok {
			return value{
				expr: w.z() + ".literal(" + tsString(c.Value) + ")", typ: w.zt("ZodLiteral") + "<" + tsString(c.Value) + ">",
				handled: []string{c.Kind}, nonZero: spec.nonZero && c.Value != "",
			}
		}
		if in, ok := rule("string.in"); ok {
			return value{
				expr: w.enumExpr(in.Values), typ: w.enumType(in.Values),
				handled: []string{in.Kind}, nonZero: spec.nonZero && !slices.Contains(in.Values, ""),
			}
		}
	}

	v := value{expr: w.z() + ".string()", typ: w.zt("ZodString")}
	if !spec.nativeOK {
		return v
	}
	for _, r := range spec.rules {
		leaf := strings.TrimPrefix(r.Kind, spec.prefix+"string.")
		if check, ok := stringChecks[leaf]; ok && w.major >= check.since {
			arg := r.Value
			if check.method == ".startsWith" || check.method == ".endsWith" || check.method == ".includes" {
				arg = tsString(r.Value)
			}
			v.expr += check.method + "(" + arg + ")"
			v.handled = append(v.handled, r.Kind)
			continue
		}
		if pattern, ok := fixedPatterns[leaf]; ok && r.Value == "true" {
			v.expr += ".regex(" + pattern + ")"
			v.handled = append(v.handled, r.Kind)
		}
	}
	if spec.nonZero {
		v.expr += ".min(1)" // required: not ""; one UTF-16 unit is one code point or more
		v.nonZero = true
	}
	return v
}

func (w *zodWriter) int32Schema(spec valueSpec, rule func(string) (parser.Rule, bool)) value {
	typ := spec.protoType
	if spec.nativeOK {
		if c, ok := rule(typ + ".const"); ok {
			return value{expr: w.z() + ".literal(" + c.Value + ")", typ: w.zt("ZodLiteral") + "<" + c.Value + ">", handled: []string{c.Kind}}
		}
		if in, ok := rule(typ + ".in"); ok {
			literals := make([]string, len(in.Values))
			types := make([]string, len(in.Values))
			for i, v := range in.Values {
				literals[i] = w.z() + ".literal(" + v + ")"
				types[i] = w.zt("ZodLiteral") + "<" + v + ">"
			}
			if len(literals) == 1 {
				return value{expr: literals[0], typ: types[0], handled: []string{in.Kind}}
			}
			return value{
				expr:    w.z() + ".union([" + strings.Join(literals, ", ") + "])",
				typ:     w.unionType(types),
				handled: []string{in.Kind},
			}
		}
	}

	v := value{expr: w.z() + ".number().int()", typ: w.zt("ZodNumber")}
	if !spec.nativeOK {
		return v
	}
	// A lower bound above the upper one is a range read inside out, which
	// chained checks cannot say; protovalidate keeps both.
	reversed := emit.ReversedBounds(spec.rules)
	for _, r := range spec.rules {
		leaf := strings.TrimPrefix(r.Kind, spec.prefix+typ+".")
		method, ok := boundChecks[leaf]
		if !ok || reversed[r.Kind] {
			continue
		}
		if _, err := strconv.ParseInt(r.Value, 10, 64); err != nil {
			continue
		}
		v.expr += method + "(" + r.Value + ")"
		v.handled = append(v.handled, r.Kind)
	}
	return v
}

// enumSchema is the enum's own schema, or a filtered copy of its member names
// when a rule narrows them. The JSON form of an enum is its member's name, so
// every EnumRules rule is a set of names.
func (w *zodWriter) enumSchema(fullName string, spec valueSpec, rule func(string) (parser.Rule, bool)) value {
	// NullValue's only JSON form is null, not a member name: its rules go to
	// protovalidate.
	if fullName == wktPackage+"NullValue" {
		expr, typ := w.enumRef(fullName)
		return value{expr: expr, typ: typ}
	}
	enum, known := w.ctx.enums[fullName]
	var narrowing []parser.Rule
	for _, leaf := range []string{"enum.const", "enum.in", "enum.not_in", "enum.defined_only"} {
		if r, ok := rule(leaf); ok {
			narrowing = append(narrowing, r)
		}
	}
	if !spec.nativeOK || !known || (len(narrowing) == 0 && !spec.nonZero) {
		expr, typ := w.enumRef(fullName)
		return value{expr: expr, typ: typ}
	}

	v := value{nonZero: spec.nonZero}
	for _, r := range narrowing {
		v.handled = append(v.handled, r.Kind)
	}
	// `required` on an implicit enum rules out the zero member.
	if spec.nonZero {
		narrowing = append(narrowing, parser.Rule{Kind: "enum.not_in", Values: []string{"0"}})
	}
	var names []string
	for _, member := range enum.Values {
		if keepMember(member, narrowing) {
			names = append(names, member.Name)
		}
	}
	v.expr, v.typ = w.enumExpr(names), w.enumType(names)
	return v
}

// keepMember reports whether an enum member satisfies every narrowing rule.
func keepMember(member parser.EnumValue, rules []parser.Rule) bool {
	number := strconv.Itoa(int(member.Number))
	for _, r := range rules {
		switch leafOf(r.Kind) {
		case "const":
			if r.Value != number {
				return false
			}
		case "in":
			if !slices.Contains(r.Values, number) {
				return false
			}
		case "not_in":
			if slices.Contains(r.Values, number) {
				return false
			}
		}
	}
	return true
}

// unionType is the type of z.union over schemas of the given types: Zod 4
// infers the options as a readonly tuple, Zod 3 as a plain one.
func (w *zodWriter) unionType(types []string) string {
	tuple := "[" + strings.Join(types, ", ") + "]"
	if w.major == Zod4 {
		tuple = "readonly " + tuple
	}
	return w.zt("ZodUnion") + "<" + tuple + ">"
}
