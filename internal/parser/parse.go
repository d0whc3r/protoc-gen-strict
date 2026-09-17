// Package parser extracts protovalidate (buf.validate) rules from proto
// descriptors and flattens them into a language-agnostic intermediate
// representation that the generators consume.
package parser

import (
	"fmt"
	"slices"
	"strings"

	validate "buf.build/gen/go/bufbuild/protovalidate/protocolbuffers/go/buf/validate"
	"google.golang.org/genproto/googleapis/api/annotations"
	"google.golang.org/protobuf/compiler/protogen"
	"google.golang.org/protobuf/proto"
	"google.golang.org/protobuf/reflect/protoreflect"
	"google.golang.org/protobuf/types/descriptorpb"
)

// ParseFile walks every message in the file (including nested ones) and
// returns their metadata. Map entry messages are synthetic and skipped.
func ParseFile(file *protogen.File) ([]MessageMetadata, error) {
	var out []MessageMetadata
	var walk func(msgs []*protogen.Message) error
	walk = func(msgs []*protogen.Message) error {
		for _, msg := range msgs {
			if msg.Desc.IsMapEntry() {
				continue
			}
			parsed, err := parseMessage(msg)
			if err != nil {
				return err
			}
			out = append(out, parsed)
			if err := walk(msg.Messages); err != nil {
				return err
			}
		}
		return nil
	}
	if err := walk(file.Messages); err != nil {
		return nil, err
	}
	return out, nil
}

// ParseEnums walks every enum the file declares, nested ones included.
func ParseEnums(file *protogen.File) []EnumMetadata {
	var out []EnumMetadata
	var walk func(enums []*protogen.Enum, msgs []*protogen.Message)
	walk = func(enums []*protogen.Enum, msgs []*protogen.Message) {
		for _, enum := range enums {
			out = append(out, parseEnum(enum))
		}
		for _, msg := range msgs {
			walk(msg.Enums, msg.Messages)
		}
	}
	walk(file.Enums, file.Messages)
	return out
}

func parseEnum(enum *protogen.Enum) EnumMetadata {
	out := EnumMetadata{
		Name:     string(enum.Desc.FullName()),
		Type:     refTo(enum.Desc),
		Comments: strings.TrimSpace(string(enum.Comments.Leading)),
	}
	for _, value := range enum.Values {
		out.Values = append(out.Values, EnumValue{
			Name:   string(value.Desc.Name()),
			Number: int32(value.Desc.Number()),
		})
	}
	return out
}

func parseMessage(msg *protogen.Message) (MessageMetadata, error) {
	out := MessageMetadata{
		Name:            string(msg.Desc.FullName()),
		Comments:        strings.TrimSpace(string(msg.Comments.Leading)),
		OutputOnlyPaths: outputOnlyPaths(msg.Desc),
	}
	for _, field := range msg.Fields {
		meta, err := parseField(field)
		if err != nil {
			return out, fmt.Errorf("%s: %w", field.Desc.FullName(), err)
		}
		out.Fields = append(out.Fields, meta)
	}

	// Message-level CEL rules validate across fields, e.g. "end must be after
	// start", so they hang off the message rather than any single field.
	rules := messageRules(msg.Desc)
	msgCEL, err := celRulesFrom(msg.Desc, rules.GetCel(), rules.GetCelExpression())
	if err != nil {
		return out, fmt.Errorf("%s: %w", msg.Desc.FullName(), err)
	}
	out.CEL = msgCEL

	for _, oneof := range msg.Oneofs {
		// proto3 `optional` is implemented as a synthetic one-field oneof;
		// it carries no user intent, so skip it.
		if oneof.Desc.IsSynthetic() {
			continue
		}
		meta := OneofMetadata{
			Name:     string(oneof.Desc.Name()),
			Required: oneofRules(oneof.Desc).GetRequired(),
		}
		for _, field := range oneof.Fields {
			meta.Fields = append(meta.Fields, string(field.Desc.Name()))
		}
		out.Oneofs = append(out.Oneofs, meta)
	}

	// `(buf.validate.message).oneof` declares exclusivity over fields that are
	// not in a real oneof; surface it the same way.
	for _, rule := range rules.GetOneof() {
		out.Oneofs = append(out.Oneofs, OneofMetadata{
			Fields:   rule.GetFields(),
			Required: rule.GetRequired(),
		})
	}
	return out, nil
}

