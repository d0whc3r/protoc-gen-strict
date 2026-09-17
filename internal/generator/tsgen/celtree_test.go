package tsgen

import (
	"testing"

	"github.com/d0whc3r/protoc-gen-strict/internal/parser"
)

// TestCELPathIntoForeignMessage covers a message CEL rule whose path reaches
// into a message from a file this run was not asked to generate. The path
// resolves against the descriptors, but there is no overlay to narrow, so the
// conjunct has to be reported rather than placed. Placing it used to emit an
// import of a module that does not exist.
func TestCELPathIntoForeignMessage(t *testing.T) {
	const source = "has(this.created_at.seconds)"
	msg := parser.MessageMetadata{
		Name: "example.v1.Event",
		Fields: []parser.FieldMetadata{
			{Name: "created_at", ProtoType: "message:google.protobuf.Timestamp"},
		},
		CEL: []parser.CELRule{{
			ID: "event.created",
			Terms: []parser.CELTerm{{
				Kind:   "present",
				Source: source,
				Path:   []string{"created_at", "seconds"},
			}},
		}},
	}

	c := &Context{messages: map[string]parser.MessageMetadata{msg.Name: msg}}
	tree, notes := c.buildCELTree(msg)

	if !tree.empty() {
		t.Error("placed a narrowing reaching into a message this run never parsed")
	}
	if len(notes) != 1 {
		t.Fatalf("notes = %+v, want exactly one", notes)
	}
	if notes[0].Carried {
		t.Error("note claims the rule was carried into the type")
	}
	if got := notes[0].Skipped; len(got) != 1 || got[0] != source {
		t.Errorf("Skipped = %v, want [%q]", got, source)
	}
}

// TestCELTermOnOwnField guards the pruning above from dropping the ordinary
// case: a term on one of the message's own fields needs no import at all.
func TestCELTermOnOwnField(t *testing.T) {
	msg := parser.MessageMetadata{
		Name:   "example.v1.User",
		Fields: []parser.FieldMetadata{{Name: "id", ProtoType: "string"}},
		CEL: []parser.CELRule{{
			ID: "user.id",
			Terms: []parser.CELTerm{{
				Kind:   parser.TermEmpty,
				Source: `this.id == ""`,
				Path:   []string{"id"},
			}},
		}},
	}

	c := &Context{messages: map[string]parser.MessageMetadata{msg.Name: msg}}
	tree, notes := c.buildCELTree(msg)

	if got := tree.children["id"]; got == nil || got.term != parser.TermEmpty {
		t.Errorf("tree.children[\"id\"] = %+v, want an empty term", got)
	}
	if len(notes) != 1 || !notes[0].Carried || len(notes[0].Skipped) != 0 {
		t.Errorf("notes = %+v, want one carried note with nothing skipped", notes)
	}
}

// TestCELPresenceAndValueBothKept covers a field two conjuncts of one rule both
// name. Presence and the value bound have separate slots: a single one would
// keep the last and drop the other while still reporting the rule as carried.
func TestCELPresenceAndValueBothKept(t *testing.T) {
	for _, order := range [][]parser.CELTerm{
		{{Kind: parser.TermPresent, Path: []string{"label"}}, {Kind: parser.TermEmpty, Path: []string{"label"}}},
		{{Kind: parser.TermEmpty, Path: []string{"label"}}, {Kind: parser.TermPresent, Path: []string{"label"}}},
	} {
		msg := parser.MessageMetadata{
			Name:   "example.v1.User",
			Fields: []parser.FieldMetadata{{Name: "label", ProtoType: "string", Optional: true}},
			CEL:    []parser.CELRule{{ID: "user.label", Terms: order}},
		}

		c := &Context{messages: map[string]parser.MessageMetadata{msg.Name: msg}}
		tree, notes := c.buildCELTree(msg)

		node := tree.children["label"]
		if node == nil || !node.present || node.term != parser.TermEmpty {
			t.Errorf("%v: node = %+v, want both present and an empty term", order, node)
		}
		if len(notes[0].Skipped) != 0 {
			t.Errorf("%v: skipped %v, want nothing dropped", order, notes[0].Skipped)
		}
	}
}

// TestCELContradictoryTermsReported covers two value terms on one path. The
// second cannot replace the first silently: the rule would then claim to carry
// a constraint the type never gained.
func TestCELContradictoryTermsReported(t *testing.T) {
	msg := parser.MessageMetadata{
		Name:   "example.v1.User",
		Fields: []parser.FieldMetadata{{Name: "slug", ProtoType: "string", Optional: true}},
		CEL: []parser.CELRule{{
			ID: "user.slug",
			Terms: []parser.CELTerm{
				{Kind: parser.TermAbsent, Path: []string{"slug"}, Source: `!has(this.slug)`},
				{Kind: parser.TermEmpty, Path: []string{"slug"}, Source: `this.slug == ""`},
			},
		}},
	}

	c := &Context{messages: map[string]parser.MessageMetadata{msg.Name: msg}}
	tree, notes := c.buildCELTree(msg)

	if got := tree.children["slug"].term; got != parser.TermAbsent {
		t.Errorf("term = %q, want the first one kept", got)
	}
	if got := notes[0].Skipped; len(got) != 1 || got[0] != `this.slug == ""` {
		t.Errorf("Skipped = %v, want the second term reported", got)
	}
}

// TestCELPathIntoOneofMember covers a path naming a oneof member. protoc-gen-es
// declares the oneof as one discriminated-union property, so its members are not
// properties: placing the term would emit a Require over a name the generated
// type does not have, which TypeScript rejects outright.
func TestCELPathIntoOneofMember(t *testing.T) {
	const source = "has(this.email)"
	msg := parser.MessageMetadata{
		Name: "example.v1.Contact",
		Fields: []parser.FieldMetadata{
			{Name: "email", ProtoType: "string", OneofName: "channel"},
		},
		Oneofs: []parser.OneofMetadata{{Name: "channel", Fields: []string{"email"}}},
		CEL: []parser.CELRule{{
			ID:    "contact.email",
			Terms: []parser.CELTerm{{Kind: parser.TermPresent, Path: []string{"email"}, Source: source}},
		}},
	}

	c := &Context{messages: map[string]parser.MessageMetadata{msg.Name: msg}}
	tree, notes := c.buildCELTree(msg)

	if !tree.empty() {
		t.Error("placed a narrowing on a oneof member")
	}
	if notes[0].Carried {
		t.Error("note claims the rule was carried into the type")
	}
	if got := notes[0].Skipped; len(got) != 1 || got[0] != source {
		t.Errorf("Skipped = %v, want [%q]", got, source)
	}
}

// TestCELPathToUnknownField covers a path whose last segment names no field at
// all. It used to pass placeable and be dropped silently by the emitter, which
// reported the rule as carried and then narrowed nothing.
func TestCELPathToUnknownField(t *testing.T) {
	msg := parser.MessageMetadata{
		Name:   "example.v1.User",
		Fields: []parser.FieldMetadata{{Name: "id", ProtoType: "string"}},
		CEL: []parser.CELRule{{
			ID:    "user.missing",
			Terms: []parser.CELTerm{{Kind: parser.TermNonEmpty, Path: []string{"nickname"}, Source: `this.nickname != ""`}},
		}},
	}

	c := &Context{messages: map[string]parser.MessageMetadata{msg.Name: msg}}
	if _, notes := c.buildCELTree(msg); notes[0].Carried || len(notes[0].Skipped) != 1 {
		t.Errorf("notes = %+v, want the term reported rather than carried", notes)
	}
}
