// Package tscode holds the TypeScript source conventions both TypeScript
// emitters share, tsgen's overlays and schemagen's schemas: how a module binds
// its identifiers, how a JSDoc block is printed, and how one module imports
// another by relative path.
package tscode

import (
	"path"
	"path/filepath"
	"strconv"
	"strings"

	"github.com/d0whc3r/protoc-gen-strict/internal/generator/emit"
)

// Names binds every identifier one output module uses to the module and symbol
// it came from.
//
// A proto name is unique only inside its package, so two packages both
// declaring `Thing` would have a module import `Thing` twice, and a local
// declaration can want a name an import already took. The second binding gets
// a `$1` suffix, as protoc-gen-es does, and every reference to it resolves
// through the same table.
type Names struct {
	taken map[string]bool   // identifier -> bound
	bound map[string]string // module + "#" + symbol -> identifier
}

// NewNames returns an empty table.
func NewNames() *Names {
	return &Names{taken: map[string]bool{}, bound: map[string]string{}}
}

// Bind returns the identifier the module uses for a symbol, assigning one on
// first sight. An empty module is a declaration of the output module itself.
func (n *Names) Bind(module, symbol string) string {
	key := module + "#" + symbol
	if local, ok := n.bound[key]; ok {
		return local
	}
	local := symbol
	for i := 1; n.taken[local]; i++ {
		local = symbol + "$" + strconv.Itoa(i)
	}
	n.taken[local] = true
	n.bound[key] = local
	return local
}

// DocBlock renders a JSDoc block, one physical line per entry, each prefixed
// with indent. Trailing empty entries are dropped; with none left, it renders
// nothing.
func DocBlock(indent string, lines []string) []string {
	lines = emit.CommentLines(lines)
	for len(lines) > 0 && lines[len(lines)-1] == "" {
		lines = lines[:len(lines)-1]
	}
	switch len(lines) {
	case 0:
		return nil
	case 1:
		return []string{indent + "/** " + comment(lines[0]) + " */"}
	}

	out := []string{indent + "/**"}
	for _, line := range lines {
		if line == "" {
			out = append(out, indent+" *")
			continue
		}
		out = append(out, indent+" * "+comment(line))
	}
	return append(out, indent+" */")
}

// comment stops a `*/` inside a rule, which is legal in a CEL string, from
// closing the block comment early.
func comment(line string) string {
	return strings.ReplaceAll(line, "*/", "*\\/")
}

// RelImport renders the specifier for `to`, an extensionless path from the
// output root, as seen from `from`. Extensionless matches protoc-gen-es.
func RelImport(from, to string) string {
	rel, err := filepath.Rel(path.Dir(from), to)
	if err != nil {
		return "./" + to // unreachable for the slash paths protogen hands us
	}
	rel = filepath.ToSlash(rel)
	if !strings.HasPrefix(rel, ".") {
		rel = "./" + rel
	}
	return rel
}
