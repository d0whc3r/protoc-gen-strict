package tsgen

import (
	"strconv"

	"google.golang.org/protobuf/compiler/protogen"

	"github.com/d0whc3r/protoc-gen-strict/internal/generator/emit"
	"github.com/d0whc3r/protoc-gen-strict/internal/parser"
)

// The retyped descriptors. A Connect or gRPC caller rarely names a request
// type: it holds a schema or a service descriptor and lets `MessageValidType`
// report the shape. Re-annotating those is how a narrowing reaches it. It is
// also what `createStrict` reads to build a message and report its strict type,
// so every message that narrows gets one, not only an RPC input or output.

// strictSchema emits `<Name>StrictSchema`, the same descriptor object retyped.
func (f *tsFile) strictSchema(msg parser.MessageMetadata) {
	name := f.ctx.tsName(msg.Name)
	strict := f.strictRef(msg.Name)
	schema := f.declare(name + "StrictSchema")
	// The cast is the only place the type is written. A declaration file prints
	// the asserted type verbatim, so annotating the const as well would repeat
	// the whole generic on the line above it.
	typ := f.gen("GenMessage") + "<" + f.shape(name) + ", { validType: " + strict + " }>"
	f.doc("", []string{
		"Describes " + msg.Name + ", reporting " + strict + " as its valid type.",
		"The same descriptor protoc-gen-es generated, so the wire format and the identity",
		"this has as a query key are unchanged — only what `MessageValidType` reports differs.",
	})
	f.P("export const ", schema, " =")
	f.P("  ", f.value(name+"Schema"), " as ", typ, ";")
	f.P()
}

// service emits `<Name>Strict`, the service descriptor with every method
// pointing at the strict schema of its input and output.
func (f *tsFile) service(service *protogen.Service) {
	name := string(service.Desc.Name())
	strict := f.declare(name + "Strict")
	descriptor := f.declare(name + "StrictDescriptor")

	f.P("type ", descriptor, " = ", f.gen("GenService"), "<{")
	for _, method := range service.Methods {
		f.doc("  ", docLines(emit.OneLine(string(method.Comments.Leading))))
		f.P("  ", methodName(string(method.Desc.Name())), ": {")
		f.P("    methodKind: ", strconv.Quote(methodKind(method)), ";")
		f.P("    input: typeof ", f.schemaRef(string(method.Input.Desc.FullName())), ";")
		f.P("    output: typeof ", f.schemaRef(string(method.Output.Desc.FullName())), ";")
		f.P("  },")
	}
	f.P("}>;")
	f.P()

	f.doc("", docLines(
		emit.OneLine(string(service.Comments.Leading)),
		"The same service descriptor protoc-gen-es generated, retyped so every method reports",
		"the strict input and output types.",
	))
	f.P("export const ", strict, " =")
	f.P("  ", f.value(name), " as ", descriptor, ";")
	f.P()
}

// schemaRef names the schema const a method points at: the strict one where it
// exists, the generated one otherwise. Either may live in another file.
func (f *tsFile) schemaRef(fullName string) string {
	strict := f.ctx.needsStrict[fullName]
	suffix, module := "Schema", esSuffix
	if strict {
		suffix, module = "StrictSchema", strictSuffix
	}

	target := f.ctx.fileOf[fullName]
	name := f.ctx.tsName(fullName) + suffix
	if target == f.proto {
		if strict {
			return f.declare(name) // declared above in this very file
		}
		return f.value(name) // from the protoc-gen-es module next door
	}
	return f.foreignValue(f.moduleFor(target, module), name)
}

func methodKind(method *protogen.Method) string {
	switch {
	case method.Desc.IsStreamingClient() && method.Desc.IsStreamingServer():
		return "bidi_streaming"
	case method.Desc.IsStreamingClient():
		return "client_streaming"
	case method.Desc.IsStreamingServer():
		return "server_streaming"
	default:
		return "unary"
	}
}
