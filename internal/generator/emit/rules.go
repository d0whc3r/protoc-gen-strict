package emit

import (
	"fmt"
	"strconv"
	"strings"

	"github.com/d0whc3r/protoc-gen-strict/internal/parser"
)

// IgnoreRule is protovalidate's switch for when the sibling rules are evaluated
// at all. It sits next to them, both on a field and under `repeated.items`.
const IgnoreRule = "ignore"

// RequiredRule is `(buf.validate.field).required`, which the parser lifts out of
// the rule list into a flag of its own.
const RequiredRule = "required"

// RepeatedItems prefixes the rules describing a list's elements rather than the
// list, e.g. "repeated.items.string.min_len".
const RepeatedItems = "repeated.items."

// numericRules are the protovalidate rule messages whose bounds are plain
// numbers. `duration` and `timestamp` are absent: their bounds are messages, and
// the parser prints one rule per sub-field ("duration.gte.seconds").
var numericRules = map[string]bool{
	"int32": true, "sint32": true, "sfixed32": true,
	"uint32": true, "fixed32": true,
	"int64": true, "sint64": true, "sfixed64": true,
	"uint64": true, "fixed64": true,
	"float": true, "double": true,
}

// RuleValue returns the value of one rule on a field, as the parser rendered
// it, reporting false when the field does not carry that rule.
func RuleValue(field parser.FieldMetadata, kind string) (string, bool) {
	for _, rule := range field.Rules {
		if rule.Kind == kind {
			return rule.Value, true
		}
	}
	return "", false
}

// RuleLine renders one standard rule the way every overlay prints it.
func RuleLine(rule parser.Rule) string {
	return fmt.Sprintf("%s = %s", rule.Kind, rule.Value)
}

// ReversedBounds names the bound rules on a field that describe a range read
// inside out.
//
// protovalidate reads a lower bound above the upper one as a reversed range:
// `{gt: 20, lt: 10}` admits everything outside 10..20, not the empty set. Every
// target here spells a pair of bounds as a conjunction with no way to say "or",
// so carrying them as written would describe a value that cannot exist. Both are
// left to runtime validation instead.
func ReversedBounds(rules []parser.Rule) map[string]bool {
	lower, upper := map[string]parser.Rule{}, map[string]parser.Rule{}
	for _, rule := range rules {
		root, leaf, ok := boundRoot(rule.Kind)
		if !ok {
			continue
		}
		switch leaf {
		case "gt", "gte":
			lower[root] = rule
		case "lt", "lte":
			upper[root] = rule
		}
	}

	out := map[string]bool{}
	for root, lo := range lower {
		hi, ok := upper[root]
		if !ok {
			continue
		}
		loValue, loErr := strconv.ParseFloat(lo.Value, 64)
		hiValue, hiErr := strconv.ParseFloat(hi.Value, 64)
		if loErr != nil || hiErr != nil || loValue <= hiValue {
			continue
		}
		out[lo.Kind], out[hi.Kind] = true, true
	}
	return out
}

// boundRoot splits a numeric bound rule into the rule message holding it and the
// bound itself: "repeated.items.int32.gt" is "repeated.items.int32" and "gt".
// Two bounds only describe one range when they share a root.
func boundRoot(kind string) (string, string, bool) {
	prefix, leaf, ok := strings.Cut(strings.TrimPrefix(kind, RepeatedItems), ".")
	if !ok || !numericRules[prefix] {
		return "", "", false
	}
	return strings.TrimSuffix(kind, "."+leaf), leaf, true
}
