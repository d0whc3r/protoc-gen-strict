package tsgen

import (
	"slices"
	"testing"

	"google.golang.org/protobuf/compiler/protogen"
	"google.golang.org/protobuf/proto"
	"google.golang.org/protobuf/reflect/protodesc"
	"google.golang.org/protobuf/types/descriptorpb"
)

// protogenFileStub is the smallest file newTSFile accepts: the import bookkeeping
// under test reads only the output path it derives from it.
func protogenFileStub() *protogen.File {
	desc, err := protodesc.NewFile(&descriptorpb.FileDescriptorProto{
		Name:    proto.String("example/v1/thing.proto"),
		Package: proto.String("example.v1"),
		Syntax:  proto.String("proto3"),
	}, nil)
	if err != nil {
		panic(err)
	}
	return &protogen.File{Desc: desc, GeneratedFilenamePrefix: "example/v1/thing"}
}

// TestDocLines covers the service and method docs: a proto declaration without
// a leading comment used to print an empty `/** */` and a blank ` *` line.
func TestDocLines(t *testing.T) {
	f := &tsFile{}
	f.doc("  ", docLines(""))                               // an undocumented method
	f.doc("", docLines("", "The same service descriptor.")) // an undocumented service

	want := []string{"/** The same service descriptor. */"}
	if !slices.Equal(f.lines, want) {
		t.Errorf("lines = %q, want %q", f.lines, want)
	}
}

// TestImportCollision covers two packages that both declare `Thing`. Importing
// both under the bare name emits a duplicate identifier, and every reference
// after the first resolves to the wrong type.
func TestImportCollision(t *testing.T) {
	f := newTSFile(&Context{}, protogenFileStub())

	first := f.foreignType("./a/thing.strict", "ThingStrict")
	second := f.foreignType("./b/thing.strict", "ThingStrict")
	again := f.foreignType("./a/thing.strict", "ThingStrict")

	if first != "ThingStrict" || second != "ThingStrict$1" {
		t.Errorf("locals = %q, %q, want ThingStrict and ThingStrict$1", first, second)
	}
	if again != first {
		t.Errorf("second lookup of the same symbol = %q, want %q", again, first)
	}

	want := []string{
		`import type { ThingStrict } from "./a/thing.strict";`,
		`import type { ThingStrict as ThingStrict$1 } from "./b/thing.strict";`,
	}
	if got := f.importLines(); !slices.Equal(got, want) {
		t.Errorf("importLines() = %q, want %q", got, want)
	}
}

// TestDeclarationKeepsBareName covers a name the output file declares itself.
// It is reserved before any body is emitted, so an import of the same name from
// another package is the one that gets renamed.
func TestDeclarationKeepsBareName(t *testing.T) {
	f := newTSFile(&Context{}, protogenFileStub())

	declared := f.declare("ThingStrict")
	imported := f.foreignType("./other.strict", "ThingStrict")

	if declared != "ThingStrict" || imported != "ThingStrict$1" {
		t.Errorf("declared %q, imported %q; want the declaration to keep the bare name", declared, imported)
	}
}
