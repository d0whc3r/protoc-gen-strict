package tsgen

import (
	"maps"
	"path"
	"path/filepath"
	"slices"
	"strconv"
	"strings"

	"google.golang.org/protobuf/compiler/protogen"

	"github.com/d0whc3r/protoc-gen-strict/internal/generator/emit"
)

// Module suffixes: protoc-gen-es writes <base>_pb, this plugin <base>.strict.
const (
	esSuffix     = "_pb"
	strictSuffix = ".strict"
)

// protobuf-es ships the well-known types in one package under their bare name,
// with no module per file.
const (
	wktDir    = "google/protobuf/"
	wktModule = "@bufbuild/protobuf/wkt"
)

// codegenModule holds the types protoc-gen-es annotates its descriptors with.
const codegenModule = "@bufbuild/protobuf/codegenv2"

// tsFile accumulates the body of one .strict.ts and the imports it turns out to
// need. Imports print before the body that discovers them, so the body is
// buffered rather than written straight out.
type tsFile struct {
	ctx   *Context
	proto string // this file's proto path, e.g. "example/v1/user.proto"
	out   string // this file's output path, e.g. "example/v1/user.strict.ts"
	base  string // the protoc-gen-es module next to it, e.g. "./user_pb"

	locals        *localNames                  // every identifier this file binds
	shapes        map[string]string            // type-only imports from base: symbol -> local
	values        map[string]string            // value imports from base, likewise
	helpers       map[string]string            // imports from strict/types
	codegen       map[string]string            // imports from @bufbuild/protobuf/codegenv2
	foreign       map[string]map[string]string // module specifier -> type-only imports
	foreignValues map[string]map[string]string // module specifier -> value imports
	lines         []string
}

func newTSFile(ctx *Context, file *protogen.File) *tsFile {
	return &tsFile{
		ctx:           ctx,
		proto:         file.Desc.Path(),
		out:           file.GeneratedFilenamePrefix + strictSuffix + ".ts",
		base:          "./" + path.Base(file.GeneratedFilenamePrefix) + esSuffix,
		locals:        newLocalNames(),
		shapes:        map[string]string{},
		values:        map[string]string{},
		helpers:       map[string]string{},
		codegen:       map[string]string{},
		foreign:       map[string]map[string]string{},
		foreignValues: map[string]map[string]string{},
	}
}

// localNames binds every identifier one output file uses to the module and
// symbol it came from.
//
// A proto name is unique only inside its package, so two packages both
// declaring `Thing` would have this file import `Thing` twice, and a local
// declaration can want a name an import already took. The second binding gets a
// `$1` suffix, as protoc-gen-es does, and every reference to it resolves through
// the same table.
type localNames struct {
	taken map[string]bool   // identifier -> bound
	bound map[string]string // module + "#" + symbol -> identifier
}

func newLocalNames() *localNames {
	return &localNames{taken: map[string]bool{}, bound: map[string]string{}}
}

