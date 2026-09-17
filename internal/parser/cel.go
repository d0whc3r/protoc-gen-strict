package parser

import (
	"fmt"
	"maps"
	"slices"
	"strings"
	"sync"

	validate "buf.build/gen/go/bufbuild/protovalidate/protocolbuffers/go/buf/validate"
	"github.com/google/cel-go/cel"
	celast "github.com/google/cel-go/common/ast"
	"google.golang.org/protobuf/reflect/protoreflect"
)

// CELRule is a custom CEL constraint declared via `(buf.validate.field).cel`
// or the shorthand `(buf.validate.field).cel_expression`.
type CELRule struct {
	ID         string   // rule id, empty for the cel_expression shorthand
	Message    string   // human readable failure message, may be empty
	Expression string   // raw CEL source
	Idents     []string // identifiers referenced by the expression (e.g. "this")
	Functions  []string // functions/operators called (e.g. "startsWith", "_>_")
	ParseError string   // non-empty when the expression failed to parse

	// Message-level rules only: the conjuncts a type can carry, and the source
	// of the ones it cannot. See celtype.go.
	Terms   []CELTerm
	Skipped []string
}

// celEnv is built once. Parsing needs no declarations; `this` is declared to
// keep the environment usable for a future type-check pass.
var celEnv = sync.OnceValues(func() (*cel.Env, error) {
	return cel.NewEnv(cel.Variable("this", cel.DynType))
})

func celRules(rules *validate.FieldRules) ([]CELRule, error) {
	// A field rule roots `this` at the field, not at a message, so there is no
	// path to resolve and nothing to translate.
	return celRulesFrom(nil, rules.GetCel(), rules.GetCelExpression())
}

// celRulesFrom parses both forms: the full `cel` rule, and the `cel_expression`
// shorthand, whose id is its own text. A non-nil root is the message `this`
// refers to, and turns on the translation into narrowing terms.
func celRulesFrom(root protoreflect.MessageDescriptor, full []*validate.Rule, shorthand []string) ([]CELRule, error) {
	var out []CELRule
	add := func(id, message, expression string) error {
		parsed, ast, err := parseCEL(id, message, expression)
		if err != nil {
			return err
		}
		if root != nil && ast != nil {
			rep := ast.NativeRep()
			parsed.Terms, parsed.Skipped = translateCEL(root, rep.Expr(), rep.SourceInfo())
		}
		out = append(out, parsed)
		return nil
	}
	for _, r := range full {
		if err := add(r.GetId(), r.GetMessage(), r.GetExpression()); err != nil {
			return nil, err
		}
	}
	for _, expr := range shorthand {
		if err := add(expr, "", expr); err != nil {
			return nil, err
		}
	}
	return out, nil
}

// parseCEL walks the AST to report which identifiers the expression reads and
// which functions it calls, handing the AST back so a message-level rule is not
// parsed twice. A syntax error lands in ParseError rather than aborting the
// whole file.
func parseCEL(id, message, expression string) (CELRule, *cel.Ast, error) {
	rule := CELRule{ID: id, Message: message, Expression: expression}

	env, err := celEnv()
	if err != nil {
		return rule, nil, fmt.Errorf("building CEL environment: %w", err)
	}

	ast, issues := env.Parse(expression)
	if issues != nil && issues.Err() != nil {
		rule.ParseError = issues.Err().Error()
		return rule, nil, nil
	}

	idents := map[string]struct{}{}
	funcs := map[string]struct{}{}
	celast.PostOrderVisit(ast.NativeRep().Expr(), celast.NewExprVisitor(func(e celast.Expr) {
		switch e.Kind() {
		case celast.IdentKind:
			addName(idents, e.AsIdent())
		case celast.SelectKind:
			addName(idents, e.AsSelect().FieldName())
		case celast.CallKind:
			addName(funcs, e.AsCall().FunctionName())
		}
	}))
	rule.Idents = slices.Sorted(maps.Keys(idents))
	rule.Functions = slices.Sorted(maps.Keys(funcs))
	return rule, ast, nil
}

// addName records a name unless it is a macro internal: all() and exists()
// expand into comprehensions carrying accumulators like "@result", which the
// schema author never wrote.
func addName(set map[string]struct{}, name string) {
	if name == "" || strings.HasPrefix(name, "@") {
		return
	}
	set[name] = struct{}{}
}
