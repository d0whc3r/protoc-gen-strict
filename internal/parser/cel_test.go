package parser

import (
	"testing"

	validate "buf.build/gen/go/bufbuild/protovalidate/protocolbuffers/go/buf/validate"
)

// TestParseCEL covers the AST walk: identifiers, receiver-call function names,
// and that a syntax error is reported instead of returned.
func TestParseCEL(t *testing.T) {
	rule, _, err := parseCEL("user.id.prefix", "must be prefixed", "this.startsWith('usr_')")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if rule.ParseError != "" {
		t.Fatalf("unexpected parse error: %s", rule.ParseError)
	}
	if got := rule.Idents; len(got) != 1 || got[0] != "this" {
		t.Errorf("Idents = %v, want [this]", got)
	}
	if got := rule.Functions; len(got) != 1 || got[0] != "startsWith" {
		t.Errorf("Functions = %v, want [startsWith]", got)
	}

	bad, _, err := parseCEL("", "", "this.startsWith(")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if bad.ParseError == "" {
		t.Error("expected ParseError for malformed expression")
	}
}

// TestFieldRulesNilSafety asserts the extension reader tolerates nil options
// rather than panicking.
func TestFieldRulesNilSafety(t *testing.T) {
	if got := standardRules(nil); got != nil {
		t.Errorf("standardRules(nil) = %v, want nil", got)
	}
	var rules *validate.FieldRules
	got, err := celRulesFrom(nil, rules.GetCel(), rules.GetCelExpression())
	if err != nil || got != nil {
		t.Errorf("celRulesFrom(nil rules) = %v, %v; want nil, nil", got, err)
	}
}
