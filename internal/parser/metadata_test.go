package parser

import (
	"slices"
	"strings"
	"testing"
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
