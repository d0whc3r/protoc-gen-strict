package schemagen

import (
	"regexp"
	"slices"
	"testing"

	"github.com/d0whc3r/protoc-gen-strict/internal/generator/emit"
	"github.com/d0whc3r/protoc-gen-strict/internal/parser"
)

// TestEmissionOrder covers the order a file's messages print in: after the
// messages their fields name, with cycles marked so the fields that close them
// can be deferred.
func TestEmissionOrder(t *testing.T) {
	ref := func(name string) parser.FieldMetadata {
		return parser.FieldMetadata{Name: "f", ProtoType: "message:" + name}
	}
	messages := []parser.MessageMetadata{
		{Name: "p.A", Fields: []parser.FieldMetadata{ref("p.B")}},
		{Name: "p.B"},
		{Name: "p.C", Fields: []parser.FieldMetadata{ref("p.D")}},
		{Name: "p.D", Fields: []parser.FieldMetadata{ref("p.C")}},
		{Name: "p.E", Fields: []parser.FieldMetadata{ref("p.E"), ref("p.Elsewhere")}},
		// z.unknown() stands for a field protovalidate never evaluates.
		{Name: "p.F", Fields: []parser.FieldMetadata{{Name: "f", ProtoType: "message:p.F", Ignore: emit.IgnoreAlways}}},
	}

	order, cycle := emissionOrder(messages)
	var names []string
	for _, msg := range order {
		names = append(names, msg.Name)
	}
	if want := []string{"p.B", "p.A", "p.C", "p.D", "p.E", "p.F"}; !slices.Equal(names, want) {
		t.Errorf("order = %v, want %v", names, want)
	}
	if cycle["p.A"] != 0 || cycle["p.B"] != 0 {
		t.Errorf("A and B are in no cycle, got %d and %d", cycle["p.A"], cycle["p.B"])
	}
	if cycle["p.C"] == 0 || cycle["p.C"] != cycle["p.D"] {
		t.Errorf("C and D share a cycle, got %d and %d", cycle["p.C"], cycle["p.D"])
	}
	if cycle["p.E"] == 0 || cycle["p.E"] == cycle["p.C"] {
		t.Errorf("E is a cycle of its own, got %d", cycle["p.E"])
	}
	if cycle["p.F"] != 0 {
		t.Errorf("F refers to itself only through an unchecked field, got cycle %d", cycle["p.F"])
	}
}

