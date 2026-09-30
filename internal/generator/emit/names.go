package emit

import "strings"

// Camel is protobuf-es's protoCamelCase: underscores capitalise the letter that
// follows, and a digit clears that, so `metric_1st` is `metric1st`.
func Camel(name string) string {
	var b strings.Builder
	capNext := false
	for i := 0; i < len(name); i++ {
		c := name[i]
		switch {
		case c == '_':
			capNext = true
		case c >= '0' && c <= '9':
			b.WriteByte(c)
			capNext = false
		default:
			if capNext {
				c = upperASCII(c)
				capNext = false
			}
			b.WriteByte(c)
		}
	}
	return b.String()
}

// Pascal is Camel with the first letter upper, for an alias fragment.
//
// ponytail: `User.address` and a message named `UserAddress` would both want the
// name `UserAddress`. Disambiguate only if a schema collides.
func Pascal(name string) string {
	name = Camel(name)
	if name == "" {
		return name
	}
	return strings.ToUpper(name[:1]) + name[1:]
}

// LowerFirst lowercases the first letter and leaves the rest alone.
func LowerFirst(name string) string {
	if name == "" {
		return name
	}
	return strings.ToLower(name[:1]) + name[1:]
}

func upperASCII(c byte) byte {
	if c >= 'a' && c <= 'z' {
		return c - ('a' - 'A')
	}
	return c
}

// OutputPrefix is where a proto file's outputs go, extensionless: the file's
// own path without ".proto", e.g. "example/v1/user". protoc-gen-es and
// protoc-gen-python write there whatever `paths=` says. protogen's
// GeneratedFilenamePrefix follows the Go import path instead under the default
// `paths=import`, which would put an overlay under github.com/..., away from the
// module it imports and from the strict/ modules at the root.
func OutputPrefix(protoPath string) string { return strings.TrimSuffix(protoPath, ".proto") }
