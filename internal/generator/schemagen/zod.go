package schemagen

import (
	"path"
	"slices"
	"strings"

	"google.golang.org/protobuf/compiler/protogen"

	"github.com/d0whc3r/protoc-gen-strict/internal/generator/emit"
	"github.com/d0whc3r/protoc-gen-strict/internal/generator/tscode"
	"github.com/d0whc3r/protoc-gen-strict/internal/parser"
)

// The shared modules every <file>.zod module imports, emitted once per run,
// extensionless. strict/protovalidate imports no zod, so both majors share it;
// the well-known-type module is one per major, named with its suffix.
const (
	protovalidateModule = "strict/protovalidate"
	wktModule           = "strict/wkt" // + the major's suffix, e.g. "strict/wkt.zod3"
	zodModule           = "zod"
	zod3Module          = "zod/v3" // zod@^3.25 and zod@^4 both ship Zod 3 here
)

// zodImport is the module the major's z comes from. The Zod 3 modules take
// zod/v3, so they resolve in a project that has Zod 4 installed as zod.
func (m ZodMajor) zodImport() string {
	if m == Zod3 {
		return zod3Module
	}
	return zodModule
}

// wktSchemas are the well-known types strict/wkt.zod declares a schema for,
// named <name>Zod. A well-known message outside this list has none.
var wktSchemas = map[string]bool{
	"Timestamp": true, "Duration": true, "FieldMask": true,
	"Struct": true, "Value": true, "ListValue": true, "Any": true, "Empty": true,
	"BoolValue": true, "BytesValue": true, "DoubleValue": true, "FloatValue": true,
	"Int32Value": true, "Int64Value": true, "StringValue": true,
	"UInt32Value": true, "UInt64Value": true,
	"NullValue": true,
}

// isWKT reports whether a well-known type has a schema in strict/wkt.zod.
func isWKT(fullName string) bool {
	name, ok := strings.CutPrefix(fullName, wktPackage)
	return ok && wktSchemas[name]
}

// zodWriter writes one <file>.zod module in one flavor.
type zodWriter struct {
	f      *tsFile
	ctx    *Context
	major  ZodMajor
	flavor flavor
	proto  string         // this file's proto path
	cycle  map[string]int // message -> id of the cycle it belongs to, for messages in one
	cur    int            // cycle id of the message being written, 0 outside one
}

// WriteZod writes <file>.zod.ts, <file>.zod3.ts for Zod 3, or its .js and
// .d.ts: an <enum>Zod per
// enum, and a <message>ZodObject and <message>Zod per message, nested ones
// included.
func WriteZod(gen *protogen.Plugin, file *protogen.File, ctx *Context, major ZodMajor, out Output) {
	proto := file.Desc.Path()
	messages, enums := ctx.byFile[proto], ctx.enumsOf[proto]
	if len(messages) == 0 && len(enums) == 0 {
		return
	}
	order, cycle := emissionOrder(messages)

	for _, fl := range out.flavors() {
		name := emit.OutputPrefix(proto) + major.suffix() + fl.extension()
		w := &zodWriter{f: newTSFile(name, out.ImportExt), ctx: ctx, major: major, flavor: fl, proto: proto, cycle: cycle}
		w.f.header = generatedHeader(proto)

		// Declarations first, so no import takes their names.
		for _, enum := range enums {
			w.f.declare(ctx.valueName(enum.Name) + "Zod")
		}
		for _, msg := range order {
			w.f.declare(ctx.valueName(msg.Name) + "ZodObject")
			w.f.declare(ctx.valueName(msg.Name) + "Zod")
		}

		// So fromJson can decode a google.protobuf.Any that packs a type of
		// this file, whichever module's schema holds the Any.
		if fl != flavorDTS {
			w.f.P(w.helper("register"), "(", w.fileDescriptor(), ");")
			w.f.P()
		}

		for _, enum := range enums {
			w.enum(enum)
		}
		for _, msg := range order {
			w.message(msg)
		}

		// A cycle declares a base or a shape it does not export, and a .d.ts
		// without an export {} exports every declaration.
		if fl == flavorDTS && len(cycle) > 0 {
			w.f.P("export {};")
		}
		write(gen, name, w.f)
	}
}

