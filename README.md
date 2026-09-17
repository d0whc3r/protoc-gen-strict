# protoc-gen-strict

A protoc/buf plugin that reads [protovalidate](https://github.com/bufbuild/protovalidate)
(`buf.validate`) rules off your `.proto` files and layers **strict types** on top
of the code the official generators already emit.

The official plugins own the types. `protoc-gen-strict` only adds what they
cannot know: your validation rules.

## The gap it fills

The proto is the source of truth for both the shape of a message and the rules
its values must satisfy. `protoc-gen-es` carries only the shape across:

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

Everything the rules said is now runtime-only. protovalidate still enforces it,
but only after the request is built. In a UI that means a round trip and an
error toast for a mistake the compiler could have caught.

This plugin turns the rules that *have* a type equivalent into part of the type:

```ts
// protoc-gen-strict output
export type DeleteProductRequestStrict = Narrow<DeleteProductRequest, {
  id: Uuid;
}>;
```

Everything else keeps being validated at runtime, and is named in the JSDoc of
the type it belongs to. A rule that stops being translated after a proto edit
then shows up as a diff in your next `buf generate` instead of quietly
disappearing.

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

Requires [`buf`](https://buf.build/docs/installation), plus Go 1.27+ if you
install with `go install`.

## Use

An overlay imports from the file it narrows, so it has to land in the same tree
as the official generator's output. `lang=` picks one language per invocation,
which is what lets each tree stay separate:

```yaml
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

protoc-gen-openapiv2 itself runs in a second pass, because it needs that file to
already exist. Give it its own template:

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

What comes out:

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

### Options

| Option | Effect |
|---|---|
| `lang=typescript` | Emit only the TypeScript overlay, and the shared `strict/types.ts` with it |
| `lang=python` | Emit only the Python overlay |
| `lang=openapi` | Emit only `openapi_config.yaml`, the protoc-gen-openapiv2 field options |
| *omitted* | Emit all three, into one tree |

Repeating it (`lang=typescript,lang=python`) selects both. Anything else is
rejected, so a typo fails the generation rather than quietly emitting nothing.

Two more lines are not optional:

- **`managed`**: the plugin is built on `protogen`, which insists on resolving a
  Go import path even when it emits no Go. Managed mode supplies one without
  putting `go_package` in your `.proto` files.
- **`strategy: all`**: without it buf forks the plugin once per directory. A
  TypeScript narrowing reaches across files, so the shard that could not see its
  target would silently drop it, and each shard would emit its own
  `strict/types.ts`.

## What it emits

**`<Message>Strict`** is the generated type with the rules applied. Narrowing
reaches through message-typed fields, singular and repeated, so a response type's
item list narrows along with it.

**`<Enum>Strict`** is the generated enum without the member protobuf numbers 0:

```ts
export type StockMovementKindStrict = Exclude<StockMovementKind, 0>;
```

Every field of that enum's type takes it, rule or no rule. This is the one
narrowing no `buf.validate` rule asked for — the convention behind `buf lint`'s
`ENUM_ZERO_VALUE_SUFFIX` is the justification, `<ENUM>_UNSPECIFIED` being "unset"
rather than a value. It narrows harder than protovalidate does; see
[Rule coverage: TypeScript](docs/rule-coverage-typescript.md#enums-the-excluded-zero).

**`<Message>StrictSchema`** is the *same* descriptor object, re-annotated so its
valid-type slot reports the strict type:

```ts
export const CreateProductRequestStrictSchema =
  CreateProductRequestSchema as GenMessage<CreateProductRequest, { validType: CreateProductRequestStrict }>;
```

Consumers of a Connect or gRPC client rarely name a request type: they hold a
descriptor and read the shape off it through `MessageValidType`. Retyping the
descriptor is how the narrowing reaches a call site that never writes the type
down. It is also what `createStrict` builds from, so every message that narrows
gets one.

**`<Service>Strict`** is the generated service descriptor, retyped so every
method points at the strict input and output schema.

**`createStrict(schema, init)`** builds a message from a `<Message>StrictSchema`
and reports its strict type. It is protobuf-es's own `create`, with the
initializer checked against what the rules narrowed each field to — so a literal
the compiler can already see fits is written as itself:

```ts
const del = createStrict(DeleteProductRequestStrictSchema, {
  id: "3fa85f64-5717-4562-b3fc-2c963f66afa6",   // the literal fits the UUID shape
});
```

A value only known at runtime is a bare `string`, which fits no shape. There is
no constructor to call: cast it — `id: fromForm as Uuid`, or `id: asUuid(fromForm)`
with the helper that is that same cast — and let protovalidate be the authority
on whether it really is one.

The initializer only has to carry what `create` would otherwise default past its
narrowing. A field a rule pins *to* that default — the read-only fields of a
create request, `this.product.id == ''` and its siblings — is left out and
filled in, since `create` already writes exactly what the rule asks for:

```ts
const created = createStrict(CreateProductRequestStrictSchema, {
  product: { sku: "ABC-1", name: "widget", price, status: ProductStatus.ACTIVE },
});                       // no id, no createdAt, no labels — create() writes them
```

**`strict/types.ts`** holds the string shape types and the mapped types the
strict types are built from. It is emitted once per generation.

**`openapi_config.yaml`** is the odd one out: not an overlay but the grpc-gateway
configuration that carries the rules into protoc-gen-openapiv2's output, one
JSONSchema per constrained field. Also once per generation, since its keys are
fully qualified proto names. See
[Rule coverage: OpenAPI](docs/rule-coverage-openapi.md).

## Example

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

  repeated string roles = 3 [
    (buf.validate.field).repeated.min_items = 1
  ];

  int32 age = 4 [(buf.validate.field).int32.gt = 0];

  Address address = 5;
}
```

```ts
// Code generated by protoc-gen-strict. DO NOT EDIT.
// source: example/v1/user.proto

import type { Email, Narrow, NonEmptyList } from "../../strict/types";
import type { Address, User } from "./user_pb";

/**
 * User demonstrates standard buf.validate rules alongside custom CEL rules.
 *
 * Left to runtime validation, having no type equivalent:
 *   id
 *     string.min_len = 5
 *     cel[user.id.prefix]: this.startsWith('usr_')
 *       message: id must start with 'usr_'
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
  address: AddressStrict;
}>;
```

`UserStrict` is assignable to `User`, so it goes straight into `create()`,
`toBinary()` or any function protobuf-es generated.

See [Rule coverage: TypeScript](docs/rule-coverage-typescript.md) for which rules
become types and what each turns into.

## Semantics

- **Runtime identity is unchanged.** The schema and service consts are the
  objects `protoc-gen-es` produced, re-annotated rather than rebuilt, so adopting
  a strict import cannot change what goes over the network or split a query cache.
- **A literal is checked by shape, not by rule.** `Uuid` is
  `` `${string}-${string}-${string}-${string}-${string}` ``: it accepts anything
  that looks like a UUID — five dash-separated groups — and checks neither the
  hex digits, nor the length bounds, nor the pattern. `string.min_len` carries
  nothing at all, since no string shape excludes the empty string.
  Under-narrowing is the safe direction here too.
- **A shape is not validation.** protovalidate remains the authority on validity;
  this plugin deliberately does not reimplement it, since
  [`@bufbuild/protovalidate`](https://github.com/bufbuild/protovalidate-es) and
  the [`protovalidate`](https://pypi.org/project/protovalidate/) PyPI package
  already run the same rules against the same descriptors.
- **The JSDoc is part of the output.** Each strict type names the rules it
  carries and the ones left to runtime validation. Read it before assuming a rule
  is enforced by the type.
- **An enum's zero member is the one deliberate over-narrowing.** protovalidate
  accepts `UNSPECIFIED` on a field no rule constrains; the strict type does not.
  Name the generated type instead of its overlay where a schema means to allow
  the unset member.

## Python

Python has no way to narrow a type: a protobuf message class is built by a
metaclass and exposes no structural type to intersect with. So `_strict.py`
emits a `typing.Annotated` alias per constrained field instead, which makes the
rules visible at the call site:

```python
from example.v1.user_strict import UserId

def promote(user_id: UserId) -> None: ...   # not `user_id: str`
```

A rule with an [annotated_types](https://github.com/annotated-types/annotated-types)
equivalent is carried as that constructor, which pydantic, msgspec and beartype
enforce. Everything else stays a metadata string, named but left to
protovalidate at runtime:

```python
UserDisplayName = Annotated[
    str,
    MinLen(3),
    "string.pattern = ^[a-zA-Z0-9_ ]+$",
]
```

That makes `annotated_types` a dependency of the generated Python — a pure
Python package with none of its own.

The overlay is annotations and nothing else. It imports a message class only
where an alias names one, and never re-exports: the classes stay in the
`_pb2` module protoc-gen-python wrote them in.

## What it understands

- **Every standard `buf.validate` rule.** Constraints are read reflectively, so
  new protovalidate rules come through without a code change, as documentation
  where they have no type equivalent.
- **Custom CEL rules**, both the full `(buf.validate.field).cel` form and the
  `cel_expression` shorthand. Message-level rules are translated into narrowings
  where their shape allows it; see [Rule coverage](docs/rule-coverage.md).
- **`oneof` exclusivity**, both real `oneof` blocks and
  `(buf.validate.message).oneof` declarations.
- **Nested messages**, including those declared inside another message.

A CEL expression that fails to parse is reported inline as
`cel[...] UNPARSEABLE: <reason>`. One broken rule does not abort generation for
the rest of the file.

## Current limitations

- Python carries only the bounds and lengths `annotated_types` spells; every
  other rule is a string there, and nothing reads it.
- `repeated.min_items = 3` narrows to "not empty", not to a 3-element tuple.
- Rules on `repeated.items` describe the element; the element type is left alone.
- A field with `ignore` set gets no narrowing at all, since under-narrowing is
  the safe direction. See [Rule coverage: TypeScript](docs/rule-coverage-typescript.md).
- The OpenAPI config carries no CEL, enum, `oneof` or `required` rule, and a
  swagger has no comment to name what was dropped. The zero enum member is still
  dropped there, by protoc-gen-openapiv2's own `omit_enum_default_value=true`.
- The Python overlay carries the excluded zero as the enum's own alias. A bare
  enum field, and any field whose enum comes from another proto file, keeps the
  class protoc-gen-python declared.
- Only TypeScript, Python and OpenAPI. Another target means another generator.

## Documentation

| Document | Covers |
|---|---|
| [Rule coverage](docs/rule-coverage.md) | What the plugin does with each rule, and which language carries what |
| [Rule coverage: TypeScript](docs/rule-coverage-typescript.md) | Every narrowing, what each rule turns into, and the structural limits |
| [Rule coverage: Python](docs/rule-coverage-python.md) | The `Annotated` aliases and how each TypeScript narrowing reads there |
| [Rule coverage: OpenAPI](docs/rule-coverage-openapi.md) | The rules that reach the swagger, as JSONSchema keywords |

## Contributing

Build, test and release instructions are in [CONTRIBUTING.md](CONTRIBUTING.md).

## License

[MIT](LICENSE).
