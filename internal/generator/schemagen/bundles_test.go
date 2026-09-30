package schemagen

import (
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"

	"google.golang.org/protobuf/types/pluginpb"
)

// TestExecJSONSchema covers the checks before the bundles are trusted: the
// executable is there, and it is protoschema-plugins' at jsonSchemaVersion.
// Another plugin ships as protoc-gen-jsonschema and answers --version with
// something else, which is the case the check exists for.
func TestExecJSONSchema(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("the fake plugins are shell scripts")
	}
	script := func(t *testing.T, body string) string {
		t.Helper()
		path := filepath.Join(t.TempDir(), "protoc-gen-jsonschema")
		if err := os.WriteFile(path, []byte("#!/bin/sh\n"+body), 0o755); err != nil {
			t.Fatal(err)
		}
		return path
	}
	req := &pluginpb.CodeGeneratorRequest{}

	for _, tt := range []struct {
		name, executable, want string
	}{
		{"missing", filepath.Join(t.TempDir(), "absent"), "is not on PATH"},
		{"another plugin", script(t, "echo 'no files to generate' >&2; exit 1\n"), `reports "no files to generate"`},
		{"another version", script(t, "echo v0.5.2\n"), `reports "v0.5.2"`},
		{"failing run", script(t, `[ "$1" = --version ] && echo `+jsonSchemaVersion+` && exit 0; echo broken >&2; exit 2`+"\n"), "broken"},
	} {
		t.Run(tt.name, func(t *testing.T) {
			_, err := ExecJSONSchema(tt.executable, req)
			if err == nil || !strings.Contains(err.Error(), tt.want) {
				t.Errorf("ExecJSONSchema error = %v, want one containing %q", err, tt.want)
			}
		})
	}

	t.Run("the right one", func(t *testing.T) {
		// Answers --version, then an empty response: zero bytes decode as one.
		path := script(t, `[ "$1" = --version ] && echo `+jsonSchemaVersion+"\nexit 0\n")
		resp, err := ExecJSONSchema(path, req)
		if err != nil || len(resp.GetFile()) != 0 {
			t.Errorf("ExecJSONSchema = %v, %v; want an empty response", resp, err)
		}
	})
}