// WriteZodRuntime writes the shared modules of the Zod majors a run emits, in
// every flavor out asks for: strict/protovalidate once, and a well-known-type
// module per major.
func WriteZodRuntime(gen *protogen.Plugin, majors []ZodMajor, out Output) error {
	type module struct{ name, source string }
	modules := []module{{protovalidateModule, "protovalidate"}}
	for _, major := range majors {
		source := "wkt.zod4"
		if major == Zod3 {
			source = "wkt.zod3"
		}
		modules = append(modules, module{wktModule + major.suffix(), source})
	}

	for _, fl := range out.flavors() {
		for _, module := range modules {
			source, err := runtimeSource(module.source, fl)
			if err != nil {
				return err
			}
			name := module.name + fl.extension()
			f := newTSFile(name, out.ImportExt)
			f.header = generatedHeader("")
			f.lines = runtimeLines(source)
			write(gen, name, f)
		}
	}
	return nil
}

func (w *zodWriter) z() string { return w.f.value(w.major.zodImport(), "z") }

// zt names a Zod type, e.g. zt("ZodString") is "z.ZodString".
func (w *zodWriter) zt(name string) string { return w.z() + "." + name }

// helper binds field, message or register from strict/protovalidate.
func (w *zodWriter) helper(name string) string {
	return w.f.value(tscode.RelImport(w.f.out, protovalidateModule), name)
}

// descriptor binds the <Message>Schema protoc-gen-es declares.
func (w *zodWriter) descriptor(fullName string) string {
	file := w.ctx.fileOf[fullName]
	module := "./" + path.Base(strings.TrimSuffix(file, ".proto")) + esSuffix
	if file != w.proto {
		module = tscode.RelImport(w.f.out, strings.TrimSuffix(file, ".proto")+esSuffix)
	}
	return w.f.value(module, w.ctx.schemaSymbol(fullName))
}

// fileDescriptor binds the file_<path> descriptor protoc-gen-es declares for
// this file.
func (w *zodWriter) fileDescriptor() string {
	module := "./" + path.Base(strings.TrimSuffix(w.proto, ".proto")) + esSuffix
	return w.f.value(module, fileSymbol(w.proto))
}

// fileSymbol is protoc-gen-es's name for a file's descriptor: "file_" and the
// path without ".proto", each run of characters outside [A-Za-z0-9_] one "_".
func fileSymbol(proto string) string {
	var b strings.Builder
	b.WriteString("file_")
	inRun := false
	for _, c := range strings.TrimSuffix(proto, ".proto") {
		if c == '_' || (c >= 'a' && c <= 'z') || (c >= 'A' && c <= 'Z') || (c >= '0' && c <= '9') {
			b.WriteRune(c)
			inRun = false
			continue
		}
		if !inRun {
			b.WriteByte('_')
		}
		inRun = true
	}
	return b.String()
}

// messageRef is the schema a message-typed value uses, its type, and the
// message it names when there is one: a well-known type's, a <message>Zod of
// this or another generated file, or z.unknown() for a message nobody
// generated.
func (w *zodWriter) messageRef(fullName string) (string, string, string) {
	if isWKT(fullName) {
		local := w.wkt(fullName)
		return local, "typeof " + local, fullName
	}
	file, ok := w.ctx.fileOf[fullName]
	switch {
	case !ok || !w.ctx.generated[file]:
		return w.z() + ".unknown()", w.zt("ZodUnknown"), ""
	case file == w.proto && w.major == Zod3 && w.inCycle(fullName):
		// Zod 3 infers no type through a getter; a lazy typed by the
		// structural shape keeps the field's type exact. JavaScript has no
		// annotation to carry.
		shape, full := w.f.declare(w.ctx.ident(fullName)+"Shape"), w.f.declare(w.ctx.valueName(fullName)+"Zod")
		typ := w.zt("ZodLazy") + "<" + w.zt("ZodType") + "<" + shape + ">>"
		if w.flavor == flavorJS {
			return w.z() + ".lazy(() => " + full + ")", typ, fullName
		}
		return w.z() + ".lazy((): " + w.zt("ZodType") + "<" + shape + "> => " + full + ")", typ, fullName
	case file == w.proto:
		local := w.f.declare(w.ctx.valueName(fullName) + "Zod")
		return local, "typeof " + local, fullName
	}
	module := tscode.RelImport(w.f.out, strings.TrimSuffix(file, ".proto")+w.major.suffix())
	local := w.f.value(module, w.ctx.valueName(fullName)+"Zod")
	return local, "typeof " + local, fullName
}

