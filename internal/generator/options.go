package generator

import "fmt"

// Options are the plugin's own parameters, on top of protogen's (`paths`,
// `M<file>=<import path>`, ...).
type Options struct {
	typeScript bool
	python     bool
	openAPI    bool
}

// Set records one `lang=` parameter. protogen calls it once per unrecognised
// option, so `lang=typescript,lang=python` selects both, as does omitting it.
//
// One language per invocation is what puts each overlay next to the output it
// narrows: buf gives each plugin entry a single `out`.
func (o *Options) Set(name, value string) error {
	if name != "lang" {
		return fmt.Errorf("unknown option %q", name)
	}
	switch value {
	case "typescript":
		o.typeScript = true
	case "python":
		o.python = true
	case "openapi":
		o.openAPI = true
	default:
		return fmt.Errorf("lang must be typescript, python or openapi, got %q", value)
	}
	return nil
}

// TypeScript reports whether to emit the TypeScript overlay: it was asked for,
// or nothing was.
func (o Options) TypeScript() bool { return o.typeScript || o.unset() }

// Python reports whether to emit the Python overlay.
func (o Options) Python() bool { return o.python || o.unset() }

// OpenAPI reports whether to emit the protoc-gen-openapiv2 configuration.
func (o Options) OpenAPI() bool { return o.openAPI || o.unset() }

// unset reports that no language was named, which selects every one of them.
func (o Options) unset() bool { return !o.typeScript && !o.python && !o.openAPI }
