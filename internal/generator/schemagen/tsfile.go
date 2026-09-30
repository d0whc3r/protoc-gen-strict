package schemagen

import (
	"encoding/json"
	"maps"
	"slices"
	"strconv"
	"strings"

	"github.com/d0whc3r/protoc-gen-strict/internal/generator/tscode"
)

// tsFile accumulates the body of one generated module and the imports it turns
// out to need. Imports print before the body that discovers them, so the body
// is buffered rather than written straight out.
type tsFile struct {
	out    string                       // output path, e.g. "example/v1/user.zod.ts"
	ext    string                       // appended to a relative import, e.g. ".js"; see Output
	names  *tscode.Names                // every identifier this file binds
	named  map[string]map[string]string // module -> symbol -> local
	json   map[string]string            // JSON module -> default-import local
	header []string                     // lines above the imports
	lines  []string
}

func newTSFile(out, ext string) *tsFile {
	return &tsFile{
		out:   out,
		ext:   ext,
		names: tscode.NewNames(),
		named: map[string]map[string]string{},
		json:  map[string]string{},
	}
}

// declare reserves a name the file defines, before any import can take it.
func (f *tsFile) declare(symbol string) string { return f.names.Bind("", symbol) }

// value records a value import and returns the local it is bound to.
func (f *tsFile) value(module, symbol string) string {
	return record(f.named, module, symbol, f.names.Bind(module, symbol))
}

// jsonModule records the default import of a JSON module under local.
func (f *tsFile) jsonModule(module, local string) string {
	local = f.names.Bind(module, local)
	f.json[module] = local
	return local
}

func record(buckets map[string]map[string]string, module, symbol, local string) string {
	if buckets[module] == nil {
		buckets[module] = map[string]string{}
	}
	buckets[module][symbol] = local
	return local
}

// P appends one line to the body.
func (f *tsFile) P(parts ...string) { f.lines = append(f.lines, strings.Join(parts, "")) }

// doc prints a JSDoc block, one physical line per entry.
func (f *tsFile) doc(indent string, lines []string) {
	f.lines = append(f.lines, tscode.DocBlock(indent, lines)...)
}

// render returns the whole module: header, imports sorted by module, body.
func (f *tsFile) render() string {
	var imports []string
	for _, module := range slices.Sorted(maps.Keys(f.json)) {
		imports = append(imports, "import "+f.json[module]+" from "+tsString(module)+` with { type: "json" };`)
	}
	for _, module := range slices.SortedFunc(maps.Keys(f.named), compareModules) {
		specifier := module
		if strings.HasPrefix(module, ".") {
			specifier += f.ext
		}
		imports = append(imports, "import { "+bindings(f.named[module])+" } from "+tsString(specifier)+";")
	}

	out := append(slices.Clone(f.header), "")
	if len(imports) > 0 {
		out = append(append(out, imports...), "")
	}
	out = append(out, f.lines...)
	for len(out) > 0 && out[len(out)-1] == "" {
		out = out[:len(out)-1]
	}
	return strings.Join(out, "\n") + "\n"
}

// compareModules puts packages before relative paths, then sorts by name, so
// `zod` and `@bufbuild/...` lead and the local modules follow.
func compareModules(a, b string) int {
	ra, rb := strings.HasPrefix(a, "."), strings.HasPrefix(b, ".")
	if ra != rb {
		if ra {
			return 1
		}
		return -1
	}
	return strings.Compare(a, b)
}

func bindings(names map[string]string) string {
	parts := make([]string, 0, len(names))
	for _, symbol := range slices.Sorted(maps.Keys(names)) {
		binding := symbol
		if local := names[symbol]; local != symbol {
			binding += " as " + local
		}
		parts = append(parts, binding)
	}
	return strings.Join(parts, ", ")
}

// tsString renders s as a TypeScript string literal. JSON's string grammar is a
// subset of JavaScript's, and encoding/json escapes the U+2028 and U+2029 line
// separators an older engine would reject inside a literal.
func tsString(s string) string {
	var b strings.Builder
	enc := json.NewEncoder(&b)
	enc.SetEscapeHTML(false) // `<` and `&` are fine in a TypeScript literal
	if err := enc.Encode(s); err != nil {
		return strconv.Quote(s) // unreachable: every Go string encodes
	}
	return strings.TrimSuffix(b.String(), "\n")
}
