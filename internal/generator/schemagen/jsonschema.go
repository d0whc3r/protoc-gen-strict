package schemagen

import (
	"embed"
	"fmt"
	"slices"
	"strings"

	"google.golang.org/protobuf/compiler/protogen"

	"github.com/d0whc3r/protoc-gen-strict/internal/generator/emit"
	"github.com/d0whc3r/protoc-gen-strict/internal/generator/tscode"
	"github.com/d0whc3r/protoc-gen-strict/internal/parser"
)

// jsonRuntimeModule is the shared module every <file>.schema.ts imports `loosen`
// from, emitted once per run, extensionless. It holds fixesTable, the fixes
// for every message the run's bundles can reach, which is why the plugin entry
// needs `strategy: all`.
const jsonRuntimeModule = "strict/jsonschema"

// The hand-written TypeScript, JavaScript and declarations the plugin emits as
// its strict/ modules, one set per flavor.
//
//go:embed runtime
var runtimeFS embed.FS

// runtimeSource reads one hand-written module, e.g. "jsonschema" as ".ts".
func runtimeSource(name string, fl flavor) (string, error) {
	data, err := runtimeFS.ReadFile("runtime/" + name + fl.extension())
	if err != nil {
		return "", fmt.Errorf("the plugin ships no runtime/%s%s: %w", name, fl.extension(), err)
	}
	return string(data), nil
}

// wktFixes are the defs protoschema-jsonschema writes for a well-known type
// with a keyword that rejects valid values, whatever field reaches them.
var wktFixes = map[string]jsonPlan{
	// format "duration" is ISO 8601 (PT1.5S); protojson writes "1.5s".
	"google.protobuf.Duration": {fixes: []jsonFix{{
		drop: []string{"format"}, reasons: []string{"google.protobuf.Duration: format is ISO 8601, protojson writes \"1.5s\""},
	}}},
	"google.protobuf.BytesValue": {fixes: []jsonFix{{
		drop: []string{"pattern"}, reasons: []string{"bytes: base64 pattern rejects URL-safe"},
	}}},
}

// WriteJSONSchema writes <file>.schema.ts, or its .js and .d.ts: a
// <message>JsonSchema for every top-level message, the only kind
// protoschema-jsonschema bundles.
func WriteJSONSchema(gen *protogen.Plugin, file *protogen.File, ctx *Context, out Output) {
	path := file.Desc.Path()
	var messages []parser.MessageMetadata
	for _, msg := range ctx.byFile[path] {
		if ctx.topLevel(msg.Name) {
			messages = append(messages, msg)
		}
	}
	if len(messages) == 0 {
		return
	}

	for _, fl := range out.flavors() {
		name := emit.OutputPrefix(path) + schemaSuffix + fl.extension()
		f := newTSFile(name, out.ImportExt)
		f.header = generatedHeader(path)
		names := make([]string, len(messages))
		for i, msg := range messages {
			names[i] = f.declare(ctx.valueName(msg.Name) + "JsonSchema")
		}
		var loosen string
		if fl != flavorDTS {
			loosen = f.value(tscode.RelImport(name, jsonRuntimeModule), "loosen")
		}

		for i, msg := range messages {
			bundle := f.jsonModule(tscode.RelImport(name, jsonSchemaDir+msg.Name+bundleSuffix), ctx.valueName(msg.Name)+"Bundle")
			f.doc("", jsonDoc(ctx, msg))
			if fl == flavorDTS {
				// loosen returns its argument's type: the bundle's.
				f.P("export declare const ", names[i], ": typeof ", bundle, ";")
			} else {
				f.P("export const ", names[i], " = ", loosen, "(", bundle, ");")
			}
			f.P()
		}
		write(gen, name, f)
	}
}

// jsonDoc is the doc comment of one <message>JsonSchema.
func jsonDoc(ctx *Context, msg parser.MessageMetadata) []string {
	var lines []string
	if msg.Comments != "" {
		lines = append(lines, emit.OneLine(msg.Comments), "")
	}
	lines = append(lines, "JSON Schema 2020-12 for "+msg.Name+", from protoschema-jsonschema.")
	if notes := jsonNotes(ctx, msg); len(notes) > 0 {
		lines = append(lines, "", "Left to runtime validation:")
		lines = append(lines, renderNotes(notes)...)
	}
	return lines
}

