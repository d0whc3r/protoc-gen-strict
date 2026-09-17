package parser

import (
	"cmp"
	"slices"
	"strings"

	validate "buf.build/gen/go/bufbuild/protovalidate/protocolbuffers/go/buf/validate"
	"google.golang.org/protobuf/reflect/protoreflect"
)

// standardRules flattens the non-CEL constraints via protoreflect rather than a
// per-type switch, so new protovalidate rules are picked up for free. Nested
// rule messages are prefixed with their field name: "string.min_len",
// "repeated.items.string.uuid".
func standardRules(rules *validate.FieldRules) []Rule {
	if rules == nil {
		return nil
	}
	return sortByKind(collect(rules.ProtoReflect(), ""))
}

// sortByKind keeps the output deterministic: protoreflect ranges in an
// unspecified order.
func sortByKind(rules []Rule) []Rule {
	slices.SortFunc(rules, func(a, b Rule) int { return cmp.Compare(a.Kind, b.Kind) })
	return rules
}

func collect(msg protoreflect.Message, prefix string) []Rule {
	var out []Rule
	msg.Range(func(fd protoreflect.FieldDescriptor, v protoreflect.Value) bool {
		name := prefix + string(fd.Name())
		switch {
		// These have fields of their own: CELRule, FieldMetadata.Required.
		case prefix == "" && (name == "cel" || name == "cel_expression" || name == "required"):
			return true
		// Nested rule message (string, int32, repeated, map, ...).
		case fd.Kind() == protoreflect.MessageKind && !fd.IsList() && !fd.IsMap():
			nested := collect(v.Message(), name+".")
			if len(nested) == 0 {
				// Set, but every field inside holds its default, e.g.
				// `timestamp.gte = {seconds: 0}`: report the rule itself
				// rather than dropping it.
				out = append(out, Rule{Kind: name, Value: "{}"})
				break
			}
			out = append(out, nested...)
		default:
			out = append(out, Rule{Kind: name, Value: formatValue(fd, v)})
		}
		return true
	})
	return out
}

func formatValue(fd protoreflect.FieldDescriptor, v protoreflect.Value) string {
	if fd.IsList() {
		list := v.List()
		parts := make([]string, 0, list.Len())
		for i := 0; i < list.Len(); i++ {
			parts = append(parts, formatElement(fd, list.Get(i)))
		}
		return "[" + strings.Join(parts, ", ") + "]"
	}
	return formatElement(fd, v)
}

// formatElement renders one value of the field's type. Messages go through the
// rule walk because Value.String() prints a Go pointer for them, and lists of
// messages are common (`duration.in`, `timestamp.not_in`).
func formatElement(fd protoreflect.FieldDescriptor, v protoreflect.Value) string {
	switch fd.Kind() {
	case protoreflect.MessageKind, protoreflect.GroupKind:
		return formatRuleMessage(v.Message())
	case protoreflect.EnumKind:
		if ev := fd.Enum().Values().ByNumber(v.Enum()); ev != nil {
			return string(ev.Name())
		}
	}
	return v.String()
}

func formatRuleMessage(msg protoreflect.Message) string {
	inner := sortByKind(collect(msg, ""))
	parts := make([]string, 0, len(inner))
	for _, rule := range inner {
		parts = append(parts, rule.Kind+": "+rule.Value)
	}
	return "{" + strings.Join(parts, ", ") + "}"
}
