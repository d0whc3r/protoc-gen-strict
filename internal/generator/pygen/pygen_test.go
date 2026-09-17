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

// TestAliasNameCollision covers a field alias that wants the name of a message
// the same file re-exports: `User.id` yields `UserId`, and a sibling
// `message UserId` is imported under that name. Emitted as written, the alias
// rebinds the class, so `from user_strict import UserId` hands a caller the
// annotation instead of the message it meant to build.
func TestAliasNameCollision(t *testing.T) {
	messages := []parser.MessageMetadata{
		{
			Name: "example.v1.User",
			Fields: []parser.FieldMetadata{
				{Name: "id", ProtoType: "string", Rules: []parser.Rule{{Kind: "string.min_len", Value: "5"}}},
			},
		},
		{
			Name: "example.v1.UserId",
			Fields: []parser.FieldMetadata{
				{Name: "value", ProtoType: "string", Rules: []parser.Rule{{Kind: "string.uuid", Value: "true"}}},
			},
		},
	}

	out := generate(t, messages)
	if !strings.Contains(out, "UserId as UserId,") {
		t.Fatalf("expected the message class to be re-exported:\n%s", out)
	}
	if strings.Contains(out, "UserId = Annotated[") {
		t.Errorf("the alias rebinds the re-exported UserId class:\n%s", out)
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
