package parser

import (
	"slices"
	"strings"
	"testing"

	"google.golang.org/protobuf/compiler/protogen"
	"google.golang.org/protobuf/proto"
	"google.golang.org/protobuf/types/pluginpb"
)

// TestEnumZero covers the member a strict enum type excludes. Proto3 requires
// one numbered 0, but a descriptor reached another way need not have it, and
// then there is nothing to exclude.
func TestEnumZero(t *testing.T) {
	withZero := EnumMetadata{Values: []EnumValue{
		{Name: "FLAVOR_UNSPECIFIED", Number: 0},
		{Name: "FLAVOR_SWEET", Number: 1},
	}}
	zero, ok := withZero.Zero()
	if !ok || zero.Name != "FLAVOR_UNSPECIFIED" {
		t.Errorf("Zero() = %q, %v; want FLAVOR_UNSPECIFIED, true", zero.Name, ok)
	}

	withoutZero := EnumMetadata{Values: []EnumValue{{Name: "FLAVOR_SWEET", Number: 1}}}
	if _, ok := withoutZero.Zero(); ok {
		t.Error("Zero() reported a member for an enum that declares none at 0")
	}
}

// TestEnumAllAboveZero covers the check that makes "at least 1" the same set as
// "not the zero member", which is the only form the Python overlay can carry.
func TestEnumAllAboveZero(t *testing.T) {
	tests := []struct {
		name   string
		values []EnumValue
		want   bool
	}{
		{
			name:   "zero and positives",
			values: []EnumValue{{Number: 0}, {Number: 1}, {Number: 7}},
			want:   true,
		},
		{
			name:   "a member below zero",
			values: []EnumValue{{Number: -1}, {Number: 0}, {Number: 1}},
			want:   false,
		},
		{
			name:   "nothing but the zero",
			values: []EnumValue{{Number: 0}},
			want:   false,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := (EnumMetadata{Values: tt.values}).AllAboveZero(); got != tt.want {
				t.Errorf("AllAboveZero() = %v, want %v", got, tt.want)
			}
		})
	}
}

// TestOutputOnly covers the google.api.field_behavior read. The extension is
// resolved through the global registry, so the test runs against the real
// descriptor set rather than a hand-built option: a missing import would leave
// the annotation in unknown fields and every field would read as writable.
func TestOutputOnly(t *testing.T) {
	msg := loadMessage(t, "shop.catalog.v1.Product")
	want := map[string]bool{
		"id": true, "created_by_email": true, "created_at": true,
		"updated_at": true, "published_at": true, "version": true,
	}
	fields := msg.Fields()
	for i := range fields.Len() {
		field := fields.Get(i)
		name := string(field.Name())
		if got := outputOnly(field); got != want[name] {
			t.Errorf("outputOnly(%s) = %v, want %v", name, got, want[name])
		}
	}
}

// TestOutputOnlyPaths covers the dotted paths <Message>OutputOnlyFields is built
// from. CreateProductRequest declares no OUTPUT_ONLY field of its own: every
// path it has comes from the Product it wraps, which is the case the flat
// per-field read misses.
func TestOutputOnlyPaths(t *testing.T) {
	want := []string{
		"product.id",
		"product.created_by_email",
		"product.created_at", "product.created_at.seconds", "product.created_at.nanos",
		"product.updated_at", "product.updated_at.seconds", "product.updated_at.nanos",
		"product.published_at", "product.published_at.seconds", "product.published_at.nanos",
		"product.version",
	}
	got := outputOnlyPaths(loadMessage(t, "shop.catalog.v1.CreateProductRequest"))
	if !slices.Equal(got, want) {
		t.Errorf("outputOnlyPaths() =\n%q\nwant\n%q", got, want)
	}

	// A repeated or map field ends a path: Product.tags and Product.labels are
	// not something a FieldMask can name a member of.
	for _, path := range outputOnlyPaths(loadMessage(t, "shop.catalog.v1.Product")) {
		if strings.HasPrefix(path, "tags.") || strings.HasPrefix(path, "labels.") {
			t.Errorf("outputOnlyPaths() walked into a repeated or map field: %q", path)
		}
	}
}

