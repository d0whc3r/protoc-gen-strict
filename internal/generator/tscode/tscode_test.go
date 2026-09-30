package tscode

import (
	"slices"
	"testing"
)

// TestDocBlock covers the JSDoc shapes: nothing for no text, one line for one
// entry, a `*/` from a CEL string that must not close the block, and the
// trailing blank entries a doc builder leaves behind.
func TestDocBlock(t *testing.T) {
	tests := []struct {
		name  string
		lines []string
		want  []string
	}{
		{"empty", []string{"", ""}, nil},
		{"one line", []string{"Only.", ""}, []string{"  /** Only. */"}},
		{"comment terminator", []string{"a */ b", "", "c\nd"}, []string{"  /**", "   * a *\\/ b", "   *", "   * c", "   * d", "   */"}},
	}
	for _, tt := range tests {
		if got := DocBlock("  ", tt.lines); !slices.Equal(got, tt.want) {
			t.Errorf("%s: DocBlock = %q, want %q", tt.name, got, tt.want)
		}
	}
}

// TestBind covers a symbol two modules export: the second gets `$1`, and each
// module keeps the identifier it was first given.
func TestBind(t *testing.T) {
	n := NewNames()
	first, second := n.Bind("./a_pb", "Money"), n.Bind("./b_pb", "Money")
	if first != "Money" || second != "Money$1" || n.Bind("./a_pb", "Money") != "Money" {
		t.Errorf("Bind = %s, %s; want Money, Money$1", first, second)
	}
}

// TestRelImport covers a sibling, a module up the tree, and one in the root.
func TestRelImport(t *testing.T) {
	for _, tt := range []struct{ from, to, want string }{
		{"example/v1/user.zod.ts", "example/v1/user_pb", "./user_pb"},
		{"example/v1/user.zod.ts", "strict/protovalidate", "../../strict/protovalidate"},
		{"user.zod.ts", "strict/types", "./strict/types"},
	} {
		if got := RelImport(tt.from, tt.to); got != tt.want {
			t.Errorf("RelImport(%q, %q) = %s, want %s", tt.from, tt.to, got, tt.want)
		}
	}
}
