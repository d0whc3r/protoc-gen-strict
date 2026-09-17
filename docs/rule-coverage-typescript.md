# Rule coverage: TypeScript

Which `buf.validate` rules become part of the generated TypeScript types, what
they turn into, and what is left to runtime validation.

← [Rule coverage](rule-coverage.md) · [README](../README.md)

## The building blocks

Every narrowing is built from the exports of the generated `strict/types` module:

- The nominal string types **`Uuid`**, **`Email`** and **`NonEmpty`**, each with
  its own `unique symbol` so `Uuid & NonEmpty` stays inhabitable. Under a shared
  key that intersection would resolve to `"Uuid" & "NonEmpty"`, which is `never`.
  Their constructors are generic in the argument so they compose:
  `nonEmpty(uuid(value))` produces `Uuid & NonEmpty`.
- **`Narrow<T, M>`**, which replaces the type of the named properties and keeps
  every other property of `T` as it is.
- **`Require<T, K>`**, which is `T & Required<Pick<T, K>>`.
- **`NonEmptyList<T>`**, the tuple `[T[number], ...T[number][]]`.

Only `Narrow` is hand-written, and only because the standard library has no way
to say it. The obvious spelling, `Omit<T, keyof M> & M`, drops the `?` of every
property it overrides. A `description?: string` documented with a rule would come
back mandatory, and a field a CEL rule marks absent would go from "may be
omitted, cannot be set" to "must be present, and nothing satisfies it". `Narrow`
is homomorphic over `keyof T`, so optionality and readonly survive.

Its constraint on `M` also makes every override prove it narrows the property it
replaces, which is what `make verify` tests the emitter with.

A message with any narrowing becomes `Narrow<<Message>, { ... }>`. If a rule also
makes fields required, that wraps the result: `Require<Narrow<...>, "a" | "b">`.

## Field rules: `(buf.validate.field)`

| Rule | Field type becomes |
|---|---|
| `string.uuid = true` | `Uuid` |
| `string.email = true` | `Email` |
| `string.min_len` of at least one | `NonEmpty` |
| `enum.not_in` | `Exclude<M["f"], ...>` over the listed numbers |
| `enum.in`, `enum.const` | `Extract<M["f"], ...>` |
| `repeated.min_items` of at least one | `NonEmptyList<M["f"]>` |
| `required` | see below |

A message-typed field is replaced by the strict type of its target, if that
target has any narrowing of its own: `Money` becomes `MoneyStrict`, and a
repeated `Product` becomes `ProductStrict[]`.

The enum rules work without this plugin learning a single member name: a numeric
TypeScript enum member is a subtype of its numeric literal, and protovalidate
stores those rule values as plain `int32`, so `Exclude<StockMovement["kind"], 0>`
removes the `UNSPECIFIED` member directly. Every base type is an indexed access
into the generated type for the same reason; see [Internals](internals.md).

`NonEmpty` is dropped when `Uuid` or `Email` is already there: neither is ever
empty, so the second brand would only cost the caller a constructor.

### `required`

`required` forbids the zero value, and what that means depends on whether the
field tracks presence, which in proto3 is only true of message fields, `oneof`
members and fields with the `optional` label.

| Field | `required` becomes |
|---|---|
| A message field, or one labelled `optional` | `Require<..., "f">`, and `NonNullable<M["f"]>` if nothing else replaced the type |
| A plain `string` | `NonEmpty`; protovalidate counts the empty string as unset |
| A plain enum | `Exclude<M["f"], 0>` |
| A plain `repeated` | `NonEmptyList<M["f"]>` |
| Anything else | Left to runtime validation |

The last row is numerics, bytes and maps: their zero is a value the generated
type has no way to name apart from the rest.

## Message rules: `(buf.validate.message).cel`

Only conjunctions (`&&`) of these shapes, on paths rooted at `this`:

| CEL | Field type becomes |
|---|---|
| `this.x == ''` | intersected with `""` |
| `this.x != ''` | intersected with `NonEmpty` |
| `this.x == 0` on an enum | `Extract<..., 0>`, the zero-numbered member |
| `this.x == 0` on anything else | Left to runtime: `0` is a number, a bigint or a duration depending on a choice protoc-gen-es made, not this plugin |

Both argument orders are read, so `'' == this.id` translates like
`this.id == ''`.

`has()` reads differently per field, because CEL defines it that way. Where the
field tracks presence — a message, a `oneof` member, an `optional` label — it
asks whether the field was set. Everywhere else it is "not the default value",
and says nothing about the property being there:

| CEL | Field tracks presence | Field does not |
|---|---|---|
| `has(this.x)` | `NonNullable<M["x"]>`, and the field is lifted into `Require<..., "x">` | a `string` is intersected with `NonEmpty`; anything else is left to runtime |
| `!has(this.x)` | `never`; the field stays optional, so it can be omitted but not set | a `string` is intersected with `""`, an enum becomes `Extract<..., 0>`; anything else is left to runtime |

The `NonNullable` matters: `Require` drops the `?` but not an `undefined`
written into the property type itself, which protoc-gen-es does emit.

Presence and a value bound on one field are separate constraints and both hold:
`has(this.nickname) && this.nickname != ''` narrows to
`NonNullable<M["nickname"]> & NonEmpty` *and* lifts `nickname` into `Require`.
Two conjuncts that both narrow the *value* of one field contradict each other;
the first is carried and the second reported under "Left to runtime validation".

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
- **The exact bound of a partial rule.** `min_len: 3` becomes plain `NonEmpty`
  and `min_items: 3` becomes plain `NonEmptyList`; the number stays in the JSDoc.
  Each property lists every rule on its field and then names the ones the type
  carries, so the gap is visible rather than implied.
