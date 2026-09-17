package generator

import (
	"maps"
	"path"
	"path/filepath"
	"slices"
	"strconv"
	"strings"

	"google.golang.org/protobuf/compiler/protogen"
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

// tsFile accumulates the body of one .strict.ts and the imports it turns out to
// need. Imports print before the body that discovers them, so the body is
// buffered rather than written straight out.
type tsFile struct {
	ctx   *Context
	proto string // this file's proto path, e.g. "example/v1/user.proto"
	out   string // this file's output path, e.g. "example/v1/user.strict.ts"
	base  string // the protoc-gen-es module next to it, e.g. "./user_pb"

	shapes        map[string]bool            // type-only names from base
	values        map[string]bool            // value names from base
	helpers       map[string]bool            // names from strict/types
	codegen       map[string]bool            // names from @bufbuild/protobuf/codegenv2
	foreign       map[string]map[string]bool // module specifier -> type-only names
	foreignValues map[string]map[string]bool // module specifier -> value names
	lines         []string
}

func newTSFile(ctx *Context, file *protogen.File) *tsFile {
	return &tsFile{
		ctx:           ctx,
		proto:         file.Desc.Path(),
		out:           file.GeneratedFilenamePrefix + strictSuffix + ".ts",
		base:          "./" + path.Base(file.GeneratedFilenamePrefix) + esSuffix,
		shapes:        map[string]bool{},
		values:        map[string]bool{},
		helpers:       map[string]bool{},
		codegen:       map[string]bool{},
		foreign:       map[string]map[string]bool{},
		foreignValues: map[string]map[string]bool{},
	}
}

func (f *tsFile) P(parts ...string) { f.lines = append(f.lines, strings.Join(parts, "")) }

// shape records a type-only import of a generated message type.
func (f *tsFile) shape(name string) string { f.shapes[name] = true; return name }

// value records an import of a generated const: a schema or a service.
func (f *tsFile) value(name string) string { f.values[name] = true; return name }

func (f *tsFile) helper(name string) string { f.helpers[name] = true; return name }

func (f *tsFile) gen(name string) string { f.codegen[name] = true; return name }

// shapeRef names a message's generated type, importing it from another file.
func (f *tsFile) shapeRef(fullName string) string {
	name := f.ctx.tsName(fullName)
	target := f.ctx.fileOf[fullName]
	if target == f.proto {
		return f.shape(name)
	}
	f.foreignType(f.moduleFor(target, esSuffix), name)
	return name
}

// strictRef names a message's strict type, importing it from another file.
func (f *tsFile) strictRef(fullName string) string {
	name := f.ctx.strictName(fullName)
	target := f.ctx.fileOf[fullName]
	if target == f.proto {
		return name
	}
	f.foreignType(f.moduleFor(target, strictSuffix), name)
	return name
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

func (f *tsFile) foreignType(module, name string)  { addImport(f.foreign, module, name) }
func (f *tsFile) foreignValue(module, name string) { addImport(f.foreignValues, module, name) }

func addImport(byModule map[string]map[string]bool, module, name string) {
	if byModule[module] == nil {
		byModule[module] = map[string]bool{}
	}
	byModule[module][name] = true
}

func (f *tsFile) importLines() []string {
	var out []string
	add := func(names map[string]bool, module string, typeOnly bool) {
		if len(names) == 0 {
			return
		}
		keyword := "import "
		if typeOnly {
			keyword = "import type "
		}
		out = append(out, keyword+"{ "+strings.Join(slices.Sorted(maps.Keys(names)), ", ")+" } from "+strconv.Quote(module)+";")
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
	add(f.codegen, "@bufbuild/protobuf/codegenv2", true)
	return out
}

func (f *tsFile) doc(indent string, lines []string) {
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