// bind returns the identifier this file uses for a symbol, assigning one on
// first sight. An empty module is a declaration in the output file itself.
func (n *localNames) bind(module, symbol string) string {
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

// declare reserves a name the output file defines itself, so no import takes it
// first. Declarations are reserved before any body is emitted.
func (f *tsFile) declare(symbol string) string { return f.locals.bind("", symbol) }

func (f *tsFile) P(parts ...string) { f.lines = append(f.lines, strings.Join(parts, "")) }

// shape records a type-only import of a generated message type.
func (f *tsFile) shape(name string) string { return f.record(f.shapes, f.base, name) }

// value records an import of a generated const: a schema or a service.
func (f *tsFile) value(name string) string { return f.record(f.values, f.base, name) }

func (f *tsFile) helper(name string) string {
	return f.record(f.helpers, relImport(f.out, strings.TrimSuffix(strictTypesFile, ".ts")), name)
}

func (f *tsFile) gen(name string) string { return f.record(f.codegen, codegenModule, name) }

// record binds one imported symbol and remembers the bucket it prints from.
func (f *tsFile) record(bucket map[string]string, module, name string) string {
	local := f.locals.bind(module, name)
	bucket[name] = local
	return local
}

// shapeRef names a message's generated type, importing it from another file.
func (f *tsFile) shapeRef(fullName string) string {
	name := f.ctx.tsName(fullName)
	target := f.ctx.fileOf[fullName]
	if target == f.proto {
		return f.shape(name)
	}
	return f.foreignType(f.moduleFor(target, esSuffix), name)
}

// strictRef names a message's strict type, importing it from another file.
func (f *tsFile) strictRef(fullName string) string {
	name := f.ctx.strictName(fullName)
	target := f.ctx.fileOf[fullName]
	if target == f.proto {
		return f.declare(name) // reserved before the body was emitted
	}
	return f.foreignType(f.moduleFor(target, strictSuffix), name)
}

// moduleFor is the specifier for another proto file's generated or strict
// module, seen from this file. A well-known type has neither, so it resolves to
// protobuf-es whatever the suffix.
func (f *tsFile) moduleFor(protoPath, suffix string) string {
	if strings.HasPrefix(protoPath, wktDir) {
		return wktModule
	}
	return relImport(f.out, strings.TrimSuffix(protoPath, ".proto")+suffix)
}

func (f *tsFile) foreignType(module, name string) string {
	return f.record(byModule(f.foreign, module), module, name)
}

func (f *tsFile) foreignValue(module, name string) string {
	return f.record(byModule(f.foreignValues, module), module, name)
}

func byModule(modules map[string]map[string]string, module string) map[string]string {
	if modules[module] == nil {
		modules[module] = map[string]string{}
	}
	return modules[module]
}

func (f *tsFile) importLines() []string {
	var out []string
	add := func(names map[string]string, module string, typeOnly bool) {
		if len(names) == 0 {
			return
		}
		bindings := make([]string, 0, len(names))
		for _, symbol := range slices.Sorted(maps.Keys(names)) {
			binding := symbol
			// A symbol another module already exported under this name is
			// imported under the one this file bound it to.
			if local := names[symbol]; local != symbol {
				binding += " as " + local
			}
			bindings = append(bindings, binding)
		}
		keyword := "import "
		if typeOnly {
			keyword = "import type "
		}
		out = append(out, keyword+"{ "+strings.Join(bindings, ", ")+" } from "+strconv.Quote(module)+";")
	}

	add(f.helpers, relImport(f.out, strings.TrimSuffix(strictTypesFile, ".ts")), true)
	add(f.shapes, f.base, true)
	add(f.values, f.base, false)
	for _, module := range slices.Sorted(maps.Keys(f.foreign)) {
		add(f.foreign[module], module, true)
	}
	for _, module := range slices.Sorted(maps.Keys(f.foreignValues)) {
		add(f.foreignValues[module], module, false)
	}
	add(f.codegen, codegenModule, true)
	return out
}

func (f *tsFile) doc(indent string, lines []string) {
	lines = emit.CommentLines(lines)
	if len(lines) == 0 {
		return
	}
	if len(lines) == 1 {
		f.P(indent, "/** ", tsComment(lines[0]), " */")
		return
	}
	f.P(indent, "/**")
	for _, line := range lines {
		if line == "" {
			f.P(indent, " *")
			continue
		}
		f.P(indent, " * ", tsComment(line))
	}
	f.P(indent, " */")
}

// relImport renders the specifier for `to`, an extensionless path from the
// output root, as seen from `from`. Extensionless matches protoc-gen-es.
func relImport(from, to string) string {
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

func quotedUnion(names []string) string {
	quoted := make([]string, 0, len(names))
	for _, name := range names {
		quoted = append(quoted, strconv.Quote(name))
	}
	return strings.Join(quoted, " | ")
}

// tsComment stops a `*/` inside a rule, which is legal in a CEL string, from
// closing the block comment early.
func tsComment(line string) string {
	return strings.ReplaceAll(line, "*/", "*\\/")
}

// docLines drops the entries a missing proto comment leaves behind, so an
// undocumented service or method does not print an empty `/** */`.
func docLines(lines ...string) []string {
	return slices.DeleteFunc(lines, func(line string) bool { return line == "" })
}
