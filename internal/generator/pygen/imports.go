package pygen

import (
	"maps"
	"slices"
	"strings"

	"github.com/d0whc3r/protoc-gen-strict/internal/parser"
)

// pyScalars maps proto scalars to the types protoc-gen-pyi declares.
var pyScalars = map[string]string{
	"string": "str", "bool": "bool", "bytes": "bytes",
	"double": "float", "float": "float",
	"int32": "int", "sint32": "int", "sfixed32": "int",
	"uint32": "int", "fixed32": "int",
	"int64": "int", "sint64": "int", "sfixed64": "int",
	"uint64": "int", "fixed64": "int",
}

// pyImports resolves proto type references to Python names and records the
// import each needs. Symbols from this file are imported by name so they are
// re-exported; the rest go through a module alias, as protoc-gen-pyi does.
type pyImports struct {
	self    string
	symbols map[string]bool   // top-level names taken from the sibling _pb2 module
	modules map[string]string // proto path -> module alias
	typing  map[string]bool   // names from typing
	abc     map[string]bool   // names from collections.abc
}

// typeOf renders the Python type of a field, ignoring the rules attached to it.
func (p *pyImports) typeOf(field parser.FieldMetadata) string {
	p.typing["Annotated"] = true
	switch {
	case field.IsMap:
		p.abc["Mapping"] = true
		return "Mapping[" + p.base(field.MapKey, parser.TypeRef{}) + ", " + p.base(field.MapValue, field.MapValType) + "]"
	case field.Repeated:
		// protoc-gen-pyi declares these as Repeated{Scalar,Composite}FieldContainer,
		// both MutableSequence subclasses.
		p.abc["Sequence"] = true
		return "Sequence[" + p.base(field.ProtoType, field.Type) + "]"
	default:
		return p.base(field.ProtoType, field.Type)
	}
}

func (p *pyImports) base(protoType string, ref parser.TypeRef) string {
	if ref.Name != "" {
		if ref.File == p.self {
			p.symbols[topLevel(ref.Name)] = true
			return ref.Name
		}
		alias := pyAliasFor(ref.File)
		p.modules[ref.File] = alias
		return alias + "." + ref.Name
	}
	if mapped, ok := pyScalars[protoType]; ok {
		return mapped
	}
	p.typing["Any"] = true
	return "Any"
}

func (p *pyImports) lines() []string {
	var out []string
	if len(p.abc) > 0 {
		out = append(out, "from collections.abc import "+strings.Join(slices.Sorted(maps.Keys(p.abc)), ", "))
	}
	if len(p.typing) > 0 {
		out = append(out, "from typing import "+strings.Join(slices.Sorted(maps.Keys(p.typing)), ", "))
	}
	if len(out) > 0 {
		out = append(out, "")
	}

	for _, file := range slices.Sorted(maps.Keys(p.modules)) {
		module, alias := pyModule(file), p.modules[file]
		// A proto at the import root is a top-level module with no package to
		// import it from: "user.proto" is "user_pb2".
		dot := strings.LastIndex(module, ".")
		if dot < 0 {
			out = append(out, "import "+module+" as "+alias)
			continue
		}
		out = append(out, "from "+module[:dot]+" import "+module[dot+1:]+" as "+alias)
	}

	names := slices.Sorted(maps.Keys(p.symbols))
	if len(names) > 0 {
		// `X as X` marks a deliberate re-export, which type checkers require
		// before a caller may import it from here.
		out = append(out, "from "+pyModule(p.self)+" import (")
		for _, name := range names {
			out = append(out, "    "+name+" as "+name+",")
		}
		out = append(out, ")")
	}
	return out
}

// topLevel is the outermost name, which is what the generated module exports:
// "Warehouse.Address" is nested in "Warehouse" and has no export of its own.
func topLevel(relative string) string {
	top, _, _ := strings.Cut(relative, ".")
	return top
}

// pyModule turns a proto path into the module protoc-gen-python writes for it.
func pyModule(protoPath string) string {
	return strings.ReplaceAll(strings.TrimSuffix(protoPath, ".proto"), "/", ".") + "_pb2"
}

// pyAliasFor names the local alias an imported module is bound to. It carries
// the whole proto path, not the base name protoc-gen-pyi uses: `a/common.proto`
// and `b/common.proto` both want `_common_pb2`, and the second import would
// rebind the name, silently retyping every field annotated with the first.
func pyAliasFor(protoPath string) string {
	return "_" + pyIdent(strings.TrimSuffix(protoPath, ".proto")) + "_pb2"
}

// pyIdent maps the characters a proto path allows but a Python identifier does
// not onto underscores.
func pyIdent(protoPath string) string {
	return strings.Map(func(r rune) rune {
		switch {
		case r >= 'a' && r <= 'z', r >= 'A' && r <= 'Z', r >= '0' && r <= '9':
			return r
		default:
			return '_'
		}
	}, protoPath)
}
