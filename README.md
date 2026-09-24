# protoc-gen-strict

A protoc/buf plugin that reads [protovalidate](https://github.com/bufbuild/protovalidate)
(`buf.validate`) rules off your `.proto` files and layers **strict types** on top
of the code the official generators already emit: `protoc-gen-es` for TypeScript,
`protoc-gen-python` for Python, and a field-options file for
`protoc-gen-openapiv2`.

The official plugins own the types. `protoc-gen-strict` only adds what they
cannot know: your validation rules.

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
export type DeleteProductRequest = Message<"shop.catalog.v1.DeleteProductRequest"> & {
  id: string;
};
```

```ts
// protoc-gen-strict output — the rules that have a type equivalent are now typed
export type DeleteProductRequestStrict = Narrow<DeleteProductRequest, {
  id: Uuid;
}>;
```

A rule with no type equivalent is named in the generated doc comment and left to
protovalidate at runtime — so a rule that stops being carried after a proto edit
shows up as a diff in your next `buf generate` instead of quietly disappearing.

## Install

```sh
go install github.com/d0whc3r/protoc-gen-strict@latest
```

That puts `protoc-gen-strict` in `$(go env GOPATH)/bin`, which needs to be on
your `PATH` when `buf` runs.

Or take a prebuilt binary for Linux, macOS or Windows from the
[releases page](https://github.com/d0whc3r/protoc-gen-strict/releases). Unpack it
onto your `PATH` **under its own name**: protoc and buf resolve a plugin by
looking for `protoc-gen-strict`, so renaming it breaks the lookup.

Requires [`buf`](https://buf.build/docs/installation), plus Go 1.26+ if you
install with `go install`.

## Configure

Two facts drive the whole configuration:

- An overlay `import`s from the file it narrows, so it has to land in the **same
  directory** as the official generator's output.
- `lang=` emits **one language per plugin entry**, which is what lets each tree
  stay separate.

So the plugin appears once per language, each time with the same `out:` as the
generator it layers on:

```yaml
# buf.gen.yaml
version: v2
managed:
  enabled: true
  override:
    - file_option: go_package_prefix
      value: github.com/you/yourrepo/gen
plugins:
  # TypeScript
  - remote: buf.build/bufbuild/es:v2.15.0
    out: gen/typescript
    opt: target=ts
  - local: protoc-gen-strict
    out: gen/typescript
    opt: paths=source_relative,lang=typescript
    strategy: all

  # Python
  - remote: buf.build/protocolbuffers/python:v34.0
    out: gen/python
  - remote: buf.build/protocolbuffers/pyi:v34.0
    out: gen/python
  - local: protoc-gen-strict
    out: gen/python
    opt: paths=source_relative,lang=python
    strategy: all

  # OpenAPI. Not an overlay: the field options protoc-gen-openapiv2 reads.
  - local: protoc-gen-strict
    out: gen/openapiv2
    opt: paths=source_relative,lang=openapi
    strategy: all
```

### OpenAPI needs a second pass

protoc-gen-openapiv2 has to read `openapi_config.yaml`, so it cannot run in the
same pass that writes it. Give it its own template:

```yaml
# buf.gen.openapi.yaml
version: v2
managed:
  enabled: true
  override:
    - file_option: go_package_prefix
      value: github.com/you/yourrepo/gen
plugins:
  # Local, not `remote:`. A BSR plugin runs on buf's servers, where
  # openapi_configuration has no local file to read.
  - local: ["go", "run", "github.com/grpc-ecosystem/grpc-gateway/v2/protoc-gen-openapiv2@latest"]
    out: gen/openapiv2
    strategy: all
    opt:
      - openapi_configuration=gen/openapiv2/openapi_config.yaml
      - omit_enum_default_value=true
```

Then run the two in order:

```sh
buf generate
buf generate --template buf.gen.openapi.yaml
```

### What lands where

```
gen/typescript/example/v1/user_pb.ts        protoc-gen-es
gen/typescript/example/v1/user.strict.ts    protoc-gen-strict
gen/typescript/strict/types.ts              protoc-gen-strict, once per generation
gen/python/example/v1/user_pb2.py           protoc-gen-python
gen/python/example/v1/user_pb2.pyi          protoc-gen-pyi
gen/python/example/v1/user_strict.py        protoc-gen-strict
gen/openapiv2/openapi_config.yaml           protoc-gen-strict, once per generation
gen/openapiv2/example/v1/user.swagger.json  protoc-gen-openapiv2, second pass
```

## Options

Everything in `opt:`, and the two `buf.gen.yaml` settings the plugin depends on:

| Setting | Where | Effect |
|---|---|---|
| `lang=typescript` | `opt:` | Emit only the TypeScript overlay, and the shared `strict/types.ts` with it |
| `lang=python` | `opt:` | Emit only the Python overlay |
| `lang=openapi` | `opt:` | Emit only `openapi_config.yaml`, the protoc-gen-openapiv2 field options |
| `lang=` omitted | `opt:` | Emit all three, into one tree |
| `paths=source_relative` | `opt:` | **Required.** Mirror the `.proto` directory layout in the output |
| `strategy: all` | plugin entry | **Required.** One plugin process for the whole request, not one per directory |
| `managed: enabled: true` | top level | **Required.** Supply a Go import path the plugin never emits but its framework insists on resolving |

`lang=` is the plugin's own option; repeating it
(`lang=typescript,lang=python`) selects both, and anything else fails the
generation rather than quietly emitting nothing. `paths=` comes from
`protogen`, the protoc-gen-go framework this plugin is built on, along with a
few options that only affect Go output and have no use here.

Why the three required settings are not optional:

- **`paths=source_relative`.** The default, `paths=import`, derives the output
  directory from the Go import path, so with the `managed` block above the
  overlay lands in `gen/typescript/github.com/you/yourrepo/gen/example/v1/`
  while `strict/types.ts` stays at the root of `out:`. Both the `./user_pb`
  import and the `../../strict/types` import then resolve to nothing.
- **`strategy: all`.** Without it buf forks the plugin once per directory. A
  TypeScript narrowing reaches across files, so a shard that cannot see its
  target silently drops the narrowing — and each shard emits its own
  `strict/types.ts`.
- **`managed`.** `protogen` resolves a Go import path for every file even when
  it emits no Go. Managed mode supplies one without putting `go_package` in your
  `.proto` files.

## Using the output

### TypeScript

```proto
// User demonstrates standard buf.validate rules alongside custom CEL rules.
message User {
  string id = 1 [
    (buf.validate.field).string.min_len = 5,
    (buf.validate.field).cel = {
      id: "user.id.prefix",
      message: "id must start with 'usr_'",
      expression: "this.startsWith('usr_')"
    }
  ];

  string email = 2 [
    (buf.validate.field).required = true,
    (buf.validate.field).string.email = true
  ];

  int32 age = 3 [(buf.validate.field).int32.gt = 0];

  repeated string roles = 4 [(buf.validate.field).repeated.min_items = 1];
}
```

```ts
// Code generated by protoc-gen-strict. DO NOT EDIT.
// source: example/v1/user.proto

import type { Email, Narrow, NonEmptyList } from "../../strict/types";
import type { User } from "./user_pb";
import { UserSchema } from "./user_pb";
import type { GenMessage } from "@bufbuild/protobuf/codegenv2";

/**
 * User demonstrates standard buf.validate rules alongside custom CEL rules.
 *
 * Left to runtime validation, having no type equivalent:
 *   id
 *     string.min_len = 5
 *     cel[user.id.prefix]: this.startsWith('usr_')
 *       message: id must start with 'usr_'
 *       refs: this
 *       calls: startsWith
 *   age
 *     int32.gt = 0
 */
export type UserStrict = Narrow<User, {
  /**
   * required
   * string.email = true
   *
   * Carried into the type: string.email.
   */
  email: Email;
  /**
   * repeated.min_items = 1
   *
   * Carried into the type: repeated.min_items.
   */
  roles: NonEmptyList<User["roles"]>;
}>;

/**
 * Describes example.v1.User, reporting UserStrict as its valid type.
 * The same descriptor protoc-gen-es generated, so the wire format and the identity
 * this has as a query key are unchanged — only what `MessageValidType` reports differs.
 */
export const UserStrictSchema =
  UserSchema as GenMessage<User, { validType: UserStrict }>;
```

`UserStrict` is assignable to `User`, so it goes straight into `create()`,
`toBinary()` or anything else protobuf-es generated. To build a message and have
the compiler check the rules, use `createStrict` with the matching schema — here
a request whose `id` carries `string.uuid`:

```ts
const del = createStrict(DeleteProductRequestStrictSchema, {
  id: "3fa85f64-5717-4562-b3fc-2c963f66afa6",   // the literal fits the UUID shape
});
```

`<Message>StrictSchema` is the descriptor `protoc-gen-es` already produced, only
re-annotated: it is what `createStrict` builds from, and what carries the
narrowing to a Connect or gRPC call site that never names the type. The overlay
emits a `<Service>Strict` descriptor for the same reason. See
[Rule coverage: TypeScript](docs/rule-coverage-typescript.md).

### Python

Python has no way to narrow a type, so the overlay emits one
`typing.Annotated` alias per constrained field, which makes the rules visible at
the call site:

```python
from example.v1.user_strict import UserId

def promote(user_id: UserId) -> None: ...   # not `user_id: str`
```

A rule with an [annotated_types](https://github.com/annotated-types/annotated-types)
equivalent is carried as that constructor, which pydantic, msgspec and beartype
enforce; everything else is metadata text. That makes `annotated_types` a
dependency of the generated Python. See
[Rule coverage: Python](docs/rule-coverage-python.md).

### OpenAPI

`openapi_config.yaml` is not an overlay but the grpc-gateway configuration that
carries the rules into protoc-gen-openapiv2's output, one JSONSchema per
constrained field. See
[Rule coverage: OpenAPI](docs/rule-coverage-openapi.md).

## Before you rely on it

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

## Documentation

| Document | Covers |
|---|---|
| [Rule coverage](docs/rule-coverage.md) | What the plugin does with each rule, which target carries what, and the current limitations |
| [Rule coverage: TypeScript](docs/rule-coverage-typescript.md) | Everything the TypeScript overlay emits, every narrowing, and the structural limits |
| [Rule coverage: Python](docs/rule-coverage-python.md) | The `Annotated` aliases and how each TypeScript narrowing reads there |
| [Rule coverage: OpenAPI](docs/rule-coverage-openapi.md) | The rules that reach the swagger, as JSONSchema keywords |
| [Internals](docs/internals.md) | The pipeline, the intermediate representation, and how to add a narrowing |
| [Contributing](CONTRIBUTING.md) | Build, test and release |

## License

[MIT](LICENSE).