// TestPlanJSON pins the fixes that depend on more than one rule, or on the
// field's presence, which the golden output shows only as a changed table.
func TestPlanJSON(t *testing.T) {
	rule := func(kind, value string) parser.Rule { return parser.Rule{Kind: kind, Value: value} }
	tests := []struct {
		name  string
		field parser.FieldMetadata
		want  []jsonFix // reasons are not compared
	}{
		{
			name:  "len_bytes alone: upstream writes maxLength 0",
			field: parser.FieldMetadata{ProtoType: "string", Rules: []parser.Rule{rule("string.len_bytes", "4")}},
			want:  []jsonFix{{drop: []string{"maxLength"}}},
		},
		{
			name: "len_bytes with max_len: max_len overwrites it",
			field: parser.FieldMetadata{ProtoType: "string", Rules: []parser.Rule{
				rule("string.len_bytes", "4"), rule("string.max_len", "4"),
			}},
		},
		{
			name: "required with presence: a set field may hold an empty string",
			field: parser.FieldMetadata{ProtoType: "string", Optional: true, Presence: true, Required: true, Rules: []parser.Rule{
				rule("string.email", "true"),
			}},
			want: []jsonFix{{drop: []string{"format", "minLength"}}},
		},
		{
			name:  "required with presence alone: upstream writes no length at all",
			field: parser.FieldMetadata{ProtoType: "string", Optional: true, Presence: true, Required: true},
		},
		{
			name: "required with presence and min_len: the length is the rule's",
			field: parser.FieldMetadata{ProtoType: "string", Optional: true, Presence: true, Required: true, Rules: []parser.Rule{
				rule("string.min_len", "2"),
			}},
		},
		{
			name:  "required on an implicit string: exact",
			field: parser.FieldMetadata{ProtoType: "string", Required: true},
		},
		{
			name: "IGNORE_IF_ZERO_VALUE: every keyword goes",
			field: parser.FieldMetadata{ProtoType: "int32", Ignore: ignoreIfZero, Rules: []parser.Rule{
				rule("ignore", ignoreIfZero), rule("int32.gt", "5"),
			}},
			want: []jsonFix{{drop: ifZeroKeywords}},
		},
		{
			name: "IGNORE_ALWAYS on a message: the $ref goes, nothing nested is evaluated",
			field: parser.FieldMetadata{ProtoType: "message:p.Detail", Ignore: emit.IgnoreAlways, Rules: []parser.Rule{
				rule("ignore", emit.IgnoreAlways),
			}},
			want: []jsonFix{{drop: []string{"$ref"}}},
		},
		{
			name: "IGNORE_ALWAYS on a well-known type: its def carries no rule",
			field: parser.FieldMetadata{ProtoType: "message:google.protobuf.Timestamp", Ignore: emit.IgnoreAlways, Rules: []parser.Rule{
				rule("ignore", emit.IgnoreAlways),
			}},
		},
		{
			name: "IGNORE_ALWAYS under repeated.items: upstream writes the item keywords anyway",
			field: parser.FieldMetadata{ProtoType: "string", Repeated: true, Rules: []parser.Rule{
				rule("repeated.items.ignore", emit.IgnoreAlways), rule("repeated.items.string.min_len", "3"),
			}},
			want: []jsonFix{{at: atItems, drop: ifZeroKeywords}},
		},
		{
			name:  "float bound: upstream writes the float32 as its shortest decimal",
			field: parser.FieldMetadata{ProtoType: "float", Rules: []parser.Rule{rule("float.lte", "0.1")}},
			want:  []jsonFix{{drop: []string{"enum", "exclusiveMaximum", "exclusiveMinimum", "maximum", "minimum"}}},
		},
		{
			name:  "map<bool, _>: the key schema goes",
			field: parser.FieldMetadata{IsMap: true, MapKey: "bool", MapValue: "string"},
			want:  []jsonFix{{drop: []string{atKeys}}},
		},
		{
			name:  "repeated bytes: the base64 pattern goes from the items",
			field: parser.FieldMetadata{ProtoType: "bytes", Repeated: true},
			want:  []jsonFix{{at: atItems, drop: []string{"pattern"}}},
		},
		{
			name: "prefix under a user pattern: only the pattern is emitted",
			field: parser.FieldMetadata{ProtoType: "string", Rules: []parser.Rule{
				rule("string.pattern", "^a"), rule("string.prefix", "a"),
			}},
			want: []jsonFix{{drop: []string{"pattern"}}},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := planJSON(tt.field).fixes
			if len(got) != len(tt.want) {
				t.Fatalf("fixes = %+v, want %+v", got, tt.want)
			}
			for i := range got {
				if got[i].at != tt.want[i].at || !slices.Equal(got[i].drop, tt.want[i].drop) {
					t.Errorf("fix %d = %+v, want %+v", i, got[i], tt.want[i])
				}
			}
		})
	}
}

// TestPropertyKey covers json_name, which admits any string as a JSON key.
func TestPropertyKey(t *testing.T) {
	for name, want := range map[string]string{
		"extId":       "extId",
		"metric1st":   "metric1st",
		"_private":    "_private",
		"external-id": `"external-id"`,
		"1st":         `"1st"`,
		"":            `""`,
	} {
		if got := propertyKey(name); got != want {
			t.Errorf("propertyKey(%q) = %s, want %s", name, got, want)
		}
	}
}