// jsonNotes lists what one bundle's top-level schema does not carry: its own
// fields and message rules, and those of the messages declared inside it.
func jsonNotes(ctx *Context, top parser.MessageMetadata) []emit.RuntimeNote {
	var notes []emit.RuntimeNote
	for _, msg := range ctx.byFile[ctx.fileOf[top.Name]] {
		prefix := ""
		switch {
		case msg.Name == top.Name:
		case strings.HasPrefix(msg.Name, top.Name+"."):
			prefix = strings.TrimPrefix(msg.Name, top.Name+".") + "."
		default:
			continue
		}
		notes = append(notes, messageNotes(msg, prefix)...)
		for _, field := range msg.Fields {
			if lines := planJSON(field).runtime; len(lines) > 0 {
				notes = append(notes, emit.RuntimeNote{Subject: prefix + field.Name, Lines: lines})
			}
		}
		// protojson omits an implicit field at its zero value, so an absent key
		// is "" or 0 or [], which no keyword of the schema sees.
		if absent := absentChecked(msg); len(absent) > 0 {
			notes = append(notes, emit.RuntimeNote{
				Subject: prefix + "absent fields",
				Lines:   []string{strings.Join(absent, ", ") + ": an absent key is the zero value, which the rules still apply to"},
			})
		}
	}
	return notes
}

// messageNotes renders the rules that belong to no single field: oneof
// exclusivity and message-level CEL. No generated schema carries them.
func messageNotes(msg parser.MessageMetadata, prefix string) []emit.RuntimeNote {
	var notes []emit.RuntimeNote
	for _, oneof := range msg.Oneofs {
		notes = append(notes, emit.RuntimeNote{Subject: prefix + emit.OneofLine(oneof)})
	}
	for _, rule := range msg.CEL {
		label := prefix + "cel[" + emit.CELLabel(rule) + "]"
		if rule.ParseError != "" {
			notes = append(notes, emit.RuntimeNote{Subject: label, Lines: []string{emit.UnparseablePrefix + rule.ParseError}})
			continue
		}
		lines := []string{rule.Expression}
		if rule.Message != "" {
			lines = append(lines, "message: "+rule.Message)
		}
		notes = append(notes, emit.RuntimeNote{Subject: label, Lines: lines})
	}
	return notes
}

// renderNotes indents each subject by two and its lines by four, the layout
// the overlays use.
func renderNotes(notes []emit.RuntimeNote) []string {
	var out []string
	for _, note := range notes {
		out = append(out, "  "+note.Subject)
		for _, line := range emit.CommentLines(note.Lines) {
			out = append(out, "    "+line)
		}
	}
	return out
}

// WriteJSONRuntime writes strict/jsonschema.ts, or its .js and .d.ts: the
// hand-written walk, then fixesTable, the fixes for every message a generated
// file's bundle can reach.
func WriteJSONRuntime(gen *protogen.Plugin, ctx *Context, out Output) error {
	for _, fl := range out.flavors() {
		source, err := runtimeSource("jsonschema", fl)
		if err != nil {
			return err
		}
		name := jsonRuntimeModule + fl.extension()
		f := newTSFile(name, out.ImportExt)
		f.header = generatedHeader("")
		f.lines = append(f.lines, runtimeLines(source)...)
		f.P()
		f.doc("", []string{
			"The keywords protoschema-jsonschema writes that reject values protovalidate accepts, by",
			"$defs id and field. Each entry names the rule or shape that makes its keyword wrong.",
		})
		switch fl {
		case flavorDTS:
			f.P("export declare const fixesTable: Fixes;")
		case flavorTS:
			f.P("export const fixesTable: Fixes = {")
			fixesTable(f, ctx)
		default:
			f.P("export const fixesTable = {")
			fixesTable(f, ctx)
		}
		f.P()
		f.doc("", []string{
			"Copies a protoschema-jsonschema bundle without the keywords in fixesTable. The import is",
			"never mutated: every importer of a JSON module shares the same object.",
		})
		switch fl {
		case flavorDTS:
			f.P("export declare function loosen<T>(bundle: T): T;")
		case flavorTS:
			f.P("export function loosen<T>(bundle: T): T {")
			f.P("  return loosenWith(fixesTable, bundle);")
			f.P("}")
		default:
			f.P("export function loosen(bundle) {")
			f.P("  return loosenWith(fixesTable, bundle);")
			f.P("}")
		}
		write(gen, name, f)
	}
	return nil
}

