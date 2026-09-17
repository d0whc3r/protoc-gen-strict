package oapigen

import (
	"slices"
	"testing"

	"github.com/d0whc3r/protoc-gen-strict/internal/parser"
)

// TestZeroBoundNeverHalfWritten covers the bounds grpc-gateway drops. Its
// swagger writer tags `minimum` and `maximum` `omitempty`, so a zero never
// reaches the output. A whole-number bound steps around it — `gt: 0` is
// `minimum: 1` — and a fractional one is left to runtime validation. Emitting
// the pair used to leave an `exclusiveMinimum: true` with no `minimum`.
func TestZeroBoundNeverHalfWritten(t *testing.T) {
	tests := []struct {
		name string
		rule parser.Rule
		want []keyword
	}{
		{"int gt zero", parser.Rule{Kind: "int32.gt", Value: "0"}, []keyword{{"minimum", "1"}}},
		{"int lt zero", parser.Rule{Kind: "sfixed32.lt", Value: "0"}, []keyword{{"maximum", "-1"}}},
		{"int gt nonzero", parser.Rule{Kind: "int32.gt", Value: "10"}, []keyword{{"minimum", "11"}}},
		{"int gte zero", parser.Rule{Kind: "int32.gte", Value: "0"}, nil},
		{"int lte zero", parser.Rule{Kind: "uint32.lte", Value: "0"}, nil},
		{"int gte nonzero", parser.Rule{Kind: "int32.gte", Value: "-128"}, []keyword{{"minimum", "-128"}}},
		{"float gt zero", parser.Rule{Kind: "float.gt", Value: "0"}, nil},
		{"float gt nonzero", parser.Rule{Kind: "double.gt", Value: "1.5"}, []keyword{{"minimum", "1.5"}, {"exclusiveMinimum", "true"}}},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := ruleKeywords(tt.rule)
			if len(got) != len(tt.want) {
				t.Fatalf("ruleKeywords(%v) = %v, want %v", tt.rule, got, tt.want)
			}
			for i, kw := range got {
				if kw != tt.want[i] {
					t.Errorf("ruleKeywords(%v)[%d] = %v, want %v", tt.rule, i, kw, tt.want[i])
				}
			}
		})
	}
}

// TestRequiredIsNotCarried covers the rule that has no safe place in a swagger.
// grpc-gateway hoists a field's `required` into the parent message, which also
// marks the flattened query parameter of a GET required — true even when the
// sub-message holding the field was never sent.
func TestRequiredIsNotCarried(t *testing.T) {
	field := parser.FieldMetadata{Name: "currency", ProtoType: "enum:shop.common.v1.Currency", Required: true}

	if got := fieldKeywords(field); got != nil {
		t.Errorf("fieldKeywords(required field) = %v, want none", got)
	}
}

// TestReversedRangeNotCarried covers protovalidate's reversed range. A lower
// bound above the upper one admits everything outside it, not the empty set, and
// a JSONSchema minimum/maximum pair can only say "and" — emitted as written it
// would reject every value the proto allows.
func TestReversedRangeNotCarried(t *testing.T) {
	reversed := parser.FieldMetadata{Name: "outside", ProtoType: "int32", Rules: []parser.Rule{
		{Kind: "int32.gt", Value: "20"},
		{Kind: "int32.lt", Value: "10"},
	}}
	if got := fieldKeywords(reversed); got != nil {
		t.Errorf("fieldKeywords(reversed range) = %v, want none", got)
	}

	ordinary := parser.FieldMetadata{Name: "inside", ProtoType: "int32", Rules: []parser.Rule{
		{Kind: "int32.gt", Value: "10"},
		{Kind: "int32.lt", Value: "20"},
	}}
	want := []keyword{{"minimum", "11"}, {"maximum", "19"}}
	if got := fieldKeywords(ordinary); !slices.Equal(got, want) {
		t.Errorf("fieldKeywords(ordinary range) = %v, want %v", got, want)
	}
}

// TestIgnoredItemsNotCarried covers `repeated.items.ignore`, which is separate
// from the field's own: the elements are not always validated, so their keywords
// describe nothing, while the list's own min_items still holds.
func TestIgnoredItemsNotCarried(t *testing.T) {
	field := parser.FieldMetadata{Name: "tags", ProtoType: "string", Repeated: true, Rules: []parser.Rule{
		{Kind: "repeated.items.ignore", Value: "IGNORE_IF_ZERO_VALUE"},
		{Kind: "repeated.items.string.min_len", Value: "3"},
		{Kind: "repeated.min_items", Value: "1"},
	}}

	want := []keyword{{"minItems", "1"}}
	if got := fieldKeywords(field); !slices.Equal(got, want) {
		t.Errorf("fieldKeywords() = %v, want %v", got, want)
	}
}
