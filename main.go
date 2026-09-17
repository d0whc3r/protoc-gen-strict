// Command protoc-gen-strict reads protovalidate (buf.validate) rules, CEL
// expressions included, and emits a strict overlay on the types the official
// generators produce.
//
// `lang=typescript`, `lang=python` or `lang=openapi` picks one, so each output
// lands in the tree it belongs to; omitting it emits every one.
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

	var opts generator.Options
	protogen.Options{ParamFunc: opts.Set}.Run(func(gen *protogen.Plugin) error {
		// Proto3 optional fields are common in validated schemas.
		gen.SupportedFeatures = uint64(pluginpb.CodeGeneratorResponse_FEATURE_PROTO3_OPTIONAL)
		return generator.Run(gen, opts)
	})
}
