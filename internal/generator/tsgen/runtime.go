package tsgen

import (
	_ "embed"
	"strings"

	"google.golang.org/protobuf/compiler/protogen"
)

// strictTypesFile is the shared module every overlay imports, emitted once per
// generation. That needs `strategy: all` in buf.gen.yaml, since the default
// directory sharding runs the plugin once per directory and each run would emit
// its own copy.
const strictTypesFile = "strict/types.ts"

// A real .ts file so it stays readable and lintable; nothing in it depends on
// the schema.
//
//go:embed strict_types.ts
var strictTypesSource string

// WriteRuntime emits the shared module. Called once, after every file.
func WriteRuntime(gen *protogen.Plugin) {
	gen.NewGeneratedFile(strictTypesFile, "").P(strings.TrimRight(strictTypesSource, "\n"))
}
