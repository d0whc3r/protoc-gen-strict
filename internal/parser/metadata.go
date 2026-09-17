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
	OutputOnly bool    // (google.api.field_behavior) = OUTPUT_ONLY
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

	// Every server-assigned field reachable from this message, as a dotted proto
	// path, e.g. "product.created_at". Depth-first in declaration order.
	OutputOnlyPaths []string
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

// EnumMetadata is one proto enum declaration. No buf.validate rule hangs off an
// enum, but the member protobuf numbers 0 is the "unset" one by convention —
// buf lint's ENUM_ZERO_VALUE_SUFFIX names it <ENUM>_UNSPECIFIED — so the
// generators rule it out of the enum's strict type.
type EnumMetadata struct {
	Name     string      // fully qualified proto name, e.g. "shop.inventory.v1.StockMovementKind"
	Type     TypeRef     // declaring file + package-relative name
	Comments string      // leading comment on the enum declaration
	Values   []EnumValue // in declaration order
}

// EnumValue is one declared member of an enum.
type EnumValue struct {
	Name   string // proto member name, e.g. "STOCK_MOVEMENT_KIND_UNSPECIFIED"
	Number int32
}

// Zero returns the member numbered 0. Proto3 requires one, but an enum reached
// through a proto2 descriptor or an import need not have it, and there is
// nothing to exclude then.
func (e EnumMetadata) Zero() (EnumValue, bool) {
	for _, value := range e.Values {
		if value.Number == 0 {
			return value, true
		}
	}
	return EnumValue{}, false
}

// AllAboveZero reports whether every member other than the zero one is
// positive, which is what makes "at least 1" the same set as "not the zero
// member". A proto may number a member below zero; then it is not.
func (e EnumMetadata) AllAboveZero() bool {
	above := false
	for _, value := range e.Values {
		if value.Number < 0 {
			return false
		}
		above = above || value.Number > 0
	}
	return above
}
