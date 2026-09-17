// Package generator parses the files the plugin was asked to generate and hands
// them to one emitter per target: tsgen for TypeScript, pygen for Python,
// oapigen for the protoc-gen-openapiv2 configuration. Nothing here re-declares a
// proto type; each emitter derives its types from, or imports them out of, the
// generated file next to its output.
package generator

import (
	"google.golang.org/protobuf/compiler/protogen"

	"github.com/d0whc3r/protoc-gen-strict/internal/generator/oapigen"
	"github.com/d0whc3r/protoc-gen-strict/internal/generator/pygen"
	"github.com/d0whc3r/protoc-gen-strict/internal/generator/tsgen"
	"github.com/d0whc3r/protoc-gen-strict/internal/parser"
)

// Run parses every file marked for generation and emits one file per selected
// language. It lives here, not in main, so the golden tests take the same
// path.
func Run(gen *protogen.Plugin, opts Options) error {
	// gen.Files holds every transitive import; only the ones on the command
	// line have Generate set, and only those are parsed.
	parsed := map[string][]parser.MessageMetadata{}
	for _, file := range gen.Files {
		if !file.Generate {
			continue
		}
		messages, err := parser.ParseFile(file)
		if err != nil {
			return err
		}
		parsed[file.Desc.Path()] = messages
	}

	// Resolution spans every file in the request, not only the parsed ones,
	// because a narrowing reaches across files (see tsgen.Context).
	var ts *tsgen.Context
	if opts.TypeScript() {
		ts = tsgen.New(gen.Files, parsed)
	}

	// The OpenAPI configuration is one file keyed by fully qualified names, so
	// it collects across the whole run rather than being written per file.
	var openAPI []parser.MessageMetadata

	for _, file := range gen.Files {
		if !file.Generate {
			continue
		}
		messages := parsed[file.Desc.Path()]
		if len(messages) == 0 && len(file.Services) == 0 {
			continue
		}
		if opts.TypeScript() {
			tsgen.Write(gen, file, ts)
		}
		if opts.Python() {
			pygen.WriteFile(gen, file, messages)
		}
		if opts.OpenAPI() {
			openAPI = append(openAPI, messages...)
		}
	}

	// The shared module belongs to the TypeScript overlay.
	if opts.TypeScript() {
		tsgen.WriteRuntime(gen)
	}
	if opts.OpenAPI() {
		oapigen.WriteConfig(gen, openAPI)
	}
	return nil
}
