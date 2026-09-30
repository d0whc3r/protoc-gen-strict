// Package schemagen writes the runtime schemas protoc-gen-strict-schema emits:
// a JSON Schema module per proto file that loosens protoschema-jsonschema's
// output where it rejects values protovalidate accepts, and a Zod module per
// proto file, generated from the IR, whose rules are native Zod checks where a
// built-in is exact and protovalidate everywhere else.
//
// docs/internals.md describes the pipeline, docs/rule-coverage-schema.md the
// rule tables this package implements.
package schemagen

import (
	"strconv"
	"strings"

	"google.golang.org/protobuf/compiler/protogen"

	"github.com/d0whc3r/protoc-gen-strict/internal/generator/emit"
	"github.com/d0whc3r/protoc-gen-strict/internal/parser"
)

// ZodMajor is the Zod release line a module is written for. The two differ in
// what their string length checks count and in how a recursive schema is
// declared; see zodfield.go and zod.go.
type ZodMajor int

// The Zod lines the plugin targets.
const (
	Zod3 ZodMajor = 3
	Zod4 ZodMajor = 4
)

// suffix is what the major's modules append to the proto path, distinct so one
// tree holds both: user.zod.ts for Zod 4, user.zod3.ts for Zod 3.
func (m ZodMajor) suffix() string {
	if m == Zod3 {
		return zod3Suffix
	}
	return zodSuffix
}

// Module suffixes and fixed locations in the output tree.
const (
	esSuffix      = "_pb"     // protoc-gen-es: user.proto -> user_pb.ts
	schemaSuffix  = ".schema" // target json: user.proto -> user.schema.ts
	zodSuffix     = ".zod"    // target zod: user.proto -> user.zod.ts
	zod3Suffix    = ".zod3"   // target zod3: user.proto -> user.zod3.ts
	jsonSchemaDir = "jsonschema/"
	bundleSuffix  = ".jsonschema.bundle.json" // protoschema-jsonschema, target=json-bundle
	defSuffix     = ".jsonschema.json"        // a $defs key inside a bundle
	wktPackage    = "google.protobuf."
	wktDir        = "google/protobuf/"
)

// Context is one pass over every file in the request. Unlike the overlays it
// parses every file, not only the ones marked for generation: a JSON Schema
// bundle carries the defs of the imported messages it reaches, and a Zod field
// of an imported enum is written out from the enum's members.
type Context struct {
	messages  map[string]parser.MessageMetadata // by fully qualified name
	enums     map[string]parser.EnumMetadata
	fileOf    map[string]string // fully qualified name -> proto file path
	pkgOf     map[string]string // proto file path -> proto package
	generated map[string]bool   // proto file paths marked for generation
	byFile    map[string][]parser.MessageMetadata
	enumsOf   map[string][]parser.EnumMetadata
	stems     map[string]string // fully qualified name -> schema stem, e.g. "warehouseAddress"
}

// New indexes what the run parsed. parsed and enums hold every file of the
// request, keyed by proto path.
func New(files []*protogen.File, parsed map[string][]parser.MessageMetadata, enums map[string][]parser.EnumMetadata) *Context {
	c := &Context{
		messages:  map[string]parser.MessageMetadata{},
		enums:     map[string]parser.EnumMetadata{},
		fileOf:    map[string]string{},
		pkgOf:     map[string]string{},
		generated: map[string]bool{},
		byFile:    parsed,
		enumsOf:   enums,
		stems:     map[string]string{},
	}
	for _, file := range files {
		path := file.Desc.Path()
		c.pkgOf[path] = string(file.Desc.Package())
		// protobuf-es writes no module for a well-known type, so neither does
		// this plugin: strict/wkt.zod.ts serves them.
		if file.Generate && !strings.HasPrefix(path, wktDir) {
			c.generated[path] = true
		}
		for _, msg := range parsed[path] {
			c.messages[msg.Name] = msg
			c.fileOf[msg.Name] = path
		}
		for _, enum := range enums[path] {
			c.enums[enum.Name] = enum
			c.fileOf[enum.Name] = path
		}
		c.nameTypes(path)
	}
	return c
}

// nameTypes assigns the stem of every message and enum of a file: ident in
// camelCase, e.g. "Warehouse_Address" -> "warehouseAddress". `Order.Status` and
// `OrderStatus` fold to the same stem, so the later one, enums after messages,
// gets a `$1`, as protoc-gen-es does with its own clashes.
func (c *Context) nameTypes(path string) {
	names := make([]string, 0, len(c.byFile[path])+len(c.enumsOf[path]))
	for _, msg := range c.byFile[path] {
		names = append(names, msg.Name)
	}
	for _, enum := range c.enumsOf[path] {
		names = append(names, enum.Name)
	}

	taken := map[string]bool{}
	for _, name := range names {
		folded := emit.LowerFirst(emit.Camel(c.ident(name)))
		stem := folded
		for i := 1; taken[stem]; i++ {
			stem = folded + "$" + strconv.Itoa(i)
		}
		taken[stem] = true
		c.stems[name] = stem
	}
}

// Generates reports whether the run writes modules for a proto file.
func (c *Context) Generates(path string) bool { return c.generated[path] }

