package generator

import (
	"fmt"
	"strings"

	"github.com/d0whc3r/protoc-gen-strict/internal/parser"
)

// The rule comments every overlay carries: a rule with no type equivalent still
// has to be visible. Both generators render the same lines, in different
// places.

// Wording both overlays print; only the shape around it differs (a JSDoc block,
// a comment, an Annotated entry).
const (
	carriedPrefix     = "Carried into the type: "
	unparseablePrefix = "UNPARSEABLE: "
)

// runtimeNote is one entry of the "left to runtime validation" list.
type runtimeNote struct {
	subject string
	lines   []string
}

// messageComments renders what belongs to no single field: cross-field CEL
// rules and oneof exclusivity.
func messageComments(msg parser.MessageMetadata) []string {
	var lines []string
	for _, oneof := range msg.Oneofs {
		lines = append(lines, oneofLine(oneof))
	}
	return append(lines, celComments(msg.CEL)...)
}

// oneofLine describes one exclusivity constraint, real oneof or not; the
// `(buf.validate.message).oneof` rule has no name.
func oneofLine(oneof parser.OneofMetadata) string {
	label := "oneof"
	if oneof.Name != "" {
		label = "oneof " + oneof.Name
	}
	if oneof.Required {
		label = "required " + label
	}
	return fmt.Sprintf("%s: exactly one of %s", label, strings.Join(oneof.Fields, ", "))
}

// ruleComments renders a field's constraints as plain comment lines.
func ruleComments(field parser.FieldMetadata) []string {
	var lines []string
	if field.Required {
		lines = append(lines, "required")
	}
	for _, rule := range field.Rules {
		lines = append(lines, fmt.Sprintf("%s = %s", rule.Kind, rule.Value))
	}
	return append(lines, celComments(field.CEL)...)
}

// celComments renders CEL rules, field- and message-level, with what the parsed
// AST says they reference and call.
func celComments(rules []parser.CELRule) []string {
	var lines []string
	for _, rule := range rules {
		label := celLabel(rule)
		if rule.ParseError != "" {
			lines = append(lines, "cel["+label+"] "+unparseablePrefix+rule.ParseError)
			continue
		}
		lines = append(lines, fmt.Sprintf("cel[%s]: %s", label, rule.Expression))
		if rule.Message != "" {
			lines = append(lines, "  message: "+rule.Message)
		}
		if len(rule.Idents) > 0 {
			lines = append(lines, "  refs: "+strings.Join(rule.Idents, ", "))
		}
		if len(rule.Functions) > 0 {
			lines = append(lines, "  calls: "+strings.Join(rule.Functions, ", "))
		}
	}
	return lines
}

func oneLine(comment string) string {
	return strings.Join(strings.Fields(comment), " ")
}
