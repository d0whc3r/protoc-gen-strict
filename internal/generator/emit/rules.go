package emit

import "github.com/d0whc3r/protoc-gen-strict/internal/parser"

// IgnoreRule is protovalidate's switch for when the sibling rules are evaluated
// at all. It sits next to them, both on a field and under `repeated.items`.
const IgnoreRule = "ignore"

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
