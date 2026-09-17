# Rule coverage: Python

What the Python overlay carries, and how each TypeScript narrowing reads here.

← [Rule coverage](rule-coverage.md) · [README](../README.md)

A protobuf message class is built by a metaclass and exposes no structural type
to intersect with. There is no `Narrow`, no `Require` and no brand to apply, so
`<file>_strict.py` carries the rules as data instead: one `typing.Annotated`
alias per constrained field, holding the type protoc-gen-pyi declared followed
by every rule on that field as a string. The message classes are re-exported, so
a caller needs one import for both.

```python
from example.v1.user_pb2 import (
    User as User,
)

UserEmail = Annotated[
    str,
    "required",
    "string.email = true",
]
```

The alias name is the message name and the field name, so `User.email` is
`UserEmail` and `Warehouse.Address.city` is `WarehouseAddressCity`. A field with
no rules gets no alias.

## The same rules, in Python

| TypeScript | Python |
|---|---|
| The rule becomes the type: `Uuid`, `NonEmptyList<...>`, `Exclude<...>` | The type stays what protoc-gen-pyi declared (`str`, `Sequence[str]`, `Mapping[str, str]`), and the rule is one metadata string next to it |
| `required` lifts the field into `Require<..., "f">` | `"required"`, the first metadata entry; the field is declared exactly as before |
| A message-typed field becomes `MoneyStrict` | It stays the plain class, `_common_pb2.Money`; there is no strict class to point at |
| A rule with no type equivalent goes to the JSDoc, under "Left to runtime validation" | There is no such list, and no "Carried into the type" line either: carried and not carried read alike, because nothing is carried |
| `(buf.validate.oneof).required` removes the `{ case: undefined }` arm | A comment above the message's aliases: `# required oneof key: exactly one of id, sku` |
| A message-level CEL rule narrows the fields on its path, or is reported | Always a comment above the message's aliases, with the expression, its message and the idents and functions the parse found |

Which makes [what TypeScript deliberately does not carry](rule-coverage-typescript.md#what-is-deliberately-not-carried)
a TypeScript-only list. In Python all of it is printed verbatim — `ignore`, the
rules under `repeated.items` and `map.keys` / `map.values`, and the exact bound
of every partial rule:

```python
PresenceRuleCoverageNeverChecked = Annotated[
    str,
    "ignore = IGNORE_ALWAYS",
]
ProductTags = Annotated[
    Sequence[str],
    "repeated.items.string.max_len = 40",
    "repeated.items.string.min_len = 1",
    "repeated.items.string.pattern = ^[a-z0-9-]+$",
    "repeated.max_items = 20",
    "repeated.unique = true",
]
```

None of it is enforced. `Annotated` metadata is inert, a type checker reads
`UserEmail` as `str`, and protovalidate keeps doing the validating at runtime.
What the overlay buys is the call site — `def promote(user_id: UserId)` instead
of `user_id: str` — and the diff: a rule that stops being carried shows up in
`make generate` instead of vanishing quietly.

## OpenAPI

No overlay here either. `lang=openapi` emits no source at all: it writes one
`openapi_config.yaml` for the whole run, the configuration
[protoc-gen-openapiv2](https://github.com/grpc-ecosystem/grpc-gateway) reads with
`openapi_configuration=<file>`. Each entry attaches a JSONSchema to a field by
fully qualified name, so the `.proto` files stay free of OpenAPI annotations and
the generated swagger is never patched after the fact.

| Rule | JSONSchema keyword |
|---|---|
| `string.min_len`, `max_len`, `len` | `minLength`, `maxLength`, both |
| `string.pattern` | `pattern` |
| `string.uuid`, `email`, `uri`, `hostname`, `ipv4`, `ipv6` | `format` |
| `int32`, `uint32`, `sint32`, `fixed32`, `float`, `double`: `.gte`, `.lte` | `minimum`, `maximum` |
| the same types' `.gt`, `.lt` | the bound, plus `exclusiveMinimum` or `exclusiveMaximum` |
| `repeated.min_items`, `max_items`, `unique` | `minItems`, `maxItems`, `uniqueItems` |
| `map.min_pairs`, `max_pairs` | `minProperties`, `maxProperties` |
| `required` | the field's JSON name, in the message's `required` |

A rule under `repeated.items` goes through the same table, and that is right by
construction: protoc-gen-openapiv2 puts every scalar keyword of an array field on
its `items`. So `repeated.items.string.pattern` lands where it belongs, which is
a rule the TypeScript overlay has no way to carry:

```json
"tags": {
  "type": "array",
  "items": { "type": "string", "maxLength": 40, "minLength": 1, "pattern": "^[a-z0-9-]+$" },
  "maxItems": 20,
  "uniqueItems": true
}
```

### What is deliberately not carried

- **A field with `ignore` set**, for the reason the TypeScript overlay drops it.
- **64-bit integer bounds.** JSON carries them as strings, and
  protoc-gen-openapiv2 types them `{"type": "string", "format": "int64"}`, where a
  `minimum` describes nothing.
- **`bytes` length rules.** They count raw bytes; the client sends base64.
- **`const`, `in` and `not_in`.** The IR holds rule values already rendered for
  display, and re-quoting a string out of that form is the same fragility the
  TypeScript side declined.
- **`required` on a list or a map.** protoc-gen-openapiv2 puts a `required` on
  the element schema for those, which says something else entirely.
- **`timestamp` and `duration` bounds, and every CEL rule.** No JSONSchema
  keyword describes them.

Two more precisions come from the plugin's own flags rather than from the rules,
and [buf.gen.openapi.yaml](../buf.gen.openapi.yaml) sets both:
`omit_enum_default_value=true` drops the `UNSPECIFIED` member, which is the
narrowing `enum.defined_only` asks for, and `use_allof_for_refs=true` keeps the
description of a message-typed field, which a sibling of `$ref` otherwise loses.

Because the configuration has to exist before protoc-gen-openapiv2 runs, the two
are separate `buf generate` passes: see `make generate`. protoc-gen-openapiv2
also has to run locally rather than as a BSR `remote:` plugin, since a remote
plugin executes on buf's servers and has no local file to read.
