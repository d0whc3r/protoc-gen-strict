package tsgen

import (
	"strings"

	"github.com/d0whc3r/protoc-gen-strict/internal/generator/emit"
)

// The identifiers protoc-gen-es declares. The overlay indexes into its output,
// so a name derived differently is either a syntax error or a property that
// does not exist.

// tsIdent is the identifier protoc-gen-es exports for a proto name: no package,
// nesting joined by underscores.
func tsIdent(pkg, fullName string) string {
	return strings.ReplaceAll(strings.TrimPrefix(fullName, pkg+"."), ".", "_")
}

// jsReserved are the property names protobuf-es escapes with a trailing `$`,
// because every JavaScript object already carries them. See safeObjectProperty
// in @bufbuild/protobuf/reflect/names.
var jsReserved = map[string]bool{
	"constructor": true,
	"toString":    true,
	"toJSON":      true,
	"valueOf":     true,
}

// localName is the property protoc-gen-es declares for a proto field or oneof.
//
// It is protobuf-es's localName, not the JSON name: an explicit
// `[json_name = "external-id"]` renames the wire encoding and leaves the
// property alone, so reading it off the JSON name would name a property that
// does not exist — or one that is not an identifier at all.
func localName(protoName string) string { return escaped(emit.Camel(protoName)) }

// methodName is the property protoc-gen-es gives an RPC on a service
// descriptor: the name with a lower first letter, and no other conversion.
func methodName(protoName string) string { return escaped(emit.LowerFirst(protoName)) }

func escaped(name string) string {
	if jsReserved[name] {
		return name + "$"
	}
	return name
}
