// Package generator turns parsed metadata into strict overlays on what the
// official plugins emit: protoc-gen-es for TypeScript, protoc-gen-python for
// Python. Nothing here re-declares a proto type: every type is imported from,
// or derived from, the generated file next to the output.
package generator

import (
	"google.golang.org/protobuf/compiler/protogen"

	"github.com/d0whc3r/protoc-gen-strict/internal/parser"
)

// Run parses every file marked for generation and emits one file per selected
// language. It lives here, not in main, so the golden tests take the same
// path.
func Run(gen *protogen.Plugin, opts Options) error {
	// gen.Files holds every transitive import; only the ones on the command
	// line have Generate set. Resolution spans all of them, because a narrowing
	// reaches across files (see Context).
	ctx, err := buildContext(gen.Files)
	if err != nil {
		return err
	}

	// The OpenAPI configuration is one file keyed by fully qualified names, so
	// it collects across the whole run rather than being written per file.
	var openAPI []parser.MessageMetadata

	for _, file := range gen.Files {
		if !file.Generate {
			continue
		}
		messages := ctx.byFile[file.Desc.Path()]
		if len(messages) == 0 && len(file.Services) == 0 {
			continue
		}
		if opts.TypeScript() {
			writeTypeScript(gen, file, ctx)
		}
		if opts.Python() {
			writePython(gen, file, messages)
		}
		if opts.OpenAPI() {
			openAPI = append(openAPI, messages...)
		}
	}

	// The shared module belongs to the TypeScript overlay.
	if opts.TypeScript() {
		generateStrictTypes(gen)
	}
	if opts.OpenAPI() {
		writeOpenAPIConfig(gen, openAPI)
	}
	return nil
}
