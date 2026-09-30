package generator_test

import (
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"google.golang.org/protobuf/compiler/protogen"
	"google.golang.org/protobuf/proto"
	"google.golang.org/protobuf/types/pluginpb"

	"github.com/d0whc3r/protoc-gen-strict/internal/generator"
	"github.com/d0whc3r/protoc-gen-strict/internal/generator/schemagen"
)

// The golden trees of protoc-gen-strict-schema, one per run: the default
// (json+zod), the Zod 3 variant, whose modules differ from Zod 4's, and both
// again as JavaScript with declarations (output=js+dts).
const (
	schemaGoldenDir       = "../testdata/golden-schema/default"
	schemaZod3GoldenDir   = "../testdata/golden-schema/zod3"
	schemaJSGoldenDir     = "../testdata/golden-schema/js"
	schemaZod3JSGoldenDir = "../testdata/golden-schema/zod3-js"
	bundleDir             = "../testdata/jsonschema"
)

// TestSchemaGolden compares protoc-gen-strict-schema's output against its
// golden copies, as TestGolden does for protoc-gen-strict. The bundles it passes
// through from protoc-gen-jsonschema are compared with what that wrote instead.
func TestSchemaGolden(t *testing.T) {
	t.Run("default", func(t *testing.T) { compareGolden(t, withoutBundles(t, generateSchema(t, "")), schemaGoldenDir) })
	t.Run("zod3", func(t *testing.T) { compareGolden(t, generateSchema(t, "target=zod3"), schemaZod3GoldenDir) })
	t.Run("js", func(t *testing.T) {
		resp := generateSchema(t, "output=js+dts,import_extension=js")
		compareGolden(t, withoutBundles(t, resp), schemaJSGoldenDir)
	})
	t.Run("zod3-js", func(t *testing.T) {
		compareGolden(t, generateSchema(t, "target=zod3,output=js+dts,import_extension=js"), schemaZod3JSGoldenDir)
	})
}

// withoutBundles checks the jsonschema/ files are protoc-gen-jsonschema's
// bundles, byte for byte, and returns the response without them.
func withoutBundles(t *testing.T, resp *pluginpb.CodeGeneratorResponse) *pluginpb.CodeGeneratorResponse {
	t.Helper()
	out := &pluginpb.CodeGeneratorResponse{}
	bundles := 0
	for _, file := range resp.GetFile() {
		name, ok := strings.CutPrefix(file.GetName(), "jsonschema/")
		if !ok {
			out.File = append(out.File, file)
			continue
		}
		bundles++
		want, err := os.ReadFile(filepath.Join(bundleDir, name))
		if err != nil {
			t.Errorf("%s is no bundle protoc-gen-jsonschema wrote: %v", file.GetName(), err)
			continue
		}
		if file.GetContent() != string(want) {
			t.Errorf("%s differs from protoc-gen-jsonschema's bundle", file.GetName())
		}
	}
	if bundles == 0 {
		t.Error("no bundle under jsonschema/")
	}
	return out
}

// TestTargetOption checks `target` decides what is emitted: the JSON Schema
// modules and their shared table, the Zod modules of each major and theirs,
// and that no two targets write the same file.
func TestTargetOption(t *testing.T) {
	tests := []struct {
		param    string
		wantJSON bool
		wantZod  bool
		wantZod3 bool
	}{
		{param: "", wantJSON: true, wantZod: true},
		{param: "target=json", wantJSON: true},
		{param: "target=zod", wantZod: true},
		{param: "target=zod3", wantZod3: true},
		{param: "target=json+zod", wantJSON: true, wantZod: true},
		{param: "target=json+zod3", wantJSON: true, wantZod3: true},
		{param: "target=json,target=zod", wantJSON: true, wantZod: true},
		{param: "target=zod+zod3", wantZod: true, wantZod3: true},
		{param: "target=json+zod,target=zod3", wantJSON: true, wantZod: true, wantZod3: true},
		{param: "target=zod+zod3,output=js+dts", wantZod: true, wantZod3: true},
		{param: "output=js", wantJSON: true, wantZod: true},
	}
	for _, test := range tests {
		name := test.param
		if name == "" {
			name = "default"
		}
		t.Run(name, func(t *testing.T) {
			var json, jsonShared, zod, zod3, zodShared bool
			written := map[string]bool{}
			for _, file := range generateSchema(t, test.param).GetFile() {
				name := file.GetName()
				if written[name] {
					t.Errorf("%s written twice", name)
				}
				written[name] = true

				switch {
				case strings.HasPrefix(name, "strict/jsonschema."):
					jsonShared = true
				case strings.HasPrefix(name, "strict/protovalidate."):
					zodShared = true
				case strings.Contains(name, ".schema."):
					json = true
				case strings.Contains(name, ".zod."):
					zod = true
				case strings.Contains(name, ".zod3."):
					zod3 = true
				}
			}
			if json != test.wantJSON || jsonShared != test.wantJSON {
				t.Errorf("JSON Schema modules = %v, shared module = %v, want both %v", json, jsonShared, test.wantJSON)
			}
			if zod != test.wantZod || zod3 != test.wantZod3 {
				t.Errorf("Zod 4 modules = %v, Zod 3 modules = %v, want %v and %v", zod, zod3, test.wantZod, test.wantZod3)
			}
			if want := test.wantZod || test.wantZod3; zodShared != want {
				t.Errorf("shared Zod module = %v, want %v", zodShared, want)
			}
		})
	}
}

