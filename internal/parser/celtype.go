// Translating a message-level CEL rule into the narrowing it implies.
//
// Only conjunctions of a few shapes, on paths rooted at `this`, say something a
// type can carry. The rest is reported verbatim as left to runtime validation,
// so a rule that stops being translated shows up in the generated diff.

package parser

import (
	"slices"

	celast "github.com/google/cel-go/common/ast"
	"github.com/google/cel-go/common/operators"
	"github.com/google/cel-go/common/types/ref"
	celparser "github.com/google/cel-go/parser"
	"google.golang.org/protobuf/reflect/protoreflect"
)

// TermKind is the narrowing a conjunct implies, and what the generators switch
// on.
type TermKind string

const (
	TermEmpty    TermKind = "empty"    // the empty string
	TermNonEmpty TermKind = "nonEmpty" // not the empty string
	TermAbsent   TermKind = "absent"   // not set
	TermPresent  TermKind = "present"  // set; the optionality changes, the type does not
	TermZero     TermKind = "zero"     // an enum's zero-numbered member
)

// CELTerm is one conjunct with a type equivalent, resolved against the
// descriptors to the field it constrains.
type CELTerm struct {
	Path   []string // proto field names from the message root, e.g. ["detail", "uuid"]
	Kind   TermKind
	Source string // the conjunct unparsed back to CEL, for when it cannot be placed
}

// translateCEL splits at every `&&` and matches each conjunct. The second
// result holds the ones with no type equivalent, unparsed back to CEL.
func translateCEL(root protoreflect.MessageDescriptor, expr celast.Expr, info *celast.SourceInfo) ([]CELTerm, []string) {
	if args, ok := callArgs(expr, operators.LogicalAnd, 2); ok {
		terms, skipped := translateCEL(root, args[0], info)
		rightTerms, rightSkipped := translateCEL(root, args[1], info)
		return append(terms, rightTerms...), append(skipped, rightSkipped...)
	}
	source, err := celparser.Unparse(expr, info)
	if err != nil {
		source = "<unprintable expression>"
	}
	if term, ok := matchTerm(root, expr); ok {
		term.Source = source
		return []CELTerm{term}, nil
	}
	return nil, []string{source}
}

// matchTerm recognises the shapes a type can carry.
func matchTerm(root protoreflect.MessageDescriptor, expr celast.Expr) (CELTerm, bool) {
	// `!has(this.x)`: a negated test-only select.
	if args, ok := callArgs(expr, operators.LogicalNot, 1); ok {
		if path, ok := testOnlyPath(root, args[0]); ok {
			return CELTerm{Path: path, Kind: TermAbsent}, true
		}
		return CELTerm{}, false
	}
	// `has(this.x)`.
	if path, ok := testOnlyPath(root, expr); ok {
		return CELTerm{Path: path, Kind: TermPresent}, true
	}

	// A comparison against a literal, in either argument order.
	for _, op := range []string{operators.Equals, operators.NotEquals} {
		args, ok := callArgs(expr, op, 2)
		if !ok {
			continue
		}
		path, leaf, literal, ok := comparison(root, args)
		if !ok {
			return CELTerm{}, false
		}
		switch value := literal.Value().(type) {
		case string:
			if value != "" {
				return CELTerm{}, false // only the empty string says something a type can carry
			}
			if op == operators.Equals {
				return CELTerm{Path: path, Kind: TermEmpty}, true
			}
			return CELTerm{Path: path, Kind: TermNonEmpty}, true
		case int64:
			// `== 0` narrows only an enum: elsewhere the zero is a number, a
			// bigint or a duration, and which one is protoc-gen-es's call.
			if value == 0 && op == operators.Equals && leaf.Kind() == protoreflect.EnumKind {
				return CELTerm{Path: path, Kind: TermZero}, true
			}
		}
		return CELTerm{}, false
	}
	return CELTerm{}, false
}

// comparison splits a two-argument call into path side and literal side, in
// either order.
func comparison(root protoreflect.MessageDescriptor, args []celast.Expr) ([]string, protoreflect.FieldDescriptor, ref.Val, bool) {
	for i, arg := range args {
		other := args[1-i]
		if other.Kind() != celast.LiteralKind {
			continue
		}
		path, leaf, ok := selectPath(root, arg)
		if !ok || leaf.IsList() || leaf.IsMap() {
			continue
		}
		return path, leaf, other.AsLiteral(), true
	}
	return nil, nil, nil, false
}

// testOnlyPath resolves the operand of `has(...)`, which cel-go parses into a
// select marked test-only.
func testOnlyPath(root protoreflect.MessageDescriptor, expr celast.Expr) ([]string, bool) {
	if expr.Kind() != celast.SelectKind || !expr.AsSelect().IsTestOnly() {
		return nil, false
	}
	path, _, ok := selectPath(root, expr)
	return path, ok
}

// selectPath resolves a chain of selects rooted at `this` against the message
// descriptors, and returns the field the last segment names. Every segment but
// the last must select into a singular message: through a list or a map the
// path describes elements, which this plugin does not narrow.
func selectPath(root protoreflect.MessageDescriptor, expr celast.Expr) ([]string, protoreflect.FieldDescriptor, bool) {
	// The walk runs outermost select first, so the names come out reversed.
	var names []string
	for expr.Kind() == celast.SelectKind {
		sel := expr.AsSelect()
		names = append(names, sel.FieldName())
		expr = sel.Operand()
	}
	if len(names) == 0 || expr.Kind() != celast.IdentKind || expr.AsIdent() != "this" {
		return nil, nil, false
	}
	slices.Reverse(names)

	current := root
	for i, name := range names {
		if current == nil {
			return nil, nil, false
		}
		field := current.Fields().ByName(protoreflect.Name(name))
		if field == nil {
			return nil, nil, false
		}
		if i == len(names)-1 {
			return names, field, true
		}
		if field.Kind() != protoreflect.MessageKind || field.IsList() || field.IsMap() {
			return nil, nil, false
		}
		current = field.Message()
	}
	return nil, nil, false
}

// callArgs returns expr's arguments when it calls function with want of them.
func callArgs(expr celast.Expr, function string, want int) ([]celast.Expr, bool) {
	if expr.Kind() != celast.CallKind {
		return nil, false
	}
	call := expr.AsCall()
	if call.FunctionName() != function || len(call.Args()) != want {
		return nil, false
	}
	return call.Args(), true
}