// enumRef is the schema an enum-typed value uses, and its type: its <enum>Zod
// where one is generated, and otherwise its member names written out, since
// every file of the request was parsed.
func (w *zodWriter) enumRef(fullName string) (string, string) {
	if isWKT(fullName) {
		local := w.wkt(fullName)
		return local, "typeof " + local
	}
	file, ok := w.ctx.fileOf[fullName]
	switch {
	case !ok:
		return w.z() + ".string()", w.zt("ZodString")
	case file == w.proto:
		local := w.f.declare(w.ctx.valueName(fullName) + "Zod")
		return local, "typeof " + local
	case w.ctx.generated[file]:
		local := w.f.value(tscode.RelImport(w.f.out, strings.TrimSuffix(file, ".proto")+w.major.suffix()), w.ctx.valueName(fullName)+"Zod")
		return local, "typeof " + local
	}
	names := enumNames(w.ctx.enums[fullName])
	return w.enumExpr(names), w.enumType(names)
}

func (w *zodWriter) wkt(fullName string) string {
	return w.f.value(tscode.RelImport(w.f.out, wktModule+w.major.suffix()), emit.LowerFirst(strings.TrimPrefix(fullName, wktPackage))+"Zod")
}

// enumNames lists an enum's member names in declaration order.
func enumNames(enum parser.EnumMetadata) []string {
	names := make([]string, 0, len(enum.Values))
	for _, value := range enum.Values {
		names = append(names, value.Name)
	}
	return names
}

// enumExpr is the Zod schema of a set of names; none is z.never().
func (w *zodWriter) enumExpr(names []string) string {
	if len(names) == 0 {
		return w.z() + ".never()"
	}
	quoted := make([]string, len(names))
	for i, name := range names {
		quoted[i] = tsString(name)
	}
	return w.z() + ".enum([" + strings.Join(quoted, ", ") + "])"
}

// enumType is the type of enumExpr: Zod 4 keys the enum by its values, Zod 3
// keeps the tuple.
func (w *zodWriter) enumType(names []string) string {
	if len(names) == 0 {
		return w.zt("ZodNever")
	}
	parts := make([]string, len(names))
	for i, name := range names {
		parts[i] = tsString(name)
		if w.major == Zod4 {
			parts[i] += ": " + tsString(name)
		}
	}
	if w.major == Zod4 {
		return w.zt("ZodEnum") + "<{ " + strings.Join(parts, "; ") + " }>"
	}
	return w.zt("ZodEnum") + "<[" + strings.Join(parts, ", ") + "]>"
}

// inCycle reports whether a referenced message belongs to the cycle of the
// message being written, which is what needs a deferred reference.
func (w *zodWriter) inCycle(fullName string) bool {
	return w.cur != 0 && fullName != "" && w.cycle[fullName] == w.cur
}

func (w *zodWriter) enum(enum parser.EnumMetadata) {
	name := w.f.declare(w.ctx.valueName(enum.Name) + "Zod")
	lines := commentLines(enum.Comments)
	lines = append(lines, "", enum.Name+", as protojson writes it: the member's name.")
	w.f.doc("", lines)
	names := enumNames(enum)
	if w.flavor == flavorDTS {
		w.f.P("export declare const ", name, ": ", w.enumType(names), ";")
	} else {
		w.f.P("export const ", name, " = ", w.enumExpr(names), ";")
	}
	w.f.P()
}

func (w *zodWriter) message(msg parser.MessageMetadata) {
	stem := w.ctx.valueName(msg.Name)
	object := w.f.declare(stem + "ZodObject")
	full := w.f.declare(stem + "Zod")
	w.cur = w.cycle[msg.Name]

	lines := commentLines(msg.Comments)
	lines = append(lines, "",
		"The composable schema of "+msg.Name+": every field carries its own rules, so",
		"`"+object+".shape.<field>`, `.pick()`, `.extend()` and `.partial()` keep validating.",
		"The rules on the message as a whole are in "+full+".",
	)

	fields := make([]zodField, len(msg.Fields))
	for i, field := range msg.Fields {
		fields[i] = w.buildField(field)
	}
	if w.flavor == flavorDTS {
		w.objectType(msg, object, lines, fields)
	} else {
		w.objectDecl(msg, object, lines, fields)
	}

	reasons := messageReasons(msg)
	doc := []string{"Full validation of " + msg.Name + ": the message has no rule beyond its fields'."}
	if len(reasons) > 0 {
		doc = []string{"Full validation of " + msg.Name + ". Adds what protovalidate checks on the whole message:"}
		for _, reason := range reasons {
			doc = append(doc, "  "+reason)
		}
	}
	w.f.doc("", doc)
	switch {
	case w.flavor == flavorDTS && len(reasons) > 0 && w.major == Zod3:
		w.f.P("export declare const ", full, ": ", w.zt("ZodEffects"), "<typeof ", object, ">;")
	case w.flavor == flavorDTS:
		// Zod 4's superRefine returns the schema it refines.
		w.f.P("export declare const ", full, ": typeof ", object, ";")
	case len(reasons) > 0:
		w.f.P("export const ", full, " = ", object, ".superRefine(", w.helper("message"), "(", w.descriptor(msg.Name), "));")
	default:
		w.f.P("export const ", full, " = ", object, ";")
	}
	w.f.P()
}

