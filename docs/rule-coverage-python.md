# Rule coverage: Python

What the Python overlay carries, and how each TypeScript narrowing reads here.

← [Rule coverage](rule-coverage.md) · [README](../README.md)

A protobuf message class is built by a metaclass and exposes no structural type
to intersect with. There is no `Narrow`, no `Require` and no brand to apply, so
`<file>_strict.py` carries the rules as metadata instead: one `typing.Annotated`
alias per constrained field, holding the type protoc-gen-pyi declared followed
by every rule on that field. The message classes are not re-exported: a caller
imports those from the `_pb2` module protoc-gen-python wrote them in.

A rule with an [annotated_types](https://github.com/annotated-types/annotated-types)
equivalent is carried as that constructor. That vocabulary is what pydantic,
msgspec and beartype all read, so those rules are enforced rather than merely
described — which makes `annotated_types` a dependency of the generated code. It
is pure Python and has none of its own.

The rest stay strings: named, so nothing disappears quietly, and left to
protovalidate at runtime.

```python
from annotated_types import MaxLen, MinLen

UserEmail = Annotated[
    str,
    "required",
    "string.email = true",
]
UserId = Annotated[
    str,
    MaxLen(64),
    MinLen(5),
    "cel[user.id.prefix]: this.startsWith('usr_')",
]
```

## What reaches annotated_types

| Rule | Constructor |
|---|---|
| `string.min_len`, `bytes.min_len` | `MinLen` |
| `string.max_len`, `bytes.max_len` | `MaxLen` |
| `string.len`, `bytes.len` | `MinLen` and `MaxLen` at the same value, which is what `Len(n, n)` unpacks into |
| `repeated.min_items`, `map.min_pairs` | `MinLen` |
| `repeated.max_items`, `map.max_pairs` | `MaxLen` |
| every numeric type's `.gt`, `.gte`, `.lt`, `.lte` | `Gt`, `Ge`, `Lt`, `Le` |

The bound is emitted as the parser printed it. Unlike the OpenAPI target there
is no 64-bit exclusion and no rounding: a Python `int` is arbitrary precision,
and protoc-gen-pyi types every integer width as `int`.

### What is deliberately not carried

- **`string.min_bytes` and `max_bytes`.** They count UTF-8 bytes; `MinLen` on a
  `str` counts code points, which is the wrong quantity for a non-ASCII value.
- **Rules under `repeated.items`, `map.keys` and `map.values`.** They describe an
  element, and the alias annotates the collection — a `MinLen` there would count
  the wrong thing.
- **`duration` and `timestamp` bounds.** Their value is a message, and the parser
  prints one rule per sub-field (`duration.gte.seconds`), which is not a number
  to compare the field against.
- **A field with `ignore` set,** for the reason the other targets drop it: the
  sibling rules do not always apply, so none of them describes the type either.
- **A reversed numeric range,** where protovalidate reads a lower bound above the
  upper one as "outside that range". `Gt(20)` and `Lt(10)` next to each other are
  a conjunction no value satisfies, which narrows harder than the proto asked.
- **`float.finite`, `string.pattern` and the format rules.** `annotated_types` has
  `IsFinite` and `Predicate`, but no type checker or validator reads them widely
  enough to be worth the over-narrowing risk.
- **`const`, `in` and `not_in`.** The IR holds rule values already rendered for
  display, and re-parsing a list out of that form is the same fragility the other
  targets declined.

## Server-assigned fields: `google.api.field_behavior`

`(google.api.field_behavior) = OUTPUT_ONLY` is AIP-203 for "the server assigns
this, a caller must not set it". `annotated_types` has no vocabulary for it, so
it is carried as metadata text — and it is the only annotation that gives an
otherwise ruleless field an alias of its own:

```python
ProductCreatedAt = Annotated[
    _google_protobuf_timestamp_pb2.Timestamp,
    "google.api.field_behavior = OUTPUT_ONLY",
]
```

The names are also emitted apart from the aliases, as the tuple
`<Message>OutputOnlyFields`, so an update mask can subtract them:

```python
ProductOutputOnlyFields = (
    "id",
    "created_by_email",
    "created_at",
    "created_at.seconds",
    "created_at.nanos",
    "updated_at",
    "updated_at.seconds",
    "updated_at.nanos",
    "published_at",
    "published_at.seconds",
    "published_at.nanos",
    "version",
)

mask.paths[:] = [p for p in mask.paths if p not in ProductOutputOnlyFields]
```

They are dotted paths: the server that assigns a message field assigns
everything under it, and a mask path can name a member of it. A message that
declares no `OUTPUT_ONLY` field of its own still gets the paths it reaches —
`CreateProductRequestOutputOnlyFields` is `("product.id", "product.created_at",
…)`. A repeated or map field ends a path, since a FieldMask may not name a
member of one.

## Enums: the excluded zero

The one alias no `buf.validate` rule produced. Every enum the run generates gets
a `<Name>Strict` of its own, without the member protobuf numbers 0:

```python
# StockMovementKind classifies a change in stock level.
# Without STOCK_MOVEMENT_KIND_UNSPECIFIED, the member protobuf numbers 0, …
StockMovementKindStrict = Annotated[
    StockMovementKind,
    Ge(1),
]
```

`Ge(1)` is the whole narrowing, and it is real enforcement: protoc-gen-pyi
declares the class as an `int` subclass, so "at least 1" is the same set as "not
the zero member". The convention behind `buf lint`'s `ENUM_ZERO_VALUE_SUFFIX` is
the justification — the member named `<ENUM>_UNSPECIFIED` is "unset", not a
value — so it holds with or without a rule, and it narrows harder than
protovalidate does.

Two cases fall back to a metadata string instead of `Ge(1)`, because "at least
1" would then be the wrong set: an enum numbering a member below zero, and an
enum whose only member is the zero.

Every field alias of that enum's type is annotated with the strict alias rather
than the bare class, so `Money.currency` is `Annotated[CurrencyStrict, …]`.

**Where Python stops short of TypeScript.** The alias is what carries the
exclusion, and a field with no rules of its own gets no alias — so a bare enum
field is left with the class protoc-gen-python declared, where the TypeScript
overlay would have retyped it. Nor does an enum declared in another proto file
reach its alias: the overlays do not import one another, so such a field keeps
the bare class too.

## Naming

The alias name is the message name and the field name, so `User.email` is
`UserEmail` and `Warehouse.Address.city` is `WarehouseAddressCity`. A field with
no rules gets no alias. An enum's alias is its own name plus `Strict`, and it
holds that name: a field alias is what gets renamed around it.

A name a class in the same file already holds gets a trailing underscore
instead: the enum typing `Product.status` is itself named `ProductStatus`, and
the alias has to import it, so the alias is `ProductStatus_` and the class keeps
the name protoc-gen-python gave it.

A type from another file is reached through a module alias built from the whole
proto path — `shop/common/v1/common.proto` is `_shop_common_v1_common_pb2`. The
base name protoc-gen-pyi uses would collapse `a/common.proto` and
`b/common.proto` onto one alias, where the second import silently rebinds the
first.

## The same rules, in Python

| TypeScript | Python |
|---|---|
| The rule becomes the type: `Uuid`, `NonEmptyList<...>`, `Exclude<...>` | The type stays what protoc-gen-pyi declared (`str`, `Sequence[str]`, `Mapping[str, str]`), and the rule is one metadata entry next to it |
| `required` lifts the field into `Require<..., "f">` | `"required"`, the first metadata entry; the field is declared exactly as before |
| A message-typed field becomes `MoneyStrict` | It stays the plain class, `_shop_common_v1_common_pb2.Money`; there is no strict class to point at |
| Every enum field becomes `EStrict`, the enum without its zero member | The enum gets the same `EStrict` alias, carried as `Ge(1)`; only a field that already has an alias is annotated with it |
| A rule with no type equivalent goes to the JSDoc, under "Left to runtime validation" | There is no such list: a constructor is what "carried" looks like here, and a string is what "left to runtime" looks like |
| `(buf.validate.oneof).required` removes the `{ case: undefined }` arm | A comment above the message's aliases: `# required oneof key: exactly one of id, sku` |
| A message-level CEL rule narrows the fields on its path, or is reported | Always a comment above the message's aliases, with the expression, its message and the idents and functions the parse found |

So [what TypeScript deliberately does not carry](rule-coverage-typescript.md#what-is-deliberately-not-carried)
is still named here, verbatim, alongside the constructors that are:

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
    MaxLen(20),
    "repeated.unique = true",
]
```

A type checker still reads `UserEmail` as `str`: `Annotated` metadata changes no
static type, and protovalidate stays the authority at runtime. What the overlay
buys is the enforcement any annotated_types reader applies, the call site — `def
promote(user_id: UserId)` instead of `user_id: str` — and the diff: a rule that
stops being carried shows up in `make generate` instead of vanishing quietly.

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
  `repeated.items.ignore` does the same for the element rules alone.
- **A reversed numeric range,** where protovalidate reads a lower bound above the
  upper one as "outside that range" and JSONSchema has no way to say "or".
- **`required`,** which grpc-gateway would also hoist onto the flattened query
  parameter of a GET. See [Rule coverage: OpenAPI](rule-coverage-openapi.md).
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
