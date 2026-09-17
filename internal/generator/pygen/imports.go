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
// import each needs. A type this file declares is imported by name; the rest go
// through a module alias, as protoc-gen-pyi does.
type pyImports struct {
	self    string
	symbols map[string]bool   // names an alias references, from the sibling _pb2 module
	modules map[string]string // proto path -> module alias
	typing  map[string]bool   // names from typing
	abc     map[string]bool   // names from collections.abc

	annotated map[string]bool // constructors from annotated_types
}

func newPyImports(self string) *pyImports {
	return &pyImports{
		self:    self,
		symbols: map[string]bool{},
		modules: map[string]string{},
		typing:  map[string]bool{},
		abc:     map[string]bool{},

		annotated: map[string]bool{},
	}
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

// lines renders the import block: PEP 8's three groups — standard library,
// third party, generated modules — blank-separated, with the empty ones left
// out.
func (p *pyImports) lines() []string {
	var stdlib []string
	if len(p.abc) > 0 {
		stdlib = append(stdlib, "from collections.abc import "+strings.Join(slices.Sorted(maps.Keys(p.abc)), ", "))
	}
	if len(p.typing) > 0 {
		stdlib = append(stdlib, "from typing import "+strings.Join(slices.Sorted(maps.Keys(p.typing)), ", "))
	}

	var thirdParty []string
	if len(p.annotated) > 0 {
		thirdParty = append(thirdParty, "from "+annotatedTypesModule+" import "+strings.Join(slices.Sorted(maps.Keys(p.annotated)), ", "))
	}

	var generated []string
	for _, file := range slices.Sorted(maps.Keys(p.modules)) {
		module, alias := pyModule(file), p.modules[file]
		// A proto at the import root is a top-level module with no package to
		// import it from: "user.proto" is "user_pb2".
		dot := strings.LastIndex(module, ".")
		if dot < 0 {
			generated = append(generated, "import "+module+" as "+alias)
			continue
		}
		generated = append(generated, "from "+module[:dot]+" import "+module[dot+1:]+" as "+alias)
	}
	// Only the types an alias names. The overlay does not re-export the message
	// classes: they stay in the module protoc-gen-python wrote them in, and a
	// blanket re-export would import every message in the file to be read by
	// nothing.
	if names := slices.Sorted(maps.Keys(p.symbols)); len(names) > 0 {
		generated = append(generated, "from "+pyModule(p.self)+" import "+strings.Join(names, ", "))
	}

	var out []string
	for _, group := range [][]string{stdlib, thirdParty, generated} {
		if len(group) == 0 {
			continue
		}
		if len(out) > 0 {
			out = append(out, "")
		}
		out = append(out, group...)
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
