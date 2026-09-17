package generator_test

import (
	"flag"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"google.golang.org/protobuf/compiler/protogen"
	"google.golang.org/protobuf/proto"
	"google.golang.org/protobuf/types/descriptorpb"
	"google.golang.org/protobuf/types/pluginpb"

	"github.com/d0whc3r/protoc-gen-strict/internal/generator"
)

var update = flag.Bool("update", false, "rewrite the golden files instead of comparing against them")

const (
	descriptorSet = "../testdata/descriptors.binpb"
	goldenDir     = "../testdata/golden"
)

// TestGolden runs the plugin over the committed descriptor set and compares
// every emitted file against its golden copy. It needs neither protoc nor buf:
// `make testdata` builds the descriptor set and it is checked in.
func TestGolden(t *testing.T) {
	resp := generate(t, "")

	seen := map[string]bool{}
	for _, file := range resp.GetFile() {
		name := file.GetName()
		seen[name] = true
		t.Run(name, func(t *testing.T) {
			path := filepath.Join(goldenDir, filepath.FromSlash(name))
			if *update {
				if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
					t.Fatal(err)
				}
				if err := os.WriteFile(path, []byte(file.GetContent()), 0o644); err != nil {
					t.Fatal(err)
				}
				return
			}
			want, err := os.ReadFile(path)
			if err != nil {
				t.Fatalf("missing golden file (run `make testdata-update`): %v", err)
			}
			if got := file.GetContent(); got != string(want) {
				t.Errorf("output differs from golden file %s\n--- got ---\n%s", path, got)
			}
		})
	}

	// A golden file with no output means a message or file was dropped, which
	// the per-file comparison alone would not catch.
	if *update {
		return
	}
	for _, name := range goldenFiles(t) {
		if !seen[name] {
			t.Errorf("golden file %s has no corresponding generated output", name)
		}
	}
}

// TestLanguageOption checks `lang` decides what is emitted, so each output lands
// in its own tree. The shared strict/types.ts follows the TypeScript overlay;
// the OpenAPI configuration is a single file for the whole run.
func TestLanguageOption(t *testing.T) {
	tests := []struct {
		param  string
		wantTS bool
		wantPy bool
		wantOA bool
	}{
		{param: "", wantTS: true, wantPy: true, wantOA: true},
		{param: "lang=typescript", wantTS: true},
		{param: "lang=python", wantPy: true},
		{param: "lang=openapi", wantOA: true},
		{param: "lang=typescript,lang=python", wantTS: true, wantPy: true},
	}

	for _, test := range tests {
		name := test.param
		if name == "" {
			name = "default"
		}
		t.Run(name, func(t *testing.T) {
			var ts, py, shared, oa bool
			for _, file := range generate(t, test.param).GetFile() {
				switch {
				case file.GetName() == "strict/types.ts":
					shared = true
				case file.GetName() == "openapi_config.yaml":
					oa = true
				case strings.HasSuffix(file.GetName(), ".strict.ts"):
					ts = true
				case strings.HasSuffix(file.GetName(), "_strict.py"):
					py = true
				}
			}
			if ts != test.wantTS || shared != test.wantTS {
				t.Errorf("TypeScript overlay = %v, shared module = %v, want both %v", ts, shared, test.wantTS)
			}
			if py != test.wantPy {
				t.Errorf("Python overlay = %v, want %v", py, test.wantPy)
			}
			if oa != test.wantOA {
				t.Errorf("OpenAPI config = %v, want %v", oa, test.wantOA)
			}
		})
	}
}

// TestUnknownOption checks a typo fails the generation instead of emitting
// nothing and looking like a working build.
func TestUnknownOption(t *testing.T) {
	var opts generator.Options
	if err := opts.Set("langs", "typescript"); err == nil {
		t.Error("expected an unknown option to be rejected")
	}
	if err := opts.Set("lang", "golang"); err == nil {
		t.Error("expected an unknown language to be rejected")
	}
}

// generate runs the real plugin entry point over the descriptor set.
func generate(t *testing.T, param string) *pluginpb.CodeGeneratorResponse {
	t.Helper()

	raw, err := os.ReadFile(descriptorSet)
	if err != nil {
		t.Fatalf("read descriptor set (run `make testdata`): %v", err)
	}
	var set descriptorpb.FileDescriptorSet
	if err := proto.Unmarshal(raw, &set); err != nil {
		t.Fatalf("unmarshal descriptor set: %v", err)
	}

	var targets []string
	params := []string{"paths=source_relative"}
	if param != "" {
		params = append(params, param)
	}
	for _, file := range set.GetFile() {
		if isDependency(file.GetName()) {
			continue
		}
		targets = append(targets, file.GetName())
		// protogen insists on a Go import path though nothing Go is emitted.
		// buf's managed mode injects go_package in production; M does it here.
		params = append(params, "M"+file.GetName()+"=example.test/"+filepath.Dir(file.GetName()))
	}
	if len(targets) == 0 {
		t.Fatal("descriptor set contains no target files")
	}

	var opts generator.Options
	gen, err := protogen.Options{ParamFunc: opts.Set}.New(&pluginpb.CodeGeneratorRequest{
		FileToGenerate: targets,
		Parameter:      proto.String(strings.Join(params, ",")),
		ProtoFile:      set.GetFile(),
	})
	if err != nil {
		t.Fatalf("build protogen plugin: %v", err)
	}
	if err := generator.Run(gen, opts); err != nil {
		t.Fatalf("generator.Run: %v", err)
	}
	resp := gen.Response()
	if resp.Error != nil {
		t.Fatalf("plugin reported an error: %s", resp.GetError())
	}
	return resp
}

// isDependency reports whether a descriptor came from an imported BSR module
// rather than this repo's proto/ directory.
func isDependency(name string) bool {
	return strings.HasPrefix(name, "google/") || strings.HasPrefix(name, "buf/")
}

// goldenFiles lists every checked-in golden file, as a slash path relative to
// the golden directory — the name the generator would have emitted it under.
func goldenFiles(t *testing.T) []string {
	t.Helper()
	var names []string
	err := filepath.WalkDir(goldenDir, func(path string, entry os.DirEntry, err error) error {
		if err != nil || entry.IsDir() {
			return err
		}
		rel, err := filepath.Rel(goldenDir, path)
		if err != nil {
			return err
		}
		names = append(names, filepath.ToSlash(rel))
		return nil
	})
	if err != nil {
		t.Fatalf("walk golden dir: %v", err)
	}
	return names
}