// fixesTable prints the body of the fixesTable literal, closing brace included.
func fixesTable(f *tsFile, ctx *Context) {
	for _, name := range ctx.reachable() {
		entries := jsonEntries(ctx.messages[name])
		if len(entries) == 0 {
			continue
		}
		f.P("  ", tsString(name+defSuffix), ": {")
		for _, e := range entries {
			for _, reason := range e.reasons {
				f.P("    // ", reason)
			}
			alias := ""
			if e.alias != "" {
				alias = "alias: " + tsString(e.alias) + ", "
			}
			f.P("    ", tsString(e.key), ": { ", alias, "fixes: [", strings.Join(e.fixes, ", "), "] },")
		}
		f.P("  },")
	}
	f.P("};")
}

// jsonEntry is one row of fixesTable: a field of a def, or its root.
type jsonEntry struct {
	key     string   // JSON name, or "" for the def itself
	alias   string   // proto name, where it differs from the JSON name
	fixes   []string // rendered Fix literals
	reasons []string
}

func jsonEntries(msg parser.MessageMetadata) []jsonEntry {
	var out []jsonEntry
	if plan, ok := wktFixes[msg.Name]; ok {
		out = append(out, jsonEntry{fixes: renderFixes(plan.fixes), reasons: fixReasons(plan.fixes)})
	}
	// Upstream writes the well-known types by hand, not from their fields.
	if isWKT(msg.Name) {
		return out
	}
	for _, field := range msg.Fields {
		plan := planJSON(field)
		if len(plan.fixes) == 0 {
			continue
		}
		entry := jsonEntry{key: field.JSONName, fixes: renderFixes(plan.fixes), reasons: fixReasons(plan.fixes)}
		if field.Name != field.JSONName {
			entry.alias = field.Name
		}
		out = append(out, entry)
	}
	return out
}

func renderFixes(fixes []jsonFix) []string {
	out := make([]string, 0, len(fixes))
	for _, fix := range fixes {
		quoted := make([]string, len(fix.drop))
		for i, keyword := range fix.drop {
			quoted[i] = tsString(keyword)
		}
		at := ""
		if fix.at != atField {
			at = "at: " + tsString(fix.at) + ", "
		}
		out = append(out, "{ "+at+"drop: ["+strings.Join(quoted, ", ")+"] }")
	}
	return out
}

func fixReasons(fixes []jsonFix) []string {
	var out []string
	for _, fix := range fixes {
		for _, reason := range fix.reasons {
			if !slices.Contains(out, reason) {
				out = append(out, reason)
			}
		}
	}
	return out
}

// reachable lists, sorted, every message a generated file's bundle can carry
// as a def: each top-level message and whatever its fields reach.
func (c *Context) reachable() []string {
	seen := map[string]bool{}
	var walk func(name string)
	walk = func(name string) {
		msg, ok := c.messages[name]
		if !ok || seen[name] {
			return
		}
		seen[name] = true
		for _, field := range msg.Fields {
			for _, protoType := range []string{field.ProtoType, field.MapValue} {
				if kind, target, ok := referenceTarget(protoType); ok && kind == "message" {
					walk(target)
				}
			}
		}
	}
	for name, file := range c.fileOf {
		if c.generated[file] && c.topLevel(name) {
			walk(name)
		}
	}
	return slices.Sorted(func(yield func(string) bool) {
		for name := range seen {
			if !yield(name) {
				return
			}
		}
	})
}

// generatedLine opens every module the plugin writes.
const generatedLine = "// Code generated by protoc-gen-strict-schema. DO NOT EDIT."

// generatedHeader is the first lines of every module the plugin writes.
func generatedHeader(source string) []string {
	lines := []string{generatedLine}
	if source != "" {
		lines = append(lines, "// source: "+source)
	}
	return lines
}

// runtimeLines is a hand-written module's body, without the header the
// writer prints anyway: the sources carry it so they read as what they emit.
func runtimeLines(source string) []string {
	source = strings.TrimLeft(strings.TrimPrefix(source, generatedLine), "\n")
	return strings.Split(strings.TrimRight(source, "\n"), "\n")
}

// write registers a module with the response.
func write(gen *protogen.Plugin, name string, f *tsFile) {
	g := gen.NewGeneratedFile(name, "")
	_, _ = g.Write([]byte(f.render())) // a GeneratedFile buffers in memory and never fails
}
