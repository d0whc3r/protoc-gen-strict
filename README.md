# protoc-gen-strict

A protoc/buf plugin that reads [protovalidate](https://github.com/bufbuild/protovalidate)
(`buf.validate`) rules off your `.proto` files and layers **strict types** on top
of the code the official generators already emit: `protoc-gen-es` for TypeScript,
`protoc-gen-python` for Python, and a field-options file for
`protoc-gen-openapiv2`.

The official plugins own the types. `protoc-gen-strict` only adds what they
cannot know: your validation rules. A second binary, `protoc-gen-strict-schema`,
turns the same rules into runtime schemas: JSON Schema, and Zod 4 or Zod 3.

## What it does

The proto is the source of truth for both the shape of a message and the rules
its values must satisfy. The official generator carries only the shape across:

```proto
message DeleteProductRequest {
  string id = 1 [
    (buf.validate.field).required = true,
    (buf.validate.field).string.uuid = true
  ];
}
```

```ts
// protoc-gen-es output — both rules are gone
export type DeleteProductRequest =
  Message<"shop.catalog.v1.DeleteProductRequest"> & {
    id: string;
  };
```

```ts
// protoc-gen-strict output — the rules that have a type equivalent are now typed
export type DeleteProductRequestStrict = Narrow<
  DeleteProductRequest,
  {
    id: Uuid;
  }
>;
```

A rule with no type equivalent is named in the generated doc comment and left to
protovalidate at runtime — so a rule that stops being carried after a proto edit
shows up as a diff in your next `buf generate` instead of quietly disappearing.

## Plugins

Two binaries, both in the same release archive, and on npm as
`@d0whc3r/protoc-gen-strict` and `@d0whc3r/protoc-gen-strict-schema`. Each
README covers install, the `buf.gen.yaml` entries, every option and what lands
where.

| Plugin                                                             | Emits                                                                             |
| ------------------------------------------------------------------ | --------------------------------------------------------------------------------- |
| [protoc-gen-strict](cmd/protoc-gen-strict/README.md)               | Type overlays for TypeScript and Python, field options for protoc-gen-openapiv2   |
| [protoc-gen-strict-schema](cmd/protoc-gen-strict-schema/README.md) | Runtime schemas: JSON Schema on top of protoschema-jsonschema, and Zod 4 or Zod 3 |

## Before you rely on the type overlays

- **A shape is not validation.** protovalidate remains the authority; the
  compiler check is a shape check. `Uuid` accepts anything with five
  dash-separated groups, and `string.min_len` carries nothing at all.
- **Under-narrowing is deliberate.** A type that admits a value protovalidate
  rejects costs a runtime error that would have happened anyway. The reverse
  would stop a caller from expressing something the schema allows.
- **Read the generated doc comment.** Each type names the rules it carries and
  the ones left to runtime validation.
- **Runtime identity is unchanged.** The schema and service consts are the
  objects the official generator produced, re-annotated rather than rebuilt, so
  adopting a strict import cannot change what goes over the network.
- **One deliberate over-narrowing:** an enum's zero member. protovalidate
  accepts `UNSPECIFIED` on an unconstrained field; the strict type does not.
  Name the generated type instead of its overlay where that member is meant to
  be allowed.

The runtime schemas have their own list:
[Rule coverage: runtime schemas](docs/rule-coverage-schema.md#before-you-rely-on-it).

## Documentation

| Document                                                       | Covers                                                                                      |
| -------------------------------------------------------------- | ------------------------------------------------------------------------------------------- |
| [Rule coverage](docs/rule-coverage.md)                         | What the plugin does with each rule, which target carries what, and the current limitations |
| [Rule coverage: TypeScript](docs/rule-coverage-typescript.md)  | Everything the TypeScript overlay emits, every narrowing, and the structural limits         |
| [Rule coverage: Python](docs/rule-coverage-python.md)          | The `Annotated` aliases and how each TypeScript narrowing reads there                       |
| [Rule coverage: OpenAPI](docs/rule-coverage-openapi.md)        | The rules that reach the swagger, as JSONSchema keywords                                    |
| [Rule coverage: runtime schemas](docs/rule-coverage-schema.md) | What the JSON Schema and Zod modules export, and which rules each enforces                  |
| [Internals](docs/internals.md)                                 | The pipeline, the intermediate representation, and how to add a narrowing                   |
| [Contributing](CONTRIBUTING.md)                                | Build, test and release                                                                     |

## License

[MIT](LICENSE).
