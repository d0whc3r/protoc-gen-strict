package parser

import (
	"os"
	"strings"
	"testing"

	"google.golang.org/protobuf/proto"
	"google.golang.org/protobuf/reflect/protodesc"
	"google.golang.org/protobuf/reflect/protoreflect"
	"google.golang.org/protobuf/types/descriptorpb"
)

// coverageMessage is the fixture the translation is exercised against: a
// string, an enum, a presence-tracking field, a message and a list.
const coverageMessage = "shop.coverage.v1.CelNarrowingCoverage"

func TestTranslateCEL(t *testing.T) {
	root := loadMessage(t, coverageMessage)

	tests := []struct {
		expression string
		want       string // "<path>:<kind>", or "" when nothing should be carried
	}{
		{"this.label != ''", "label:nonEmpty"},
		{"this.slug == ''", "slug:empty"},
		{"'' == this.slug", "slug:empty"},   // both argument orders
		{"this.flavor == 0", "flavor:zero"}, // an enum zero is a member
		{"!has(this.note)", "note:absent"},
		{"has(this.detail)", "detail:present"},
		{"this.detail.uuid != ''", "detail.uuid:nonEmpty"},

		// Shapes that must not match: translation may narrow, never widen.
		{"this.label == 'x'", ""}, // only the empty string says something
		{"this.slug != 'x'", ""},
		{"this.label != '' || this.slug == ''", ""}, // a disjunction is not a conjunction
		{"!(this.label == '')", ""},                 // negation only reads a has() test
		{"this.label == this.slug", ""},             // no literal side
		{"has(this.label) == has(this.slug)", ""},
		{"this.missing == ''", ""}, // not a field of this message
		{"this.tags == ''", ""},    // not a field of this message either
	}

	for _, test := range tests {
		t.Run(test.expression, func(t *testing.T) {
			terms, skipped := translate(t, root, test.expression)
			if test.want == "" {
				if len(terms) != 0 {
					t.Fatalf("expected nothing carried, got %v", terms)
				}
				if len(skipped) == 0 {
					t.Fatal("a conjunct that is not carried must be reported")
				}
				return
			}
			if len(terms) != 1 {
				t.Fatalf("want 1 term, got %d (skipped %v)", len(terms), skipped)
			}
			if got := termKey(terms[0]); got != test.want {
				t.Errorf("got %q, want %q", got, test.want)
			}
		})
	}
}

// TestTranslateCELConjunction checks a mixed rule keeps both halves: the
// carried conjuncts become terms, the rest are reported one by one rather than
// sinking the whole rule.
func TestTranslateCELConjunction(t *testing.T) {
	root := loadMessage(t, coverageMessage)
	terms, skipped := translate(t, root, "this.label != '' && this.label == this.slug && !has(this.note)")

	want := []string{"label:nonEmpty", "note:absent"}
	if len(terms) != len(want) {
		t.Fatalf("want %d terms, got %d", len(want), len(terms))
	}
	for i, term := range terms {
		if got := termKey(term); got != want[i] {
			t.Errorf("term %d: got %q, want %q", i, got, want[i])
		}
	}
	if len(skipped) != 1 || skipped[0] != "this.label == this.slug" {
		t.Errorf("skipped conjuncts: got %v, want [this.label == this.slug]", skipped)
	}
}

// TestTranslateCELRepeatedPath pins the order two terms on one path come back
// in. They contradict each other, and the generator keeps the first and reports
// the second, which only means something if the order is the source order.
func TestTranslateCELRepeatedPath(t *testing.T) {
	root := loadMessage(t, coverageMessage)
	terms, skipped := translate(t, root, "this.slug == '' && this.slug != ''")

	want := []string{"slug:empty", "slug:nonEmpty"}
	if len(terms) != len(want) {
		t.Fatalf("want %d terms, got %d (skipped %v)", len(want), len(terms), skipped)
	}
	for i, term := range terms {
		if got := termKey(term); got != want[i] {
			t.Errorf("term %d: got %q, want %q", i, got, want[i])
		}
	}
}

