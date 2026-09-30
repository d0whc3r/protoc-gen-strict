package schemagen

import (
	"bytes"
	"fmt"
	"maps"
	"os/exec"
	"slices"
	"strings"

	"google.golang.org/protobuf/compiler/protogen"
	"google.golang.org/protobuf/proto"
	"google.golang.org/protobuf/types/pluginpb"
)

// The JSON Schema bundles come from protoschema-plugins' protoc-gen-jsonschema,
// which this plugin runs itself over the request buf handed it. A plugin never
// sees another plugin's output, so running it here is what lets the run fail
// when a bundle a .schema.ts imports would be missing.

// jsonSchemaVersion is the protoschema-plugins release jsonfixes.go mirrors.
// Another version may write other keywords, so it is refused rather than
// loosened by a table written for this one.
const jsonSchemaVersion = "v0.6.0"

// DefaultJSONSchemaPlugin is the executable `go install` gives
// protoc-gen-jsonschema.
const DefaultJSONSchemaPlugin = "protoc-gen-jsonschema"

// bundleTarget is the protoschema-jsonschema option the bundles are written
// with: JSON names, lenient, self-contained. See jsonfixes.go.
const bundleTarget = "target=json-bundle"

// jsonSchemaInstall is how to get the executable the plugin expects.
const jsonSchemaInstall = "go install github.com/bufbuild/protoschema-plugins/cmd/protoc-gen-jsonschema@" + jsonSchemaVersion

// JSONSchemaPlugin runs protoc-gen-jsonschema, the executable named, over a
// request and returns its response. ExecJSONSchema is the real one; the golden
// tests pass one that returns committed output.
type JSONSchemaPlugin func(executable string, req *pluginpb.CodeGeneratorRequest) (*pluginpb.CodeGeneratorResponse, error)

// ExecJSONSchema runs the executable as buf would run a plugin, after checking
// it is protoschema-plugins' protoc-gen-jsonschema at jsonSchemaVersion. Another
// plugin ships under the same name, so the check is not a formality.
func ExecJSONSchema(executable string, req *pluginpb.CodeGeneratorRequest) (*pluginpb.CodeGeneratorResponse, error) {
	path, err := exec.LookPath(executable)
	if err != nil {
		return nil, fmt.Errorf("%s is not on PATH; install it with `%s`, or set jsonschema_plugin=<path>", executable, jsonSchemaInstall)
	}

	out, err := exec.Command(path, "--version").CombinedOutput()
	if version := strings.TrimSpace(string(out)); err != nil || version != jsonSchemaVersion {
		return nil, fmt.Errorf("protoc-gen-jsonschema %s from bufbuild/protoschema-plugins is needed, but %s reports %q, and another plugin ships under that name; install it with `%s`, or set jsonschema_plugin=<path>", jsonSchemaVersion, path, version, jsonSchemaInstall)
	}

	in, err := proto.Marshal(req)
	if err != nil {
		return nil, fmt.Errorf("encode the request for %s: %w", path, err)
	}
	var stdout, stderr bytes.Buffer
	cmd := exec.Command(path)
	cmd.Stdin, cmd.Stdout, cmd.Stderr = bytes.NewReader(in), &stdout, &stderr
	if err := cmd.Run(); err != nil {
		return nil, fmt.Errorf("run %s: %w: %s", path, err, strings.TrimSpace(stderr.String()))
	}

	var resp pluginpb.CodeGeneratorResponse
	if err := proto.Unmarshal(stdout.Bytes(), &resp); err != nil {
		return nil, fmt.Errorf("decode the response of %s: %w", path, err)
	}
	return &resp, nil
}

// WriteBundles runs protoc-gen-jsonschema over the request, writes its bundles
// under jsonschema/, and fails unless there is one for every top-level message
// of a generated file: the ones a .schema.ts imports.
func WriteBundles(gen *protogen.Plugin, ctx *Context, executable string, run JSONSchemaPlugin) error {
	req, ok := proto.Clone(gen.Request).(*pluginpb.CodeGeneratorRequest)
	if !ok {
		return fmt.Errorf("copy the request for %s", executable) // unreachable: Clone keeps the type
	}
	req.Parameter = proto.String(bundleTarget)

	resp, err := run(executable, req)
	if err != nil {
		return err
	}
	if resp.GetError() != "" {
		return fmt.Errorf("%s: %s", executable, resp.GetError())
	}

	bundles := map[string]string{}
	for _, file := range resp.GetFile() {
		bundles[file.GetName()] = file.GetContent()
	}
	var missing []string
	for _, name := range ctx.bundled() {
		if _, ok := bundles[name+bundleSuffix]; !ok {
			missing = append(missing, name)
		}
	}
	if len(missing) > 0 {
		return fmt.Errorf("%s wrote no %s bundle for %s", executable, bundleTarget, strings.Join(missing, ", "))
	}

	for _, name := range slices.Sorted(maps.Keys(bundles)) {
		g := gen.NewGeneratedFile(jsonSchemaDir+name, "")
		_, _ = g.Write([]byte(bundles[name])) // a GeneratedFile buffers in memory and never fails
	}
	return nil
}

// bundled lists, sorted, the messages a .schema.ts imports a bundle for: the
// top-level messages of every file the run writes modules for.
func (c *Context) bundled() []string {
	var out []string
	for path := range c.generated {
		for _, msg := range c.byFile[path] {
			if c.topLevel(msg.Name) {
				out = append(out, msg.Name)
			}
		}
	}
	slices.Sort(out)
	return out
}
