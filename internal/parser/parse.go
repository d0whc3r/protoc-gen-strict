// Package parser extracts protovalidate (buf.validate) rules from proto
// descriptors and flattens them into a language-agnostic intermediate
// representation that the generators consume.
package parser

import (
	"fmt"
	"strings"

	validate "buf.build/gen/go/bufbuild/protovalidate/protocolbuffers/go/buf/validate"
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

func parseMessage(msg *protogen.Message) (MessageMetadata, error) {
	out := MessageMetadata{
		Name:     string(msg.Desc.FullName()),
		Comments: strings.TrimSpace(string(msg.Comments.Leading)),
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
		Name:      string(desc.Name()),
		ProtoType: protoTypeName(desc),
		Type:      typeRef(desc),
		Repeated:  desc.IsList(),
		IsMap:     desc.IsMap(),
		Optional:  desc.HasOptionalKeyword(),
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

func oneofRules(desc protoreflect.OneofDescriptor) *validate.OneofRules {
	opts, _ := desc.Options().(*descriptorpb.OneofOptions)
	if !proto.HasExtension(opts, validate.E_Oneof) {
		return nil
	}
	rules, _ := proto.GetExtension(opts, validate.E_Oneof).(*validate.OneofRules)
	return rules
}