// TestValueName covers the camelCase name a schema is declared under: nesting
// folded, first letter lowered, the rest of protoc-gen-es's casing kept, and a
// `$1` on the later of two types of one file that fold to the same name.
func TestValueName(t *testing.T) {
	message := func(name string) parser.MessageMetadata { return parser.MessageMetadata{Name: name} }
	ctx := &Context{
		pkgOf:  map[string]string{"a.proto": "p.v1"},
		fileOf: map[string]string{},
		byFile: map[string][]parser.MessageMetadata{"a.proto": {
			message("p.v1.User"), message("p.v1.Warehouse.Address"), message("p.v1.Outer.Inner.Leaf"),
			message("p.v1.UInt32Rules"), message("p.v1.Order"), message("p.v1.OrderStatus"),
		}},
		enumsOf: map[string][]parser.EnumMetadata{"a.proto": {{Name: "p.v1.Order.Status"}}},
		stems:   map[string]string{},
	}
	want := map[string]string{
		"p.v1.User":              "user",
		"p.v1.Warehouse.Address": "warehouseAddress",
		"p.v1.Outer.Inner.Leaf":  "outerInnerLeaf",
		"p.v1.UInt32Rules":       "uInt32Rules",
		"p.v1.Order":             "order",
		"p.v1.OrderStatus":       "orderStatus",
		"p.v1.Order.Status":      "orderStatus$1",
	}
	for fullName := range want {
		ctx.fileOf[fullName] = "a.proto"
	}
	ctx.nameTypes("a.proto")

	for fullName, stem := range want {
		if got := ctx.valueName(fullName); got != stem {
			t.Errorf("valueName(%q) = %s, want %s", fullName, got, stem)
		}
	}
}

// TestFileSymbol covers protoc-gen-es's file descriptor name: a run of
// characters that cannot be in an identifier is one underscore.
func TestFileSymbol(t *testing.T) {
	for proto, want := range map[string]string{
		"example/v1/user.proto":  "file_example_v1_user",
		"scratch/v1/a--b.proto":  "file_scratch_v1_a_b",
		"scratch/v1/a.b_c.proto": "file_scratch_v1_a_b_c",
	} {
		if got := fileSymbol(proto); got != want {
			t.Errorf("fileSymbol(%q) = %s, want %s", proto, got, want)
		}
	}
}

// TestRuntimeTwins covers the hand-written strict/ modules, kept as .ts, .js
// and .d.ts by hand: each export of one exists in the other two, and both Zod
// majors' well-known-type modules export the same schemas. make verify checks
// the logic and the types; this catches a missing export without the network.
func TestRuntimeTwins(t *testing.T) {
	values := map[flavor]*regexp.Regexp{
		flavorTS:  regexp.MustCompile(`(?m)^export (?:function|const) ([\w$]+)`),
		flavorJS:  regexp.MustCompile(`(?m)^export (?:function|const) ([\w$]+)`),
		flavorDTS: regexp.MustCompile(`(?m)^export declare (?:function|const) ([\w$]+)`),
	}
	exports := func(name string, fl flavor) []string {
		t.Helper()
		src, err := runtimeSource(name, fl)
		if err != nil {
			t.Fatal(err)
		}
		var names []string
		for _, m := range values[fl].FindAllStringSubmatch(src, -1) {
			names = append(names, m[1])
		}
		slices.Sort(names)
		return names
	}

	for _, name := range []string{"jsonschema", "protovalidate", "wkt.zod3", "wkt.zod4"} {
		ts := exports(name, flavorTS)
		if len(ts) == 0 {
			t.Errorf("runtime/%s.ts exports nothing", name)
		}
		for _, fl := range []flavor{flavorJS, flavorDTS} {
			if got := exports(name, fl); !slices.Equal(got, ts) {
				t.Errorf("runtime/%s%s exports %v, the .ts %v", name, fl.extension(), got, ts)
			}
		}
	}
	if zod3, zod4 := exports("wkt.zod3", flavorTS), exports("wkt.zod4", flavorTS); !slices.Equal(zod3, zod4) {
		t.Errorf("wkt.zod3 exports %v, wkt.zod4 %v", zod3, zod4)
	}
}