// TestTranslateCELPresence covers what `has()` means per field.
//
// CEL only reads it as "was it set" where the field tracks presence. On a plain
// proto3 scalar it is "not the default value", and translating it as presence
// would narrow a property protoc-gen-es declares required down to `never`.
func TestTranslateCELPresence(t *testing.T) {
	root := loadMessage(t, "shop.coverage.v1.AmbiguityCoverage")

	tests := []struct {
		expression string
		want       string // "<path>:<kind>", or "" when nothing should be carried
	}{
		{"has(this.nickname)", "nickname:present"},                 // explicit proto3 optional
		{"!has(this.nickname)", "nickname:absent"},                 //
		{"has(this.unchecked_detail)", "unchecked_detail:present"}, // a message
		{"has(this.label)", "label:nonEmpty"},                      // implicit presence: a value test
		{"!has(this.label)", "label:empty"},                        //
		{"has(this.quantity)", ""},                                 // no type rules out a non-zero number
		{"!has(this.quantity)", ""},                                // nor pins it to zero
		{"has(this.optional_tags)", ""},                            // a list compares against empty
		{"!has(this.optional_tags)", ""},                           //
	}

	for _, test := range tests {
		t.Run(test.expression, func(t *testing.T) {
			terms, skipped := translate(t, root, test.expression)
			if test.want == "" {
				if len(terms) != 0 {
					t.Fatalf("expected nothing carried, got %v", terms)
				}
				if len(skipped) == 0 {
					t.Fatal("a conjunct that is not carried must be reported")
				}
				return
			}
			if len(terms) != 1 {
				t.Fatalf("want 1 term, got %d (skipped %v)", len(terms), skipped)
			}
			if got := termKey(terms[0]); got != test.want {
				t.Errorf("got %q, want %q", got, test.want)
			}
		})
	}
}

// TestTranslateCELEnumPresence covers the enum, whose zero is a declared member
// and so the one implicit-presence type a `!has` can name.
func TestTranslateCELEnumPresence(t *testing.T) {
	root := loadMessage(t, coverageMessage)

	terms, _ := translate(t, root, "!has(this.flavor)")
	if len(terms) != 1 || termKey(terms[0]) != "flavor:zero" {
		t.Errorf("!has(this.flavor) = %v, want one flavor:zero term", terms)
	}
	if terms, skipped := translate(t, root, "has(this.flavor)"); len(terms) != 0 || len(skipped) != 1 {
		t.Errorf("has(this.flavor) = %v / %v, want it reported rather than carried", terms, skipped)
	}
}

func translate(t *testing.T, root protoreflect.MessageDescriptor, expression string) ([]CELTerm, []string) {
	t.Helper()
	env, err := celEnv()
	if err != nil {
		t.Fatal(err)
	}
	ast, issues := env.Parse(expression)
	if issues != nil && issues.Err() != nil {
		t.Fatalf("parse %q: %v", expression, issues.Err())
	}
	rep := ast.NativeRep()
	return translateCEL(root, rep.Expr(), rep.SourceInfo())
}

func termKey(term CELTerm) string {
	return strings.Join(term.Path, ".") + ":" + string(term.Kind)
}

// loadMessage reads the descriptor set the golden tests share, so the test
// needs neither protoc nor a network.
func loadMessage(t *testing.T, name string) protoreflect.MessageDescriptor {
	t.Helper()
	raw, err := os.ReadFile("../testdata/descriptors.binpb")
	if err != nil {
		t.Fatalf("read descriptor set (run `make testdata`): %v", err)
	}
	var set descriptorpb.FileDescriptorSet
	if err := proto.Unmarshal(raw, &set); err != nil {
		t.Fatal(err)
	}
	files, err := protodesc.NewFiles(&set)
	if err != nil {
		t.Fatal(err)
	}
	desc, err := files.FindDescriptorByName(protoreflect.FullName(name))
	if err != nil {
		t.Fatal(err)
	}
	msg, ok := desc.(protoreflect.MessageDescriptor)
	if !ok {
		t.Fatalf("%s is not a message", name)
	}
	return msg
}