// objectDecl prints <message>ZodObject. A message in a cycle defers the fields
// that name the cycle. Zod 4 takes a getter, which keeps the object and its
// inferred type. Zod 3 infers nothing through a getter, so the other fields go
// in a base object, spread next to the deferred ones as a z.lazy, and a
// structural <Message>Shape types the lazies; JavaScript needs no Shape. Spread,
// not .extend(), so the inferred shape is the flat one the .d.ts declares.
func (w *zodWriter) objectDecl(msg parser.MessageMetadata, object string, doc []string, fields []zodField) {
	z := w.z()
	if w.cur == 0 || w.major == Zod4 {
		w.f.doc("", doc)
		w.f.P("export const ", object, " = ", z, ".strictObject({")
		for i, field := range msg.Fields {
			w.property(msg, field, fields[i])
		}
		w.f.P("});")
		w.f.P()
		return
	}

	base := w.f.declare(w.ctx.valueName(msg.Name) + "Base")
	if !slices.ContainsFunc(fields, func(zf zodField) bool { return !zf.recursive }) {
		w.f.P("const ", base, " = ", z, ".strictObject({});")
	} else {
		w.f.P("const ", base, " = ", z, ".strictObject({")
		for i, field := range msg.Fields {
			if !fields[i].recursive {
				w.property(msg, field, fields[i])
			}
		}
		w.f.P("});")
	}
	w.f.P()
	if w.flavor == flavorTS {
		w.shapeDecl(msg, base, fields)
	}

	w.f.doc("", doc)
	w.f.P("export const ", object, " = ", z, ".strictObject({")
	w.f.P("  ...", base, ".shape,")
	for i, field := range msg.Fields {
		if fields[i].recursive {
			w.property(msg, field, fields[i])
		}
	}
	w.f.P("});")
	w.f.P()
}

// shapeDecl prints the structural type a Zod 3 cycle's lazies are typed by:
// the base object's input, and the fields that name the cycle.
func (w *zodWriter) shapeDecl(msg parser.MessageMetadata, base string, fields []zodField) {
	var members []string
	for i, field := range msg.Fields {
		if fields[i].recursive {
			members = append(members, propertyKey(field.JSONName)+optionalMark(fields[i])+": "+w.shapeType(field))
		}
	}
	shape := w.f.declare(w.ctx.ident(msg.Name) + "Shape")
	w.f.P("type ", shape, " = ", w.z(), ".input<typeof ", base, "> & { ", strings.Join(members, "; "), " };")
	w.f.P()
}

// objectType prints the declaration of <message>ZodObject in a .d.ts: the
// type TypeScript infers for objectDecl's value. A Zod 4 cycle names its shape,
// since a declaration cannot refer to its own type through typeof; a Zod 3
// cycle declares its base and shape as the .ts does.
func (w *zodWriter) objectType(msg parser.MessageMetadata, object string, doc []string, fields []zodField) {
	strict := w.zt("core") + ".$strict"
	if w.major == Zod3 {
		strict = `"strict"`
	}
	// members prints one object type's properties, each with its doc.
	members := func(keep func(zodField) bool) {
		for i, field := range msg.Fields {
			if keep(fields[i]) {
				w.f.doc("  ", fieldDoc(field, fields[i]))
				w.f.P("  ", propertyKey(field.JSONName), ": ", w.propertyType(fields[i]), ";")
			}
		}
	}
	every := func(zodField) bool { return true }

	switch {
	case w.cur != 0 && w.major == Zod4:
		shape := w.f.declare(w.ctx.ident(msg.Name) + "ZodShape")
		w.f.P("type ", shape, " = {")
		members(every)
		w.f.P("};")
		w.f.P()
		w.f.doc("", doc)
		w.f.P("export declare const ", object, ": ", w.zt("ZodObject"), "<", shape, ", ", strict, ">;")
		w.f.P()
		return
	case w.cur != 0:
		base := w.f.declare(w.ctx.valueName(msg.Name) + "Base")
		w.f.P("declare const ", base, ": ", w.zt("ZodObject"), "<{")
		members(func(zf zodField) bool { return !zf.recursive })
		w.f.P("}, ", strict, ">;")
		w.f.P()
		w.shapeDecl(msg, base, fields)
	}
	w.f.doc("", doc)
	w.f.P("export declare const ", object, ": ", w.zt("ZodObject"), "<{")
	members(every)
	w.f.P("}, ", strict, ">;")
	w.f.P()
}

