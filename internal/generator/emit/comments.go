// Package emit holds what every target's generator needs and none of them owns:
// the wording of a rule comment, the lookup of a rule on a field, and the
// identifier casing the official generators chose. Nothing here knows a target
// language; a symbol only one of them uses belongs in that target's package.
package emit

import (
	"cmp"
	"fmt"
	"strings"

	"github.com/d0whc3r/protoc-gen-strict/internal/parser"
)

// Wording every overlay prints, so a rule with no type equivalent stays
// visible; only the shape around it differs (a JSDoc block, a comment, an
// Annotated entry).
const (
	CarriedPrefix     = "Carried into the type: "
	UnparseablePrefix = "UNPARSEABLE: "
)

// RuntimeNote is one entry of the "left to runtime validation" list.
type RuntimeNote struct {
	Subject string
	Lines   []string
}

// MessageComments renders what belongs to no single field: cross-field CEL
// rules and oneof exclusivity.
func MessageComments(msg parser.MessageMetadata) []string {
	var lines []string
	for _, oneof := range msg.Oneofs {
		lines = append(lines, OneofLine(oneof))
	}
	return append(lines, celComments(msg.CEL)...)
}

// OneofLine describes one exclusivity constraint, real oneof or not; the
// `(buf.validate.message).oneof` rule has no name.
func OneofLine(oneof parser.OneofMetadata) string {
	label := "oneof"
	if oneof.Name != "" {
		label = "oneof " + oneof.Name
	}
	if oneof.Required {
		label = "required " + label
	}
	return fmt.Sprintf("%s: exactly one of %s", label, strings.Join(oneof.Fields, ", "))
}

// RuleComments renders a field's constraints as plain comment lines.
func RuleComments(field parser.FieldMetadata) []string {
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
		label := CELLabel(rule)
		if rule.ParseError != "" {
			lines = append(lines, "cel["+label+"] "+UnparseablePrefix+rule.ParseError)
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

// CELLabel names a rule in the generated doc; the cel_expression shorthand has
// no id of its own.
func CELLabel(rule parser.CELRule) string { return cmp.Or(rule.ID, "cel") }

// OneLine collapses a proto comment's whitespace, so it fits on one line.
func OneLine(comment string) string {
	return strings.Join(strings.Fields(comment), " ")
}

// CommentLines splits every entry at its newlines, so one doc line is one line
// of output. A CEL expression spans several lines as often as not, and cel-go
// prints a parse error with the offending source under it — emitted whole, the
// second line would leave the comment and land in the generated source as code.
func CommentLines(lines []string) []string {
	out := make([]string, 0, len(lines))
	for _, line := range lines {
		for _, physical := range strings.Split(line, "\n") {
			out = append(out, strings.TrimSuffix(physical, "\r"))
		}
	}
	return out
}
