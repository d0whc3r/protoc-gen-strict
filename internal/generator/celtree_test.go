package generator

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
			{Name: "created_at", JSONName: "createdAt", ProtoType: "message:google.protobuf.Timestamp"},
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
		Fields: []parser.FieldMetadata{{Name: "id", JSONName: "id", ProtoType: "string"}},
		CEL: []parser.CELRule{{
			ID: "user.id",
			Terms: []parser.CELTerm{{
				Kind:   "nonEmpty",
				Source: `this.id != ""`,
				Path:   []string{"id"},
			}},
		}},
	}

	c := &Context{messages: map[string]parser.MessageMetadata{msg.Name: msg}}
	tree, notes := c.buildCELTree(msg)

	if got := tree.children["id"]; got == nil || got.term != "nonEmpty" {
		t.Errorf("tree.children[\"id\"] = %+v, want a nonEmpty term", got)
	}
	if len(notes) != 1 || !notes[0].Carried || len(notes[0].Skipped) != 0 {
		t.Errorf("notes = %+v, want one carried note with nothing skipped", notes)
	}
}
