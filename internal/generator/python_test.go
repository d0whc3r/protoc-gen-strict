package generator

import "testing"

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
