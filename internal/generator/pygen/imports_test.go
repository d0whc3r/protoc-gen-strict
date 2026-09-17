package pygen

import (
	"slices"
	"testing"

	"github.com/d0whc3r/protoc-gen-strict/internal/parser"
)

// TestImportsRootLevelProto covers a proto at the import root: `user.proto`
// becomes `user_pb2`, not `pkg.user_pb2`. Slicing at the last dot of that name
// used to go out of range.
func TestImportsRootLevelProto(t *testing.T) {
	p := &pyImports{
		self:    "root.proto",
		symbols: map[string]bool{},
		modules: map[string]string{"user.proto": "_user_pb2"},
	}

	want := "import user_pb2 as _user_pb2"
	for _, line := range p.lines() {
		if line == want {
			return
		}
	}
	t.Errorf("lines() = %q, want it to contain %q", p.lines(), want)
}

// TestImportAliasCollision covers two protos with the same base name. Under the
// base-name alias protoc-gen-pyi uses, the second import rebinds the first, and
// every field annotated with a type from a/common.proto silently becomes the
// same-named type from b/common.proto.
func TestImportAliasCollision(t *testing.T) {
	p := &pyImports{self: "root.proto", symbols: map[string]bool{}, modules: map[string]string{}, typing: map[string]bool{}}

	first := p.base("message:a.Thing", parser.TypeRef{File: "a/common.proto", Name: "Thing"})
	second := p.base("message:b.Thing", parser.TypeRef{File: "b/common.proto", Name: "Thing"})

	if first == second {
		t.Fatalf("both modules resolved to %q", first)
	}
	want := []string{
		"from a import common_pb2 as _a_common_pb2",
		"from b import common_pb2 as _b_common_pb2",
	}
	lines := p.lines()
	for _, line := range want {
		if !slices.Contains(lines, line) {
			t.Errorf("lines() = %q, want it to contain %q", lines, line)
		}
	}
}
