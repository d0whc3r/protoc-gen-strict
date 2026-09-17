package pygen

import (
	"math"
	"strconv"
	"strings"

	"github.com/d0whc3r/protoc-gen-strict/internal/generator/emit"
	"github.com/d0whc3r/protoc-gen-strict/internal/parser"
)

// annotated_types is the vocabulary pydantic, msgspec and beartype all read, so
// a rule that maps onto it is enforced rather than merely described. The package
// is pure Python and depends on nothing.
const annotatedTypesModule = "annotated_types"

// outputOnlyNote is the AIP-203 annotation `(google.api.field_behavior) =
// OUTPUT_ONLY`: the server assigns the field and a caller must not set it.
// annotated_types has no vocabulary for it, so it is carried as metadata text.
const outputOnlyNote = "google.api.field_behavior = OUTPUT_ONLY"

// lengthConstraints are the rules counting the annotated type itself: code
// points of a `str`, bytes of a `bytes`, entries of a `Sequence` or a `Mapping`.
// A rule under `repeated.items` or `map.keys` counts something else and is
// absent for that reason.
var lengthConstraints = map[string]string{
	"string.min_len":     "MinLen",
	"string.max_len":     "MaxLen",
	"bytes.min_len":      "MinLen",
	"bytes.max_len":      "MaxLen",
	"repeated.min_items": "MinLen",
	"repeated.max_items": "MaxLen",
	"map.min_pairs":      "MinLen",
	"map.max_pairs":      "MaxLen",
}

// exactLengths fix one length. annotated_types spells that `Len(n, n)` and
// unpacks it into the two bounds below, which are emitted directly.
var exactLengths = map[string]bool{"string.len": true, "bytes.len": true}

// boundConstraints are the numeric bounds. The leaf alone identifies them: the
// rule messages carrying a bound that is not a number — `duration`, `timestamp`
// — print one rule per sub-field, so their kind never ends here.
var boundConstraints = map[string]string{
	"gt": "Gt", "gte": "Ge", "lt": "Lt", "lte": "Le",
}

// metadataOf renders a field's rules as Annotated metadata: an annotated_types
// constructor where the vocabulary has one, the rule text otherwise. A rule
// carried as a constructor is not also printed as a string.
func (p *pyImports) metadataOf(field parser.FieldMetadata) []string {
	var out []string
	if field.OutputOnly {
		out = append(out, strconv.Quote(outputOnlyNote))
	}
	if field.Required {
		out = append(out, strconv.Quote(emit.RequiredRule))
	}

	// `ignore` says the sibling rules do not always apply, so none of them
	// describes the type either — the call the other targets make too. A
	// reversed range is dropped for the reason emit.ReversedBounds gives.
	_, ignored := emit.RuleValue(field, emit.IgnoreRule)
	reversed := emit.ReversedBounds(field.Rules)

	for _, rule := range field.Rules {
		if !ignored && !reversed[rule.Kind] {
			if calls := p.constraints(rule); len(calls) > 0 {
				out = append(out, calls...)
				continue
			}
		}
		out = append(out, strconv.Quote(emit.RuleLine(rule)))
	}

	for _, line := range emit.CELComments(field.CEL) {
		out = append(out, strconv.Quote(line))
	}
	return out
}

// constraints maps one rule onto annotated_types calls, recording the import
// each needs. A rule the vocabulary cannot express yields none: under-narrowing
// is the safe direction.
func (p *pyImports) constraints(rule parser.Rule) []string {
	if name, ok := lengthConstraints[rule.Kind]; ok {
		return p.calls(rule.Value, name)
	}
	if exactLengths[rule.Kind] {
		return p.calls(rule.Value, "MinLen", "MaxLen")
	}
	_, leaf, ok := strings.Cut(rule.Kind, ".")
	if !ok {
		return nil
	}
	if name, ok := boundConstraints[leaf]; ok {
		return p.calls(rule.Value, name)
	}
	return nil
}

// calls renders one value through each constructor, or nothing when the parser
// did not print a Python numeric literal.
func (p *pyImports) calls(value string, names ...string) []string {
	if !pyNumber(value) {
		return nil
	}
	out := make([]string, 0, len(names))
	for _, name := range names {
		p.annotated[name] = true
		out = append(out, name+"("+value+")")
	}
	return out
}

// pyNumber reports whether a rule value is a Python numeric literal.
//
// The value is emitted as the parser printed it rather than re-rendered: a
// 64-bit bound loses precision through float64, and Python needs none of the
// rounding the OpenAPI target does. What it cannot take is the infinity and NaN
// spellings Go parses and Python writes as calls.
func pyNumber(value string) bool {
	if _, err := strconv.ParseInt(value, 10, 64); err == nil {
		return true
	}
	number, err := strconv.ParseFloat(value, 64)
	return err == nil && !math.IsInf(number, 0) && !math.IsNaN(number)
}

// enumAliases renders one `<Name>Strict` per enum the file declares: the class
// protoc-gen-python emitted, annotated so the member numbered 0 is out.
//
// It is the only alias here that no buf.validate rule produced. The convention
// that names that member <ENUM>_UNSPECIFIED makes it "unset" rather than a
// value, so every field of the enum's type is annotated with the alias.
func (p *pyImports) enumAliases(enums []parser.EnumMetadata) []pyBlock {
	var out []pyBlock
	for _, enum := range enums {
		zero, ok := enum.Zero()
		if !ok {
			continue
		}
		p.typing["Annotated"] = true
		p.symbols[topLevel(enum.Type.Name)] = true

		// The generated class subclasses int, so "at least 1" is the same set as
		// "not the zero member" — but only where no member is below zero.
		metadata := strconv.Quote("zero member " + zero.Name + " ruled out")
		if enum.AllAboveZero() {
			p.annotated["Ge"] = true
			metadata = "Ge(1)"
		}

		alias := pyAlias{
			name:     strings.ReplaceAll(enum.Type.Name, ".", "") + "Strict",
			typ:      enum.Type.Name,
			metadata: []string{metadata},
		}
		var doc []string
		if enum.Comments != "" {
			doc = append(doc, emit.OneLine(enum.Comments))
		}
		out = append(out, pyBlock{
			doc: append(doc,
				"Without "+zero.Name+", the member protobuf numbers 0, which this overlay",
				"reads as \"unset\" rather than a value. No buf.validate rule says so; the",
				"convention that names it <ENUM>_UNSPECIFIED does.",
			),
			aliases: []pyAlias{alias},
		})
		p.enumStrict["enum:"+enum.Name] = alias.name
	}
	return out
}