// fieldExpr is a field's schema as the object literal holds it: with field()
// when protovalidate checks a rule or the schema is z.unknown(), and optional
// unless the key is mandatory.
func (w *zodWriter) fieldExpr(msg parser.MessageMetadata, field parser.FieldMetadata, zf zodField) string {
	expr := zf.expr
	if zf.refined() {
		native := ""
		if len(zf.native) > 0 {
			quoted := make([]string, len(zf.native))
			for j, kind := range zf.native {
				quoted[j] = tsString(kind)
			}
			native = ", [" + strings.Join(quoted, ", ") + "]"
		}
		expr += ".superRefine(" + w.helper("field") + "(" + w.descriptor(msg.Name) + ", " + tsString(field.Name) + native + "))"
	}
	if !zf.required {
		expr += ".optional()"
	}
	return expr
}

// propertyType is the type of fieldExpr: Zod 3's superRefine wraps the schema
// in a ZodEffects, Zod 4's returns it.
func (w *zodWriter) propertyType(zf zodField) string {
	typ := zf.typ
	if zf.refined() && w.major == Zod3 {
		typ = w.zt("ZodEffects") + "<" + typ + ">"
	}
	if !zf.required {
		typ = w.zt("ZodOptional") + "<" + typ + ">"
	}
	return typ
}

// property prints one field of an object literal, with its doc.
func (w *zodWriter) property(msg parser.MessageMetadata, field parser.FieldMetadata, zf zodField) {
	w.f.doc("  ", fieldDoc(field, zf))
	expr := w.fieldExpr(msg, field, zf)
	key := propertyKey(field.JSONName)
	if zf.recursive && w.major == Zod4 {
		w.f.P("  get ", key, "() {")
		w.f.P("    return ", expr, ";")
		w.f.P("  },")
		return
	}
	w.f.P("  ", key, ": ", expr, ",")
}

// shapeType is the TypeScript type of a field that names a message of the
// cycle: the message's shape, a list of it, or a map to it.
func (w *zodWriter) shapeType(field parser.FieldMetadata) string {
	protoType := field.ProtoType
	if field.IsMap {
		protoType = field.MapValue
	}
	_, target, _ := referenceTarget(protoType)
	shape := w.f.declare(w.ctx.ident(target) + "Shape")
	switch {
	case field.IsMap:
		return "{ [key: string]: " + shape + " }"
	case field.Repeated:
		return shape + "[]"
	}
	return shape
}

func optionalMark(zf zodField) string {
	if zf.required {
		return ""
	}
	return "?"
}

// fieldDoc lists a field's rules and who enforces each.
func fieldDoc(field parser.FieldMetadata, zf zodField) []string {
	lines := emit.RuleComments(field)
	if len(lines) == 0 && zf.note == "" {
		return nil
	}
	lines = append(lines, "")
	if len(zf.native) > 0 {
		lines = append(lines, "Carried by Zod: "+strings.Join(zf.native, ", ")+".")
	}
	if len(zf.checked) > 0 {
		lines = append(lines, "Checked by protovalidate: "+strings.Join(zf.checked, ", ")+".")
	}
	if zf.note != "" {
		lines = append(lines, zf.note)
	}
	return lines
}