// parseField reads the buf.validate.field extension off a field and flattens
// it. A field without options, or without the extension, yields metadata with
// empty Rules and CEL slices rather than an error.
func parseField(field *protogen.Field) (FieldMetadata, error) {
	desc := field.Desc
	meta := FieldMetadata{
		Name:       string(desc.Name()),
		ProtoType:  protoTypeName(desc),
		Type:       typeRef(desc),
		Repeated:   desc.IsList(),
		IsMap:      desc.IsMap(),
		Optional:   desc.HasOptionalKeyword(),
		OutputOnly: outputOnly(desc),
	}
	if desc.IsMap() {
		meta.MapKey = protoTypeName(desc.MapKey())
		meta.MapValue = protoTypeName(desc.MapValue())
		meta.MapValType = typeRef(desc.MapValue())
	}
	if oneof := desc.ContainingOneof(); oneof != nil && !oneof.IsSynthetic() {
		meta.OneofName = string(oneof.Name())
	}

	rules := fieldRules(desc)
	if rules == nil {
		return meta, nil
	}

	meta.Required = rules.GetRequired()
	meta.Rules = standardRules(rules)

	// A field rule roots `this` at the field, not at a message, so there is no
	// path to resolve and nothing to translate.
	parsed, err := celRulesFrom(nil, rules.GetCel(), rules.GetCelExpression())
	if err != nil {
		return meta, err
	}
	meta.CEL = parsed
	return meta, nil
}

// The three readers below pull a buf.validate extension off a descriptor's
// options. Options and rules are both routinely absent, so nil is a normal
// result rather than an error: a failed type assertion leaves a typed-nil
// pointer, which proto.HasExtension reports as unpopulated rather than
// panicking.

func fieldRules(desc protoreflect.FieldDescriptor) *validate.FieldRules {
	opts, _ := desc.Options().(*descriptorpb.FieldOptions)
	if !proto.HasExtension(opts, validate.E_Field) {
		return nil
	}
	rules, _ := proto.GetExtension(opts, validate.E_Field).(*validate.FieldRules)
	return rules
}

func messageRules(desc protoreflect.MessageDescriptor) *validate.MessageRules {
	opts, _ := desc.Options().(*descriptorpb.MessageOptions)
	if !proto.HasExtension(opts, validate.E_Message) {
		return nil
	}
	rules, _ := proto.GetExtension(opts, validate.E_Message).(*validate.MessageRules)
	return rules
}

// outputOnly reports whether the field is one the server assigns. It is not a
// buf.validate rule: google.api.field_behavior is the AIP-203 annotation, and
// OUTPUT_ONLY is the value saying a caller must not set the field.
func outputOnly(desc protoreflect.FieldDescriptor) bool {
	opts, _ := desc.Options().(*descriptorpb.FieldOptions)
	if !proto.HasExtension(opts, annotations.E_FieldBehavior) {
		return false
	}
	behaviors, _ := proto.GetExtension(opts, annotations.E_FieldBehavior).([]annotations.FieldBehavior)
	return slices.Contains(behaviors, annotations.FieldBehavior_OUTPUT_ONLY)
}

// outputOnlyPaths lists every server-assigned field reachable from the message
// as a dotted proto path, the form a google.protobuf.FieldMask carries. A field
// under an OUTPUT_ONLY message field is server-assigned too — the whole subtree
// is — and a writable message field is walked all the same, so a nested
// server-assigned field surfaces as "product.id".
//
// A repeated or map field is never walked into: a FieldMask path may not
// continue past one. Neither is a message already on the path, which is what
// terminates a self-referential message and google.protobuf.Struct.
func outputOnlyPaths(desc protoreflect.MessageDescriptor) []string {
	var out []string

	var walk func(md protoreflect.MessageDescriptor, prefix string, assigned bool, seen []protoreflect.FullName)
	walk = func(md protoreflect.MessageDescriptor, prefix string, assigned bool, seen []protoreflect.FullName) {
		fields := md.Fields()
		for i := range fields.Len() {
			field := fields.Get(i)
			path := prefix + string(field.Name())
			serverAssigned := assigned || outputOnly(field)
			if serverAssigned {
				out = append(out, path)
			}

			child := field.Message()
			if child == nil || field.IsList() || field.IsMap() || slices.Contains(seen, child.FullName()) {
				continue
			}
			walk(child, path+".", serverAssigned, append(seen, child.FullName()))
		}
	}

	walk(desc, "", false, []protoreflect.FullName{desc.FullName()})
	return out
}

func oneofRules(desc protoreflect.OneofDescriptor) *validate.OneofRules {
	opts, _ := desc.Options().(*descriptorpb.OneofOptions)
	if !proto.HasExtension(opts, validate.E_Oneof) {
		return nil
	}
	rules, _ := proto.GetExtension(opts, validate.E_Oneof).(*validate.OneofRules)
	return rules
}
