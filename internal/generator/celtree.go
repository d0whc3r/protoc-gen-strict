package generator

import (
	"cmp"
	"strings"

	"github.com/d0whc3r/protoc-gen-strict/internal/parser"
)

// celNode is what a message's CEL rules said about one field: a term on it, and
// what the paths reaching through it said about the message it carries. The
// root is the message itself and never holds a term.
type celNode struct {
	term     parser.TermKind // "" when no term landed on this field
	order    []string
	children map[string]*celNode
}

func newCELNode() *celNode { return &celNode{children: map[string]*celNode{}} }

func (n *celNode) child(name string) *celNode {
	if existing, ok := n.children[name]; ok {
		return existing
	}
	child := newCELNode()
	n.children[name] = child
	n.order = append(n.order, name)
	return child
}

func (n *celNode) empty() bool { return n == nil || (n.term == "" && len(n.children) == 0) }

// celNote records what one CEL rule did with the type, so a rule that stops
// being carried shows up in the generated diff.
type celNote struct {
	ID      string
	Message string
	Carried bool
	Skipped []string
}

// buildCELTree folds every message-level rule into one tree plus one note each.
//
// ponytail: two terms on one path (`this.x == "" && this.x != ""`, itself
// unsatisfiable) leave only the last. Report a conflict if a schema writes one.
func (c *Context) buildCELTree(msg parser.MessageMetadata) (*celNode, []celNote) {
	root := newCELNode()
	notes := make([]celNote, 0, len(msg.CEL))
	for _, rule := range msg.CEL {
		note := celNote{ID: celLabel(rule), Message: rule.Message}
		if rule.ParseError != "" {
			note.Skipped = []string{unparseablePrefix + rule.ParseError}
			notes = append(notes, note)
			continue
		}
		note.Skipped = rule.Skipped
		for _, term := range rule.Terms {
			if !c.placeable(msg, term.Path) {
				note.Skipped = append(note.Skipped, term.Source)
				continue
			}
			node := root
			for _, step := range term.Path {
				node = node.child(step)
			}
			node.term = term.Kind
			note.Carried = true
		}
		notes = append(notes, note)
	}
	return root, notes
}

// placeable reports whether every message a path reaches through is one this
// run parsed. `has(this.created_at.seconds)` resolves against Timestamp's
// descriptors, but nobody asked to generate that file, so there is no module to
// import a narrowed type from; the conjunct goes to runtime validation.
func (c *Context) placeable(msg parser.MessageMetadata, path []string) bool {
	for _, step := range path[:max(len(path)-1, 0)] {
		field, ok := fieldByName(msg, step)
		if !ok {
			return false
		}
		target, ok := messageTarget(field)
		if !ok {
			return false
		}
		if msg, ok = c.messages[target]; !ok {
			return false
		}
	}
	return true
}

func fieldByName(msg parser.MessageMetadata, name string) (parser.FieldMetadata, bool) {
	for _, field := range msg.Fields {
		if field.Name == name {
			return field, true
		}
	}
	return parser.FieldMetadata{}, false
}

// celLabel names a rule in the generated doc; the cel_expression shorthand has
// no id of its own.
func celLabel(rule parser.CELRule) string { return cmp.Or(rule.ID, "cel") }

// celRuntimeNotes renders the conjuncts no type could carry.
func celRuntimeNotes(notes []celNote) []runtimeNote {
	var out []runtimeNote
	for _, note := range notes {
		if len(note.Skipped) == 0 {
			continue
		}
		grouped := runtimeNote{subject: "cel[" + note.ID + "]"}
		grouped.lines = append(grouped.lines, note.Skipped...)
		if note.Message != "" {
			grouped.lines = append(grouped.lines, "message: "+note.Message)
		}
		out = append(out, grouped)
	}
	return out
}

// celCarried names the rules that did become part of the type.
func celCarried(notes []celNote) string {
	var ids []string
	for _, note := range notes {
		if note.Carried {
			ids = append(ids, "cel["+note.ID+"]")
		}
	}
	if len(ids) == 0 {
		return ""
	}
	return carriedPrefix + strings.Join(ids, ", ") + "."
}
