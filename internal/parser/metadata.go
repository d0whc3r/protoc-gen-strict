package parser

import (
	"fmt"
	"strings"

	"google.golang.org/protobuf/reflect/protoreflect"
)

// Rule is one standard buf.validate constraint, flattened to a dotted name and
// its textual value, e.g. {Kind: "string.min_len", Value: "3"}.
type Rule struct {
	Kind  string
	Value string
}

// TypeRef locates where a message or enum is declared: the proto file, and the
// name relative to that file's package ("Warehouse.Address" when nested).
type TypeRef struct {
	File string
	Name string
}

// FieldMetadata is one proto field plus every validation rule on it.
type FieldMetadata struct {
	Name       string  // proto field name, e.g. "user_id"
	ProtoType  string  // "string", "int32", "message:example.v1.Address", "enum:..."
	Type       TypeRef // declaring file + package-relative name, empty for scalars
	Repeated   bool
	IsMap      bool
	MapKey     string  // key type, set only when IsMap
	MapValue   string  // value type, set only when IsMap
	MapValType TypeRef // Type, but for MapValue
	Optional   bool    // explicit proto3 `optional`
	Required   bool    // (buf.validate.field).required
	OneofName  string  // enclosing real oneof, empty when the field is not in one
	Rules      []Rule
	CEL        []CELRule
}

// OneofMetadata is a set of mutually exclusive fields: a real proto `oneof`, or
// a `(buf.validate.message).oneof` rule, which names its fields and has no
// oneof declaration.
type OneofMetadata struct {
	Name     string   // real oneof name; empty for a message-level oneof rule
	Fields   []string // member field names
	Required bool     // exactly one member must be set
}

// MessageMetadata groups the parsed fields of a single proto message.
type MessageMetadata struct {
	Name     string // fully qualified proto name
	Fields   []FieldMetadata
	Oneofs   []OneofMetadata
	CEL      []CELRule // (buf.validate.message).cel rules, which span several fields
	Comments string
}

// typeRef resolves where a field's type is declared. Scalars need no import, so
// they yield the zero TypeRef.
func typeRef(desc protoreflect.FieldDescriptor) TypeRef {
	switch desc.Kind() {
	case protoreflect.MessageKind, protoreflect.GroupKind:
		if desc.IsMap() {
			return TypeRef{}
		}
		return refTo(desc.Message())
	case protoreflect.EnumKind:
		return refTo(desc.Enum())
	default:
		return TypeRef{}
	}
}

func refTo(desc protoreflect.Descriptor) TypeRef {
	file := desc.ParentFile()
	return TypeRef{
		File: file.Path(),
		Name: strings.TrimPrefix(string(desc.FullName()), string(file.Package())+"."),
	}
}

// protoTypeName renders the field's proto type. Messages and enums keep their
// fully qualified name so generators can emit a reference.
func protoTypeName(desc protoreflect.FieldDescriptor) string {
	switch desc.Kind() {
	case protoreflect.MessageKind, protoreflect.GroupKind:
		if desc.IsMap() {
			return fmt.Sprintf("map<%s, %s>",
				protoTypeName(desc.MapKey()), protoTypeName(desc.MapValue()))
		}
		return "message:" + string(desc.Message().FullName())
	case protoreflect.EnumKind:
		return "enum:" + string(desc.Enum().FullName())
	default:
		return desc.Kind().String()
	}
}
