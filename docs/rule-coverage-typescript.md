# Rule coverage: TypeScript

What the TypeScript overlay emits, which `buf.validate` rules become part of the
generated types, what each turns into, and what is left to runtime validation.

← [Rule coverage](rule-coverage.md) · [README](../README.md)

## What it emits

Per `.proto` file, one `<file>.strict.ts` next to the `protoc-gen-es` output:

- **`<Message>Strict`** — the generated type with its constrained fields
  narrowed. Narrowing reaches through message-typed fields, singular and
  repeated, so a response type's item list narrows along with it.
- **`<Enum>Strict`** — the generated enum without its zero member; see
  [Enums: the excluded zero](#enums-the-excluded-zero).
- **`<Message>StrictSchema`** — the *same* descriptor object, re-annotated so
  its valid-type slot reports the strict type:

  ```ts
  export const CreateProductRequestStrictSchema =
    CreateProductRequestSchema as GenMessage<CreateProductRequest, { validType: CreateProductRequestStrict }>;
  ```

  Consumers of a Connect or gRPC client rarely name a request type: they hold a
  descriptor and read the shape off it through `MessageValidType`. Retyping the
  descriptor is how the narrowing reaches a call site that never writes the type
  down. It is also what `createStrict` builds from, so every message that
  narrows gets one.
- **`<Service>Strict`** — the generated service descriptor, retyped so every
  method points at the strict input and output schema.

Plus **`strict/types.ts`**, once per generation: the string shape types and the
mapped types every narrowing is built from.

Because the schemas are the objects `protoc-gen-es` produced, re-annotated
rather than rebuilt, runtime identity is unchanged: adopting a strict import
cannot change what goes over the network or split a query cache.

### Building a message

`createStrict` only needs the fields that `create` would otherwise default past
the narrowing. A field a rule pins *to* that default — the read-only fields of a
create request, `this.product.id == ''` and its siblings — can be left out,
since `create` already writes exactly what the rule asks for:

```ts
const created = createStrict(CreateProductRequestStrictSchema, {
  product: { sku: "ABC-1", name: "widget", price, status: ProductStatus.ACTIVE },
});                       // no id, no createdAt, no labels — create() writes them
```

A value only known at runtime is a bare `string`, which fits no shape. There is
no constructor to call: cast it — `id: fromForm as Uuid` — and let protovalidate
be the authority on whether it really is one.

## The building blocks

Every narrowing is built from the exports of the generated `strict/types` module:

- The string shape types **`Uuid`**, `` `${string}-${string}-${string}-${string}-${string}` ``,
  and **`Email`**, `` `${string}@${string}.${string}` ``. A template literal type
  is the whole check: a literal of the right shape is assignable and a misspelt
  one is not, with no constructor to call and nothing to brand. It is a shape
  check and nothing more — protovalidate stays the authority on validity. A value
  only known at runtime is a bare `string`, which fits no shape; that one needs a
  cast.
- **`Narrow<T, M>`**, which replaces the type of the named properties and keeps
  every other property of `T` as it is.
- **`Require<T, K>`**, which is `T & Required<Pick<T, K>>`.
- **`NonEmptyList<T>`**, the tuple `[T[number], ...T[number][]]`.
- **`createStrict(schema, init)`**, which builds a message from a
  `<Message>StrictSchema` and reports its strict type. It checks the initializer
  against the narrowing, so a string literal the compiler can see fits the shape
  is accepted and a bare `string` is not. A field a rule pins to the
  value `create` writes — `Uuid & ""`, `Extract<Enum, 0>`, `never` — may be left
  out, and takes only that value when written.

Only `Narrow` is hand-written, and only because the standard library has no way
to say it. The obvious spelling, `Omit<T, keyof M> & M`, drops the `?` of every
property it overrides. A `description?: string` documented with a rule would come
back mandatory, and a field a CEL rule marks absent would go from "may be
omitted, cannot be set" to "must be present, and nothing satisfies it". `Narrow`
is homomorphic over `keyof T`, so optionality and readonly survive.

Its constraint on `M` also makes every override prove it narrows the property it
replaces, which is what `make verify` tests the emitter with.

A message with any narrowing becomes `Narrow<<Message>, { ... }>`. Required
fields wrap that result: `Require<Narrow<...>, "a" | "b">`.

## Field rules: `(buf.validate.field)`

| Rule | Field type becomes |
|---|---|
| `string.uuid = true` | `Uuid` |
| `string.email = true` | `Email` |
| `enum.not_in` | `Exclude<EStrict, ...>` over the listed numbers |
| `enum.in`, `enum.const` | `Extract<EStrict, ...>` |
| `repeated.min_items` of at least one | `NonEmptyList<M["f"]>` |
| `required` | see below |

A message-typed field is replaced by the strict type of its target, if that
target has any narrowing of its own: `Money` becomes `MoneyStrict`, and a
repeated `Product` becomes `ProductStrict[]`.

The enum rules work without this plugin learning a single member name: a numeric
TypeScript enum member is a subtype of its numeric literal, and protovalidate
stores those rule values as plain `int32`, so `Extract<FlavorStrict, 1 | 2>`
filters the generated enum directly. Every other base type is an indexed access
into the generated type for the same reason; see [Internals](internals.md).

`string.min_len` carries nothing. No string shape excludes the empty string, and
saying so would take a nominal type with a constructor as the only way to produce
it — which is what a caller then has to call on every value. The bound is
reported under "Left to runtime validation" instead.

### Enums: the excluded zero

This is the one narrowing no `buf.validate` rule asked for. Every enum the run
generates gets a strict type of its own, without the member protobuf numbers 0:

```ts
/** StockMovementKind classifies a change in stock level. … */
export type StockMovementKindStrict = Exclude<StockMovementKind, 0>;
```

Every field of that enum's type takes it, **rule or no rule**. The convention
behind `buf lint`'s `ENUM_ZERO_VALUE_SUFFIX` is the whole justification: the
member named `<ENUM>_UNSPECIFIED` is "unset", not a value.

It narrows harder than protovalidate does. A field with no `required`,
`enum.not_in` or `enum.in` rule accepts `UNSPECIFIED` at runtime and the strict
type rejects it. Reach for the generated type rather than its strict overlay
where a schema means to allow the unset member.

An enum rule stacks on top of it — `Exclude<StockMovementKindStrict, 3>` — so
the zero stays out however the rule is written, and an `enum.not_in = [0]` that
only repeats the exclusion is folded away. An enum declared in a file the run
was not asked to generate has no strict type, and its fields keep the type
protoc-gen-es gave them.

### `required`

`required` forbids the zero value, and what that means depends on whether the
field tracks presence, which in proto3 is only true of message fields, `oneof`
members and fields with the `optional` label.

| Field | `required` becomes |
|---|---|
| A message field, or one labelled `optional` | `Require<..., "f">`, and `NonNullable<M["f"]>` if nothing else replaced the type |
| A plain enum | Already carried: the enum's strict type excludes the zero |
| A plain `repeated` | `NonEmptyList<M["f"]>` |
| Anything else | Left to runtime validation |

The last row is strings, numerics, bytes and maps: their zero is a value the
generated type has no way to name apart from the rest.

## Message rules: `(buf.validate.message).cel`

Only conjunctions (`&&`) of these shapes, on paths rooted at `this`:

| CEL | Field type becomes |
|---|---|
| `this.x == ''` | intersected with `""` |
| `this.x != ''` | Left to runtime, for the same reason `string.min_len` is |
| `this.x == 0` on an enum | `Extract<EStrict, 0>`, which is `never`: the rule demands the member the enum's strict type rules out |
| `this.x == 0` on anything else | Left to runtime: `0` is a number, a bigint or a duration depending on a choice protoc-gen-es made, not this plugin |

Both argument orders are read, so `'' == this.id` translates like
`this.id == ''`.

`has()` reads differently per field, because CEL defines it that way. Where the
field tracks presence — a message, a `oneof` member, an `optional` label — it
asks whether the field was set. Everywhere else it is "not the default value",
and says nothing about the property being there:

| CEL | Field tracks presence | Field does not |
|---|---|---|
| `has(this.x)` | `NonNullable<M["x"]>`, and the field is lifted into `Require<..., "x">` | left to runtime: it reads as `!= ''` on a string, and nothing at all elsewhere |
| `!has(this.x)` | `never`; the field stays optional, so it can be omitted but not set | a `string` is intersected with `""`, an enum becomes `Extract<..., 0>`; anything else is left to runtime |

The `NonNullable` matters: `Require` drops the `?` but not an `undefined`
written into the property type itself, which protoc-gen-es does emit.

Presence and a value bound on one field are separate constraints, and each is
carried on its own: `has(this.nickname) && this.nickname != ''` narrows to
`NonNullable<M["nickname"]>` and lifts `nickname` into `Require`, while the
`!= ''` half is reported under "Left to runtime validation". Two conjuncts that
both narrow the *value* of one field contradict each other; the first is carried
and the second reported the same way.

A term **intersects** with what the field already had rather than replacing it.
That keeps `Narrow`'s constraint satisfied by construction, and it says the truth
when the two disagree: a `string.uuid` field that a CEL rule also requires to be
empty becomes `Uuid & ""`, which nothing inhabits. That is not a bug in the
plugin: the proto says two things that cannot both hold, and the type shows it.

Everything else (a disjunction, a comparison between two fields, a macro) is
reported in the JSDoc of the type, conjunct by conjunct, under "Left to runtime
validation".

### Nested paths

A path may be nested, as in `this.product.id`. Each segment produces its own
wrapper, so the narrowing nests to match:

```proto
// create_product.no_read_only_fields
expression: "this.product.id == '' && !has(this.product.created_at)
          && !has(this.product.updated_at) && this.product.version == 0"
```

```ts
export type CreateProductRequestStrict = Require<Narrow<CreateProductRequest, {
  product: Narrow<ProductStrict, { id: ProductStrict["id"] & ""; createdAt: never; updatedAt: never; }>;
  idempotencyKey: Uuid;
}>, "product">;
```

The inner base is `ProductStrict`, not `Product`: the nested narrowing is applied
*on top of* the target's own rules, so `Product`'s UUID and enum rules are still
in force inside it. `this.product.version == 0` is not carried; `version` is an
`int64`, which protoc-gen-es types as `bigint`.

A path reaching *through* a message from a file the plugin was not asked to
generate (`has(this.inner.mask.paths)`, where `mask` is a
`google.protobuf.FieldMask`) resolves against the descriptors but has no overlay
to narrow, so the whole conjunct is left to runtime validation and named in the
JSDoc. Pass that file to the plugin to carry it.

A segment that is not a singular message ends the translation: a path reaching
through a list or a map describes elements, which this plugin does not narrow.

A segment naming a `oneof` member ends it too. protoc-gen-es declares the oneof
as one discriminated-union property, so its members are not properties to
narrow, and the conjunct is left to runtime validation.

## `oneof`

A real `oneof` is a discriminated union in the protoc-gen-es output, with a
`{ case: undefined }` arm for "nothing set". `(buf.validate.oneof).required`
removes that arm:

```ts
key: Exclude<GetProductRequest["key"], { case: undefined }>;
```

Rules on the members cannot narrow an arm, so they are listed in the JSDoc of
that property instead. Without `required` the union stands as protoc-gen-es
declared it and there is no property to document, so the member rules are listed
under "Left to runtime validation" on the type itself.

A `(buf.validate.message).oneof` rule names fields that are ordinary properties,
with no union to constrain, so it is reported as left to runtime validation.

## Server-assigned fields: `google.api.field_behavior`

The one annotation here that is not a `buf.validate` rule.
`(google.api.field_behavior) = OUTPUT_ONLY` is AIP-203 for "the server assigns
this, a caller must not set it".

It narrows no type: the field keeps whatever protoc-gen-es gave it, optionality
included. It is carried as a list instead, so code that has to *name* a field —
a `google.protobuf.FieldMask`, a form, a diff — can read it. Dotted proto paths,
the form a FieldMask carries, not the camelCase property:

```ts
export const ProductOutputOnlyFields = [
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
] as const;
```

The server that assigns a message field assigns everything under it, and a mask
path can name a member of it. So the list carries the whole subtree, and a
message that declares no `OUTPUT_ONLY` field of its own still gets the paths it
reaches through the ones it wraps:

```ts
export const CreateProductRequestOutputOnlyFields = [
  "product.id",
  "product.created_at",
  // …
] as const;
```

A repeated or map field ends a path: a FieldMask may not name a member of one.

```ts
const paths = updateMask.paths.filter(
  (path) => !(ProductOutputOnlyFields as readonly string[]).includes(path),
);
```

A field inside a `oneof` is listed too.

The other `field_behavior` values are not carried. `REQUIRED` is
`(buf.validate.field).required`'s job, and protovalidate is what enforces it;
`IMMUTABLE` and `INPUT_ONLY` describe a transition between two messages, which a
single type cannot see.

## What is deliberately not carried

- **A field with `ignore` set.** `IGNORE_ALWAYS` drops its rules outright;
  `IGNORE_IF_ZERO_VALUE` admits the zero value alongside whatever they allow,
  which leaves `min_len` and `min_items` saying nothing and turns the rest into a
  union with the zero. Rather than reason per rule, the whole field is left to
  runtime validation. Under-narrowing is the safe direction.

  `IGNORE_ALWAYS` on a message field goes further: protovalidate stops recursing
  into the message, so the field keeps the generated type rather than the
  target's `<Name>Strict`. Requiring the strict type there would reject a value
  the proto accepts.
- **`const`, `in` and `not_in` on strings and numbers.** Only the enum forms are
  carried; the rest would need the rule values re-quoted out of their rendered
  form, which is fragile for little gain.
- **Rules under `repeated.items` and `map.keys` / `map.values`.** They describe
  the element, and protoc-gen-es types a map as an index signature.
- **The exact bound of a partial rule.** `min_items: 3` becomes plain
  `NonEmptyList`; the number stays in the JSDoc.
  Each property lists every rule on its field and then names the ones the type
  carries, so the gap is visible rather than implied.