// TestUnknownTarget checks a typo fails the generation instead of emitting
// nothing.
func TestUnknownTarget(t *testing.T) {
	for _, test := range []struct{ name, value string }{
		{"lang", "json"},
		{"target", "jsonschema"},
		{"target", ""},
		{"output", "tsx"},
		{"output", ""},
		{"import_extension", ".ts"},
		{"jsonschema_plugin", ""},
	} {
		var opts generator.SchemaOptions
		if err := opts.Set(test.name, test.value); err == nil {
			t.Errorf("Set(%q, %q) = nil, want an error", test.name, test.value)
		}
	}
}

// generateSchema runs protoc-gen-strict-schema over the descriptor set, with
// protoc-gen-jsonschema played by the bundles `make testdata` committed.
func generateSchema(t *testing.T, param string) *pluginpb.CodeGeneratorResponse {
	t.Helper()
	var opts generator.SchemaOptions
	return respond(t, param, opts.Set, func(gen *protogen.Plugin) error {
		return generator.RunSchema(gen, opts, committedBundles(t))
	})
}

// committedBundles stands in for protoc-gen-jsonschema: it answers with the
// bundles the real one wrote for the same files, after checking it was asked
// for the bundle target.
func committedBundles(t *testing.T) schemagen.JSONSchemaPlugin {
	return func(_ string, req *pluginpb.CodeGeneratorRequest) (*pluginpb.CodeGeneratorResponse, error) {
		if got := req.GetParameter(); got != "target=json-bundle" {
			t.Errorf("protoc-gen-jsonschema run with %q, want target=json-bundle", got)
		}
		entries, err := os.ReadDir(bundleDir)
		if err != nil {
			return nil, err
		}
		resp := &pluginpb.CodeGeneratorResponse{}
		for _, entry := range entries {
			data, err := os.ReadFile(filepath.Join(bundleDir, entry.Name()))
			if err != nil {
				return nil, err
			}
			resp.File = append(resp.File, &pluginpb.CodeGeneratorResponse_File{
				Name: proto.String(entry.Name()), Content: proto.String(string(data)),
			})
		}
		return resp, nil
	}
}

// TestJSONSchemaFailures checks target=json fails the generation, with the
// reason, when protoc-gen-jsonschema cannot give it a bundle for every message.
func TestJSONSchemaFailures(t *testing.T) {
	for _, tt := range []struct {
		name string
		run  schemagen.JSONSchemaPlugin
		want string
	}{
		{
			name: "no bundles",
			run: func(string, *pluginpb.CodeGeneratorRequest) (*pluginpb.CodeGeneratorResponse, error) {
				return &pluginpb.CodeGeneratorResponse{}, nil
			},
			want: "wrote no target=json-bundle bundle for example.v1.Address, example.v1.User",
		},
		{
			name: "plugin error",
			run: func(string, *pluginpb.CodeGeneratorRequest) (*pluginpb.CodeGeneratorResponse, error) {
				return &pluginpb.CodeGeneratorResponse{Error: proto.String("boom")}, nil
			},
			want: "protoc-gen-jsonschema: boom",
		},
		{
			name: "not runnable",
			run: func(string, *pluginpb.CodeGeneratorRequest) (*pluginpb.CodeGeneratorResponse, error) {
				return nil, errors.New("not on PATH")
			},
			want: "not on PATH",
		},
	} {
		t.Run(tt.name, func(t *testing.T) {
			var opts generator.SchemaOptions
			var err error
			respond(t, "target=json", opts.Set, func(gen *protogen.Plugin) error {
				err = generator.RunSchema(gen, opts, tt.run)
				return nil
			})
			if err == nil || !strings.Contains(err.Error(), tt.want) {
				t.Errorf("RunSchema error = %v, want one containing %q", err, tt.want)
			}
		})
	}
}
