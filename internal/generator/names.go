package generator

import "strings"

// Identifier conversions shared by the emitters. Each mirrors a naming decision
// the official generator made, so the overlay names the same symbols.

// tsIdent is the identifier protoc-gen-es exports for a proto name: no package,
// nesting joined by underscores.
func tsIdent(pkg, fullName string) string {
	return strings.ReplaceAll(strings.TrimPrefix(fullName, pkg+"."), ".", "_")
}

// camel is the property name protoc-gen-es gives a proto snake_case field.
//
// ponytail: protobuf-es also escapes names that collide with JS reserved words
// by appending `$`. Add that mapping if a schema ever hits one.
func camel(name string) string {
	parts := strings.Split(name, "_")
	for i := 1; i < len(parts); i++ {
		parts[i] = upperFirst(parts[i])
	}
	return strings.Join(parts, "")
}

// pascal is camel with the first letter upper, for an alias fragment.
//
// ponytail: `User.address` and a message named `UserAddress` would both want the
// name `UserAddress`. Disambiguate only if a schema collides.
func pascal(name string) string { return upperFirst(camel(name)) }

func upperFirst(name string) string {
	if name == "" {
		return name
	}
	return strings.ToUpper(name[:1]) + name[1:]
}

func lowerFirst(name string) string {
	if name == "" {
		return name
	}
	return strings.ToLower(name[:1]) + name[1:]
}
