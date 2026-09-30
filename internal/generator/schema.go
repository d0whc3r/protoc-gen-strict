package generator

import (
	"fmt"
	"strings"

	"google.golang.org/protobuf/compiler/protogen"

	"github.com/d0whc3r/protoc-gen-strict/internal/generator/schemagen"
	"github.com/d0whc3r/protoc-gen-strict/internal/parser"
)

// The `target=` values protoc-gen-strict-schema accepts, joined with `+` the
// way protoschema-jsonschema joins its own.
const (
	targetJSON = "json"
	targetZod  = "zod"
	targetZod3 = "zod3"
)

// The `output=` values, joined with `+`: protoc-gen-es's `target` values, so
// the two plugin entries can say the same thing.
const (
	outputTS  = "ts"
	outputJS  = "js"
	outputDTS = "dts"
)

// The `import_extension=` values, protoc-gen-es's.
const (
	importExtensionJS   = "js"
	importExtensionNone = "none"
)

// SchemaOptions are protoc-gen-strict-schema's parameters, on top of
// protogen's.
type SchemaOptions struct {
	json bool
	zod4 bool // target=zod
	zod3 bool // target=zod3
	set  bool // a target was named at all

	output    schemagen.Output
	outputSet bool   // an output was named at all
	plugin    string // jsonschema_plugin=, "" for schemagen.DefaultJSONSchemaPlugin
}

// Set records one parameter:
//
//   - `target=`: json, zod or zod3, joined with +. Omitting it selects
//     json+zod. zod and zod3 write differently named modules, so both fit.
//   - `output=`: ts, js or dts, joined with +. Omitting it selects ts.
//   - `import_extension=`: js or none, the extension relative imports carry.
//   - `jsonschema_plugin=`: the protoc-gen-jsonschema executable target=json
//     runs.
//
// Anything else fails the generation rather than quietly emitting nothing.
func (o *SchemaOptions) Set(name, value string) error {
	switch name {
	case "target":
		return o.setTarget(value)
	case "output":
		return o.setOutput(value)
	case "import_extension":
		switch value {
		case importExtensionJS:
			o.output.ImportExt = ".js"
		case importExtensionNone:
			o.output.ImportExt = ""
		default:
			return fmt.Errorf("import_extension must be js or none, got %q", value)
		}
		return nil
	case "jsonschema_plugin":
		if value == "" {
			return fmt.Errorf("jsonschema_plugin needs an executable")
		}
		o.plugin = value
		return nil
	}
	return fmt.Errorf("unknown option %q", name)
}

func (o *SchemaOptions) setTarget(value string) error {
	o.set = true
	for _, target := range strings.Split(value, "+") {
		switch target {
		case targetJSON:
			o.json = true
		case targetZod:
			o.zod4 = true
		case targetZod3:
			o.zod3 = true
		default:
			return fmt.Errorf("target must be json, zod or zod3, joined with +; got %q", target)
		}
	}
	return nil
}

func (o *SchemaOptions) setOutput(value string) error {
	o.outputSet = true
	for _, output := range strings.Split(value, "+") {
		switch output {
		case outputTS:
			o.output.TS = true
		case outputJS:
			o.output.JS = true
		case outputDTS:
			o.output.DTS = true
		default:
			return fmt.Errorf("output must be ts, js or dts, joined with +; got %q", output)
		}
	}
	return nil
}

// JSON reports whether to emit the JSON Schema modules.
func (o SchemaOptions) JSON() bool { return o.json || !o.set }

// Zod reports the Zod majors to emit for, none when no Zod target was named.
func (o SchemaOptions) Zod() []schemagen.ZodMajor {
	if !o.set {
		return []schemagen.ZodMajor{schemagen.Zod4}
	}
	var majors []schemagen.ZodMajor
	if o.zod4 {
		majors = append(majors, schemagen.Zod4)
	}
	if o.zod3 {
		majors = append(majors, schemagen.Zod3)
	}
	return majors
}

// Output reports what each module is written as: TypeScript unless output=
// says otherwise.
func (o SchemaOptions) Output() schemagen.Output {
	out := o.output
	if !o.outputSet {
		out.TS = true
	}
	return out
}

// JSONSchemaPlugin is the protoc-gen-jsonschema executable target=json runs.
func (o SchemaOptions) JSONSchemaPlugin() string {
	if o.plugin == "" {
		return schemagen.DefaultJSONSchemaPlugin
	}
	return o.plugin
}

// RunSchema is protoc-gen-strict-schema's generation. Unlike Run it parses
// every file in the request: a JSON Schema bundle carries the defs of the
// messages it imports, and a Zod field of an imported enum is written out from
// the enum's members. jsonSchema runs protoc-gen-jsonschema for target=json;
// main passes schemagen.ExecJSONSchema, the golden tests a fake.
func RunSchema(gen *protogen.Plugin, opts SchemaOptions, jsonSchema schemagen.JSONSchemaPlugin) error {
	parsed := map[string][]parser.MessageMetadata{}
	enums := map[string][]parser.EnumMetadata{}
	for _, file := range gen.Files {
		messages, err := parser.ParseFile(file)
		if err != nil {
			return fmt.Errorf("parse %s: %w", file.Desc.Path(), err)
		}
		parsed[file.Desc.Path()] = messages
		enums[file.Desc.Path()] = parser.ParseEnums(file)
	}
	ctx := schemagen.New(gen.Files, parsed, enums)
	out, majors := opts.Output(), opts.Zod()

	// The bundles first: a .schema module is only written once the bundle it
	// imports is known to exist.
	if opts.JSON() {
		if err := schemagen.WriteBundles(gen, ctx, opts.JSONSchemaPlugin(), jsonSchema); err != nil {
			return fmt.Errorf("target=json: %w", err)
		}
	}

	for _, file := range gen.Files {
		if !ctx.Generates(file.Desc.Path()) {
			continue
		}
		if opts.JSON() {
			schemagen.WriteJSONSchema(gen, file, ctx, out)
		}
		for _, major := range majors {
			schemagen.WriteZod(gen, file, ctx, major, out)
		}
	}

	// The shared modules, once for the run.
	if opts.JSON() {
		if err := schemagen.WriteJSONRuntime(gen, ctx, out); err != nil {
			return err
		}
	}
	if len(majors) > 0 {
		return schemagen.WriteZodRuntime(gen, majors, out)
	}
	return nil
}