// ident is the identifier protoc-gen-es exports for a message or enum: no
// package, nesting joined by underscores. The schemas are named after it.
func (c *Context) ident(fullName string) string {
	pkg := c.pkgOf[c.fileOf[fullName]]
	return strings.ReplaceAll(strings.TrimPrefix(fullName, pkg+"."), ".", "_")
}

// valueName is the stem every schema of a message or enum is declared under,
// e.g. "warehouseAddress" for warehouseAddressZod; see nameTypes.
func (c *Context) valueName(fullName string) string { return c.stems[fullName] }

// schemaSymbol is the descriptor protoc-gen-es exports for a message:
// <Name>Schema, with a `$` added where another type of the file is already
// named that, as protoc-gen-es does.
func (c *Context) schemaSymbol(fullName string) string {
	symbol := c.ident(fullName) + "Schema"
	file := c.fileOf[fullName]
	for _, msg := range c.byFile[file] {
		if c.ident(msg.Name) == symbol {
			return symbol + "$"
		}
	}
	for _, enum := range c.enumsOf[file] {
		if c.ident(enum.Name) == symbol {
			return symbol + "$"
		}
	}
	return symbol
}

// topLevel reports whether a message is declared at file scope, the only kind
// protoschema-jsonschema writes a bundle for.
func (c *Context) topLevel(fullName string) bool {
	pkg := c.pkgOf[c.fileOf[fullName]]
	return !strings.Contains(strings.TrimPrefix(fullName, pkg+"."), ".")
}

// referenceTarget is the fully qualified message or enum a ProtoType names,
// e.g. "message:example.v1.Address" -> "example.v1.Address".
func referenceTarget(protoType string) (kind, fullName string, ok bool) {
	kind, fullName, ok = strings.Cut(protoType, ":")
	if !ok || (kind != "message" && kind != "enum") {
		return "", "", false
	}
	return kind, fullName, true
}

// ignoreValue is the `ignore` set under a nested rule prefix, e.g.
// "repeated.items.", or "" when none is. The field's own is FieldMetadata.Ignore,
// which the parser resolves.
func ignoreValue(field parser.FieldMetadata, prefix string) string {
	value, _ := emit.RuleValue(field, prefix+emit.IgnoreRule)
	return value
}

// The `ignore` value that exempts the zero value, next to emit.IgnoreAlways,
// and the leaf of the rules that document a value rather than constrain it.
const (
	ignoreIfZero    = "IGNORE_IF_ZERO_VALUE"
	exampleRuleLeaf = "example"
)

// isNoop reports rules that constrain nothing: `example` documents a value, and
// a well-known format set to false switches its check off.
func isNoop(rule parser.Rule) bool {
	if leafOf(rule.Kind) == exampleRuleLeaf {
		return true
	}
	return rule.Value == "false" && boolFormats[leafOf(rule.Kind)]
}

// leafOf is the last segment of a dotted rule kind.
func leafOf(kind string) string {
	if i := strings.LastIndex(kind, "."); i >= 0 {
		return kind[i+1:]
	}
	return kind
}

// boolFormats are the StringRules and BytesRules formats switched on by `true`.
var boolFormats = map[string]bool{
	"email": true, "hostname": true, "ip": true, "ipv4": true, "ipv6": true,
	"uri": true, "uri_ref": true, "address": true, "uuid": true, "tuuid": true,
	"ip_with_prefixlen": true, "ipv4_with_prefixlen": true, "ipv6_with_prefixlen": true,
	"ip_prefix": true, "ipv4_prefix": true, "ipv6_prefix": true,
	"host_and_port": true, "ulid": true, "protobuf_fqn": true, "protobuf_dot_fqn": true,
}

// int32Kinds are the 32-bit integers protojson writes as JSON numbers.
var int32Kinds = map[string]bool{
	"int32": true, "sint32": true, "sfixed32": true, "uint32": true, "fixed32": true,
}

// int64Kinds are the 64-bit integers protojson writes as decimal strings; the
// value is whether the type is signed.
var int64Kinds = map[string]bool{
	"int64": true, "sint64": true, "sfixed64": true, "uint64": false, "fixed64": false,
}

// floatKinds are the floating-point types, which protojson writes as a number
// or one of the strings "NaN", "Infinity" and "-Infinity".
var floatKinds = map[string]bool{"float": true, "double": true}

// Output is what each module is written as, and how it names the modules it
// imports: protoc-gen-es's `target` and `import_extension`, which the two
// plugin entries should agree on.
type Output struct {
	TS, JS, DTS bool
	ImportExt   string // "" or ".js", appended to every relative import
}

// flavor is one of the files a module is written as.
type flavor int

const (
	flavorTS  flavor = iota // .ts: the code and its types
	flavorJS                // .js: the code alone
	flavorDTS               // .d.ts: the types alone
)

// flavors lists what out asks for, in a fixed order.
func (o Output) flavors() []flavor {
	var out []flavor
	if o.TS {
		out = append(out, flavorTS)
	}
	if o.JS {
		out = append(out, flavorJS)
	}
	if o.DTS {
		out = append(out, flavorDTS)
	}
	return out
}

// extension is the file extension of a flavor.
func (f flavor) extension() string {
	switch f {
	case flavorJS:
		return ".js"
	case flavorDTS:
		return ".d.ts"
	}
	return ".ts"
}
