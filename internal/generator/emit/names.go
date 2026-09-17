package emit

import "strings"

// Identifier conversions shared by the emitters. Each mirrors a naming decision
// the official generator made, so the overlay names the same symbols.

// Camel is protobuf-es's protoCamelCase: underscores capitalise the letter that
// follows, and a digit clears that, so `port_2_name` is `port2name`.
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
func Pascal(name string) string { return upperFirst(Camel(name)) }

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

func upperFirst(name string) string {
	if name == "" {
		return name
	}
	return strings.ToUpper(name[:1]) + name[1:]
}
