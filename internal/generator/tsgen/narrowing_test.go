package tsgen

import (
	"testing"

	"github.com/d0whc3r/protoc-gen-strict/internal/parser"
)

// TestIgnoreAlwaysStopsPropagation covers a message-typed field validation is
// switched off for. IGNORE_ALWAYS skips protovalidate's recursion into the
// message too, so demanding the target's strict type would reject a value the
// proto accepts.
func TestIgnoreAlwaysStopsPropagation(t *testing.T) {
	const target = "example.v1.Detail"
	c := &Context{needsStrict: map[string]bool{target: true}}

	ignored := parser.FieldMetadata{
		Name:      "unchecked",
		ProtoType: "message:" + target,
		Rules:     []parser.Rule{{Kind: "ignore", Value: "IGNORE_ALWAYS"}},
	}
	if got := c.narrowingOf(ignored).Target; got != "" {
		t.Errorf("Target = %q, want none for an IGNORE_ALWAYS field", got)
	}

	// IGNORE_IF_ZERO_VALUE only skips the unset case, which the optional
	// property protoc-gen-es declares already allows.
	whenSet := parser.FieldMetadata{
		Name:      "detail",
		ProtoType: "message:" + target,
		Rules:     []parser.Rule{{Kind: "ignore", Value: "IGNORE_IF_ZERO_VALUE"}},
	}
	if got := c.narrowingOf(whenSet).Target; got != target {
		t.Errorf("Target = %q, want %q", got, target)
	}
}