// TestListRuleValues covers Rule.Values. The joined Value reads the same for
// ["a, b"] and ["a", "b"], so a generator that emits the members — a Zod enum —
// has to have them one by one.
func TestListRuleValues(t *testing.T) {
	fields := loadMessage(t, "shop.coverage.v1.StringRuleCoverage").Fields()
	rules := standardRules(fieldRules(fields.ByName("enumerated")))

	want := map[string][]string{
		"string.in":     {"alpha", "beta", "stable"},
		"string.not_in": {"deprecated"},
	}
	for _, rule := range rules {
		if got := rule.Values; !slices.Equal(got, want[rule.Kind]) {
			t.Errorf("%s: Values = %q, want %q", rule.Kind, got, want[rule.Kind])
		}
	}
	if len(rules) != len(want) {
		t.Errorf("got %d rules, want %d", len(rules), len(want))
	}

	// A scalar rule has no members to list.
	constant := standardRules(fieldRules(fields.ByName("constant")))
	if len(constant) != 1 || constant[0].Values != nil {
		t.Errorf("string.const: Values = %q, want nil", constant[0].Values)
	}
}

// TestFieldResolution covers the field facts ParseFile resolves from the
// descriptor rather than copies from the rules: the protojson name, which
// json_name overrides; presence; and the ignore protovalidate applies, which a
// member of a (buf.validate.message).oneof gets without setting it.
func TestFieldResolution(t *testing.T) {
	messages := parseFixture(t, "shop/schema/v1/schema.proto")
	field := func(message, name string) FieldMetadata {
		t.Helper()
		for _, msg := range messages {
			if msg.Name != message {
				continue
			}
			for _, f := range msg.Fields {
				if f.Name == name {
					return f
				}
			}
		}
		t.Fatalf("no field %s.%s", message, name)
		return FieldMetadata{}
	}

	for _, tt := range []struct {
		message, field string
		jsonName       string
		presence       bool
		ignore         string
	}{
		{"shop.schema.v1.JsonNameCoverage", "external_id", "extId", false, ""},
		{"shop.schema.v1.JsonNameCoverage", "metric_1st", "metric1st", false, ""},
		{"shop.schema.v1.StringContentCoverage", "nickname", "nickname", true, ""},
		{"shop.schema.v1.ImplicitCoverage", "required_item", "requiredItem", true, ""},
		{"shop.schema.v1.OneofCoverage", "email", "email", true, ""},
		{"shop.schema.v1.IgnoreCoverage", "email", "email", false, "IGNORE_IF_ZERO_VALUE"},
		{"shop.schema.v1.IgnoreCoverage", "never_checked", "neverChecked", false, "IGNORE_ALWAYS"},
		{"shop.schema.v1.MessageOneofCoverage", "code", "code", false, "IGNORE_IF_ZERO_VALUE"},
		{"shop.schema.v1.MessageOneofCoverage", "tags", "tags", false, "IGNORE_IF_ZERO_VALUE"},
		{"shop.schema.v1.MessageOneofCoverage", "alias", "alias", false, "IGNORE_ALWAYS"},
	} {
		got := field(tt.message, tt.field)
		if got.JSONName != tt.jsonName || got.Presence != tt.presence || got.Ignore != tt.ignore {
			t.Errorf("%s.%s: JSONName %q, Presence %v, Ignore %q; want %q, %v, %q",
				tt.message, tt.field, got.JSONName, got.Presence, got.Ignore, tt.jsonName, tt.presence, tt.ignore)
		}
	}
}

// parseFixture runs ParseFile over one file of the committed descriptor set,
// through protogen the way the plugin receives it.
func parseFixture(t *testing.T, path string) []MessageMetadata {
	t.Helper()
	set := readDescriptorSet(t)
	param := "M" + path + "=example.test/x"
	gen, err := protogen.Options{}.New(&pluginpb.CodeGeneratorRequest{
		FileToGenerate: []string{path},
		Parameter:      proto.String(param),
		ProtoFile:      set.GetFile(),
	})
	if err != nil {
		t.Fatal(err)
	}
	file, ok := gen.FilesByPath[path]
	if !ok {
		t.Fatalf("no file %s in the descriptor set", path)
	}
	messages, err := ParseFile(file)
	if err != nil {
		t.Fatal(err)
	}
	return messages
}
