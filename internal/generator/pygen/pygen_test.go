package pygen

import (
	"strings"
	"testing"

	"google.golang.org/protobuf/compiler/protogen"
	"google.golang.org/protobuf/proto"
	"google.golang.org/protobuf/types/descriptorpb"
	"google.golang.org/protobuf/types/pluginpb"

	"github.com/d0whc3r/protoc-gen-strict/internal/parser"
)

// TestAliasNameCollision covers a field alias that wants the name of a class the
// same file imports: a `UserId id` field yields the alias `UserId`, and its own
// type is imported under that name. Emitted as written, the alias rebinds the
// class, and every annotation naming it would resolve to the annotation instead.
func TestAliasNameCollision(t *testing.T) {
	messages := []parser.MessageMetadata{
		{
			Name: "example.v1.User",
			Fields: []parser.FieldMetadata{{
				Name:      "id",
				ProtoType: "message:example.v1.UserId",
				Type:      parser.TypeRef{File: "example/v1/user.proto", Name: "UserId"},
				Required:  true,
			}},
		},
		{
			Name: "example.v1.UserId",
			Fields: []parser.FieldMetadata{
				{Name: "value", ProtoType: "string", Rules: []parser.Rule{{Kind: "string.uuid", Value: "true"}}},
			},
		},
	}

	out := generate(t, messages)
	if !strings.Contains(out, "from example.v1.user_pb2 import UserId") {
		t.Fatalf("expected the message class to be imported:\n%s", out)
	}
	if strings.Contains(out, "\nUserId = Annotated[") {
		t.Errorf("the alias rebinds the imported UserId class:\n%s", out)
	}
	if !strings.Contains(out, "\nUserId_ = Annotated[") {
		t.Errorf("expected the alias to be renamed out of the way:\n%s", out)
	}
}

// TestNoBlanketReexport checks the overlay imports only what an alias names. A
// message with no alias referencing it has nothing to do with this module, and
// importing it would re-export a class protoc-gen-python already exports.
func TestNoBlanketReexport(t *testing.T) {
	messages := []parser.MessageMetadata{
		{
			Name:   "example.v1.User",
			Fields: []parser.FieldMetadata{{Name: "id", ProtoType: "string", Rules: []parser.Rule{{Kind: "string.uuid", Value: "true"}}}},
		},
		{Name: "example.v1.Unrelated"},
	}

	out := generate(t, messages)
	if strings.Contains(out, "from example.v1.user_pb2 import") {
		t.Errorf("nothing in this file names a class, so nothing should be imported:\n%s", out)
	}
	if strings.Contains(out, "import (") {
		t.Errorf("the parenthesised re-export block is back:\n%s", out)
	}
}

// generate runs WriteFile over a synthetic file and returns its content.
func generate(t *testing.T, messages []parser.MessageMetadata) string {
	t.Helper()

	const path = "example/v1/user.proto"
	file := &descriptorpb.FileDescriptorProto{
		Name:    proto.String(path),
		Package: proto.String("example.v1"),
		Syntax:  proto.String("proto3"),
		Options: &descriptorpb.FileOptions{GoPackage: proto.String("example.test/example/v1")},
	}
	for _, msg := range messages {
		name := msg.Name[strings.LastIndex(msg.Name, ".")+1:]
		file.MessageType = append(file.MessageType, &descriptorpb.DescriptorProto{Name: proto.String(name)})
	}

	gen, err := protogen.Options{}.New(&pluginpb.CodeGeneratorRequest{
		FileToGenerate: []string{path},
		Parameter:      proto.String("paths=source_relative"),
		ProtoFile:      []*descriptorpb.FileDescriptorProto{file},
	})
	if err != nil {
		t.Fatalf("build protogen plugin: %v", err)
	}

	WriteFile(gen, gen.Files[0], messages)
	resp := gen.Response()
	if resp.Error != nil {
		t.Fatalf("plugin reported an error: %s", resp.GetError())
	}
	return resp.GetFile()[0].GetContent()
}

// TestConstraintsCarried covers which rules reach annotated_types. A rule
// carried as a constructor is enforced by anything reading that vocabulary, so
// one carried by mistake narrows harder than protovalidate does.
func TestConstraintsCarried(t *testing.T) {
	tests := []struct {
		name  string
		rules []parser.Rule
		want  []string
		avoid []string
	}{
		{
			name:  "lengths and bounds",
			rules: []parser.Rule{{Kind: "string.min_len", Value: "3"}, {Kind: "int32.gt", Value: "0"}},
			want:  []string{"MinLen(3)", "Gt(0)", "from annotated_types import Gt, MinLen"},
		},
		{
			name:  "exact length is the two bounds",
			rules: []parser.Rule{{Kind: "string.len", Value: "2"}},
			want:  []string{"MinLen(2)", "MaxLen(2)"},
		},
		{
			// `ignore` says the sibling rules do not always apply.
			name:  "ignore leaves every rule to runtime",
			rules: []parser.Rule{{Kind: "ignore", Value: "IGNORE_ALWAYS"}, {Kind: "string.min_len", Value: "3"}},
			want:  []string{`"string.min_len = 3"`},
			avoid: []string{"MinLen(3)", "from annotated_types import"},
		},
		{
			// protovalidate reads this as "outside 10..20"; the two constructors
			// would be a conjunction, which no value satisfies.
			name:  "reversed range",
			rules: []parser.Rule{{Kind: "int32.gt", Value: "20"}, {Kind: "int32.lt", Value: "10"}},
			want:  []string{`"int32.gt = 20"`, `"int32.lt = 10"`},
			avoid: []string{"Gt(20)", "Lt(10)"},
		},
		{
			// These count an element, not the collection they sit on.
			name: "element rules stay strings",
			rules: []parser.Rule{
				{Kind: "repeated.min_items", Value: "1"},
				{Kind: "repeated.items.string.min_len", Value: "4"},
				{Kind: "map.values.int32.gte", Value: "0"},
			},
			want:  []string{"MinLen(1)", `"repeated.items.string.min_len = 4"`, `"map.values.int32.gte = 0"`},
			avoid: []string{"MinLen(4)", "Ge(0)"},
		},
		{
			// A timestamp bound is a message, printed one rule per sub-field.
			name:  "well-known bounds stay strings",
			rules: []parser.Rule{{Kind: "duration.gte.seconds", Value: "3600"}},
			want:  []string{`"duration.gte.seconds = 3600"`},
			avoid: []string{"Ge(3600)"},
		},
		{
			// Go parses these; Python writes them as calls, not literals.
			name:  "non-finite bound stays a string",
			rules: []parser.Rule{{Kind: "double.lte", Value: "+Inf"}},
			want:  []string{`"double.lte = +Inf"`},
			avoid: []string{"Le(+Inf)"},
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			out := generate(t, []parser.MessageMetadata{{
				Name:   "example.v1.User",
				Fields: []parser.FieldMetadata{{Name: "value", ProtoType: "string", Rules: test.rules}},
			}})
			for _, want := range test.want {
				if !strings.Contains(out, want) {
					t.Errorf("missing %s in:\n%s", want, out)
				}
			}
			for _, avoid := range test.avoid {
				if strings.Contains(out, avoid) {
					t.Errorf("unexpected %s in:\n%s", avoid, out)
				}
			}
		})
	}
}
