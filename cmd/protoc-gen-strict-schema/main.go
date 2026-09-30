// Command protoc-gen-strict-schema reads protovalidate (buf.validate) rules and
// emits runtime schemas that enforce them: JSON Schema modules built on
// protoschema-jsonschema's output, and Zod schemas generated from the rules,
// with protovalidate checking what Zod cannot say exactly.
//
// `target=json`, `target=zod` or `target=zod3`, joined with `+`, picks the
// schemas; omitting it emits json+zod. `output=ts|js|dts` picks the files each
// module is written as, `import_extension=js|none` how they import each other.
// target=json runs protoc-gen-jsonschema, which has to be on PATH, or named by
// `jsonschema_plugin=`.
//
// protoc/buf drive it over stdin/stdout; never run it directly. `--version` is
// the one thing a human types at it.
package main

import (
	"fmt"
	"os"
	"path/filepath"

	"google.golang.org/protobuf/compiler/protogen"
	"google.golang.org/protobuf/types/pluginpb"

	"github.com/d0whc3r/protoc-gen-strict/internal/generator"
	"github.com/d0whc3r/protoc-gen-strict/internal/generator/schemagen"
)

// version is stamped by the release build; see .goreleaser.yaml. A source
// build reports "dev", which is what tells a bug report the two apart.
var version = "dev"

func main() {
	// protoc and buf invoke the plugin with no arguments and talk over stdin,
	// so this is the only flag, and it has to be handled before protogen reads
	// the request that is not coming.
	if len(os.Args) == 2 && (os.Args[1] == "--version" || os.Args[1] == "-version") {
		fmt.Printf("%s %s\n", filepath.Base(os.Args[0]), version)
		return
	}

	var opts generator.SchemaOptions
	protogen.Options{ParamFunc: opts.Set}.Run(func(gen *protogen.Plugin) error {
		// Proto3 optional fields are common in validated schemas.
		gen.SupportedFeatures = uint64(pluginpb.CodeGeneratorResponse_FEATURE_PROTO3_OPTIONAL)
		return generator.RunSchema(gen, opts, schemagen.ExecJSONSchema)
	})
}
