package tsgen

import (
	"testing"

	"github.com/d0whc3r/protoc-gen-strict/internal/parser"
)

// TestIgnoreAlwaysStopsPropagation covers a message-typed field validation is
// switched off for. IGNORE_ALWAYS skips protovalidate's recursion into the
// message too, so demanding the target's strict type would reject a value the
// proto accepts.
func TestIgnoreAlwaysStopsPropagation(t *testing.T) {
	const target = "example.v1.Detail"
	c := &Context{needsStrict: map[string]bool{target: true}}

	ignored := parser.FieldMetadata{
		Name:      "unchecked",
		ProtoType: "message:" + target,
		Rules:     []parser.Rule{{Kind: "ignore", Value: "IGNORE_ALWAYS"}},
	}
	if got := c.narrowingOf(ignored).Target; got != "" {
		t.Errorf("Target = %q, want none for an IGNORE_ALWAYS field", got)
	}

	// IGNORE_IF_ZERO_VALUE only skips the unset case, which the optional
	// property protoc-gen-es declares already allows.
	whenSet := parser.FieldMetadata{
		Name:      "detail",
		ProtoType: "message:" + target,
		Rules:     []parser.Rule{{Kind: "ignore", Value: "IGNORE_IF_ZERO_VALUE"}},
	}
	if got := c.narrowingOf(whenSet).Target; got != target {
		t.Errorf("Target = %q, want %q", got, target)
	}
}

// TestEnumNarrowingStacksOnAlias covers an enum rule alongside the excluded
// zero: the rule narrows the enum's strict alias, so the zero stays out however
// the rule is written. A rule naming the zero itself is dropped, since the
// alias already excludes it.
func TestEnumNarrowingStacksOnAlias(t *testing.T) {
	const flavor = "shop.coverage.v1.Flavor"
	c := &Context{strictEnums: map[string]bool{flavor: true}}

	field := func(rules ...parser.Rule) parser.FieldMetadata {
		return parser.FieldMetadata{Name: "flavor", ProtoType: "enum:" + flavor, Rules: rules}
	}

	bare := c.fieldNarrowing(field())
	if bare.Enum != flavor || bare.Exclude != "" {
		t.Errorf("bare enum field: Enum = %q, Exclude = %q; want %q, none", bare.Enum, bare.Exclude, flavor)
	}

	// enum.not_in = [0] says exactly what the alias already does.
	redundant := c.fieldNarrowing(field(parser.Rule{Kind: "enum.not_in", Value: "[0]"}))
	if redundant.Enum != flavor || redundant.Exclude != "" {
		t.Errorf("not_in [0]: Enum = %q, Exclude = %q; want %q, none", redundant.Enum, redundant.Exclude, flavor)
	}
	if !redundant.consumed["enum.not_in"] {
		t.Error("not_in [0] was dropped without being reported as carried")
	}

	// A rule excluding something else keeps its own term on top of the alias.
	other := c.fieldNarrowing(field(parser.Rule{Kind: "enum.not_in", Value: "[1, 0]"}))
	if other.Enum != flavor || other.Exclude != "1" {
		t.Errorf("not_in [1, 0]: Enum = %q, Exclude = %q; want %q, \"1\"", other.Enum, other.Exclude, flavor)
	}

	// An enum no file in this run declares has no alias to narrow off.
	foreign := (&Context{strictEnums: map[string]bool{}}).fieldNarrowing(field())
	if !foreign.isZero() {
		t.Errorf("enum outside the run narrowed to %+v, want nothing", foreign)
	}
}

// TestStringRulesLeaveLengthToRuntime covers the two string rules a shape type
// can carry and the one it cannot. `Uuid` and `Email` are template literal
// types, so a literal of the right shape is assignable with no constructor to
// call — but no string shape excludes the empty string, so string.min_len is
// reported as left to runtime rather than narrowed.
func TestStringRulesLeaveLengthToRuntime(t *testing.T) {
	c := &Context{}
	field := func(rules ...parser.Rule) parser.FieldMetadata {
		return parser.FieldMetadata{Name: "value", ProtoType: "string", Rules: rules}
	}

	shaped := c.fieldNarrowing(field(
		parser.Rule{Kind: "string.uuid", Value: "true"},
		parser.Rule{Kind: "string.min_len", Value: "1"},
	))
	if got := shaped.Brands; len(got) != 1 || got[0] != "Uuid" {
		t.Errorf("Brands = %v, want [Uuid] alone", got)
	}
	if shaped.consumed["string.min_len"] {
		t.Error("string.min_len reported as carried; no string shape excludes the empty string")
	}

	// min_len on its own narrows nothing at all.
	length := c.fieldNarrowing(field(parser.Rule{Kind: "string.min_len", Value: "1"}))
	if !length.isZero() {
		t.Errorf("string.min_len narrowed to %+v, want nothing", length)
	}

	// `required` on a string is the same bound by another name.
	required := c.fieldNarrowing(parser.FieldMetadata{Name: "value", ProtoType: "string", Required: true})
	if !required.isZero() || required.consumed["required"] {
		t.Errorf("required string narrowed to %+v, want nothing carried", required)
	}
}
