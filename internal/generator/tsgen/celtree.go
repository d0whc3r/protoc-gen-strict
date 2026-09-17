package tsgen

import (
	"strings"

	"github.com/d0whc3r/protoc-gen-strict/internal/generator/emit"
	"github.com/d0whc3r/protoc-gen-strict/internal/parser"
)

// celNode is what a message's CEL rules said about one field: whether it has to
// be present, a term narrowing its value, and what the paths reaching through it
// said about the message it carries. The root is the message itself and holds
// neither.
//
// Presence and value are separate because one field takes both: with
// `has(this.label) && this.label != ""` a single slot would keep the last and
// drop the other while still reporting the whole rule as carried.
type celNode struct {
	present  bool            // a conjunct requires the field to be set
	term     parser.TermKind // "" when no value term landed on this field
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

func (n *celNode) empty() bool {
	return n == nil || (!n.present && n.term == "" && len(n.children) == 0)
}

// celNote records what one CEL rule did with the type, so a rule that stops
// being carried shows up in the generated diff.
type celNote struct {
	ID      string
	Message string
	Carried bool
	Skipped []string
}

// buildCELTree folds every message-level rule into one tree plus one note each.
func (c *Context) buildCELTree(msg parser.MessageMetadata) (*celNode, []celNote) {
	root := newCELNode()
	notes := make([]celNote, 0, len(msg.CEL))
	for _, rule := range msg.CEL {
		note := celNote{ID: emit.CELLabel(rule), Message: rule.Message}
		if rule.ParseError != "" {
			note.Skipped = []string{emit.UnparseablePrefix + rule.ParseError}
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
			if !node.place(term.Kind) {
				// Two value terms on one path, e.g. `this.x == "" &&
				// this.x != ""`. The rule contradicts itself; report the
				// second rather than letting it replace the first.
				note.Skipped = append(note.Skipped, term.Source)
				continue
			}
			note.Carried = true
		}
		notes = append(notes, note)
	}
	return root, notes
}

// place records a term on the node, reporting false when a second value term
// wants the same slot. Presence has a slot of its own, so `has(this.x)` and a
// term narrowing x's value both land.
func (n *celNode) place(kind parser.TermKind) bool {
	if kind == parser.TermPresent {
		n.present = true
		return true
	}
	if n.term != "" && n.term != kind {
		return false
	}
	n.term = kind
	return true
}

// placeable reports whether a path names fields the overlay can narrow.
//
// Every message it reaches through has to be one this run parsed:
// `has(this.created_at.seconds)` resolves against Timestamp's descriptors, but
// nobody asked to generate that file, so there is no module to import a narrowed
// type from. No segment may be a oneof member either: protoc-gen-es models a
// oneof as one discriminated-union property, so its members are not properties
// to narrow. Either way the conjunct goes to runtime validation.
func (c *Context) placeable(msg parser.MessageMetadata, path []string) bool {
	for i, step := range path {
		field, ok := fieldByName(msg, step)
		if !ok || field.OneofName != "" {
			return false
		}
		if i == len(path)-1 {
			return true
		}
		target, ok := messageTarget(field)
		if !ok {
			return false
		}
		if msg, ok = c.messages[target]; !ok {
			return false
		}
	}
	return false
}

func fieldByName(msg parser.MessageMetadata, name string) (parser.FieldMetadata, bool) {
	for _, field := range msg.Fields {
		if field.Name == name {
			return field, true
		}
	}
	return parser.FieldMetadata{}, false
}

// celRuntimeNotes renders the conjuncts no type could carry.
func celRuntimeNotes(notes []celNote) []emit.RuntimeNote {
	var out []emit.RuntimeNote
	for _, note := range notes {
		if len(note.Skipped) == 0 {
			continue
		}
		grouped := emit.RuntimeNote{Subject: "cel[" + note.ID + "]"}
		grouped.Lines = append(grouped.Lines, note.Skipped...)
		if note.Message != "" {
			grouped.Lines = append(grouped.Lines, "message: "+note.Message)
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
	return emit.CarriedPrefix + strings.Join(ids, ", ") + "."
}