// messageReasons names what <message>Zod's refinement adds over the object:
// message CEL, oneof exclusivity, and the rules of fields that protojson
// omits at their zero value, which the object never sees.
func messageReasons(msg parser.MessageMetadata) []string {
	var reasons []string
	for _, rule := range msg.CEL {
		reasons = append(reasons, "cel["+emit.CELLabel(rule)+"]")
	}
	for _, oneof := range msg.Oneofs {
		reasons = append(reasons, emit.OneofLine(oneof))
	}
	if absent := absentChecked(msg); len(absent) > 0 {
		reasons = append(reasons, "the rules of "+strings.Join(absent, ", ")+" when absent: protojson omits a zero value")
	}
	return reasons
}

// absentChecked lists the implicit fields whose rules still apply when the key
// is absent: protojson omits a zero value, and protovalidate validates it. A
// mandatory key already rejects absence; `ignore` exempts the zero value; and
// the rules of list items or map entries never see an empty list or map.
func absentChecked(msg parser.MessageMetadata) []string {
	var out []string
	for _, field := range msg.Fields {
		if field.Presence || field.Required || field.Ignore != "" {
			continue
		}
		constrained := len(field.CEL) > 0
		for _, rule := range field.Rules {
			nested := strings.HasPrefix(rule.Kind, emit.RepeatedItems) ||
				strings.HasPrefix(rule.Kind, "map.keys.") || strings.HasPrefix(rule.Kind, "map.values.")
			if !nested && !isNoop(rule) && leafOf(rule.Kind) != emit.IgnoreRule {
				constrained = true
			}
		}
		if constrained {
			out = append(out, field.Name)
		}
	}
	return out
}

// propertyKey is a JSON name as an object key: bare when it is an identifier,
// quoted otherwise. json_name admits any string.
func propertyKey(name string) string {
	if name == "" {
		return `""`
	}
	for i, c := range name {
		identChar := c == '_' || c == '$' || (c >= 'a' && c <= 'z') || (c >= 'A' && c <= 'Z')
		if !identChar && (i == 0 || c < '0' || c > '9') {
			return tsString(name)
		}
	}
	return name
}

// commentLines turns a proto comment into doc lines: one, collapsed, the way
// the overlays print it.
func commentLines(comment string) []string {
	if comment == "" {
		return nil
	}
	return []string{emit.OneLine(comment)}
}

// emissionOrder sorts a file's messages so each follows the messages its fields
// name, and reports which ones sit in a cycle. Tarjan's algorithm yields the
// strongly connected components dependencies first; declaration order breaks
// ties, so the output is stable.
func emissionOrder(messages []parser.MessageMetadata) ([]parser.MessageMetadata, map[string]int) {
	byName := map[string]parser.MessageMetadata{}
	for _, msg := range messages {
		byName[msg.Name] = msg
	}
	edges := func(msg parser.MessageMetadata) []string {
		var out []string
		for _, field := range msg.Fields {
			// z.unknown() stands for a message protovalidate never evaluates.
			if uncheckedValue(field) {
				continue
			}
			for _, protoType := range []string{field.ProtoType, field.MapValue} {
				if kind, target, ok := referenceTarget(protoType); ok && kind == "message" {
					if _, local := byName[target]; local {
						out = append(out, target)
					}
				}
			}
		}
		return out
	}

	index, low := map[string]int{}, map[string]int{}
	onStack := map[string]bool{}
	var stack []string
	var order []parser.MessageMetadata
	cycle := map[string]int{}
	next, cycles := 0, 0

	var visit func(name string)
	visit = func(name string) {
		index[name], low[name] = next, next
		next++
		stack = append(stack, name)
		onStack[name] = true
		selfLoop := false
		for _, target := range edges(byName[name]) {
			if target == name {
				selfLoop = true
			}
			if _, seen := index[target]; !seen {
				visit(target)
				low[name] = min(low[name], low[target])
			} else if onStack[target] {
				low[name] = min(low[name], index[target])
			}
		}
		if low[name] != index[name] {
			return
		}
		var component []string
		for {
			top := stack[len(stack)-1]
			stack = stack[:len(stack)-1]
			onStack[top] = false
			component = append(component, top)
			if top == name {
				break
			}
		}
		if len(component) > 1 || selfLoop {
			cycles++
			for _, member := range component {
				cycle[member] = cycles
			}
		}
		// Within a component, keep declaration order.
		for _, msg := range messages {
			for _, member := range component {
				if msg.Name == member {
					order = append(order, msg)
				}
			}
		}
	}
	for _, msg := range messages {
		if _, seen := index[msg.Name]; !seen {
			visit(msg.Name)
		}
	}
	return order, cycle
}
