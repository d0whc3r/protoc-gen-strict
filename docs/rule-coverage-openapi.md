# Rule coverage: OpenAPI

Which `buf.validate` rules reach the generated swagger, and what each becomes.

← [Rule coverage](rule-coverage.md) · [README](../README.md)

Not an overlay. protoc-gen-openapiv2 reads no protovalidate rule, so every
constraint is missing from the swagger it emits. It does accept an external
configuration, `openapi_configuration=<file>`, whose `field` entries attach a
JSONSchema to a field by fully qualified name. This plugin writes that file:

```yaml
openapiOptions:
  field:
    - field: shop.catalog.v1.Product.sku
      option:
        maxLength: 32
        minLength: 3
        pattern: "^[A-Z0-9]+(-[A-Z0-9]+)*$"
```

The `.proto` files stay free of OpenAPI annotations and the generated JSON is
never patched. Two properties fall out of the format:

- **One file for the whole run**, `openapi_config.yaml`. The keys are fully
  qualified names, so there is nothing per-directory to shard, which is why the
  plugin entry needs `strategy: all`.
- **Two passes.** The config has to exist before protoc-gen-openapiv2 runs, so
  `buf.gen.yaml` writes it and `buf.gen.openapi.yaml` runs the generator over
  it. `make generate` runs them in order.

## Field rules

| Rule | Becomes |
|---|---|
| `string.uuid`, `string.email`, `string.uri`, `string.hostname`, `string.ipv4`, `string.ipv6` | `format` |
| `string.len` | `minLength` and `maxLength`, both |
| `string.min_len`, `string.max_len` | `minLength`, `maxLength` |
| `string.pattern` | `pattern`, the RE2 source quoted |
| `gte`, `lte` on a 32-bit integer, `float` or `double` | `minimum`, `maximum` |
| `gt`, `lt` on a 32-bit integer | the inclusive bound one step away: `gt: 0` is `minimum: 1` |
| `gt`, `lt` on a `float` or `double` | `minimum` + `exclusiveMinimum`, `maximum` + `exclusiveMaximum` |
| `repeated.min_items`, `repeated.max_items` | `minItems`, `maxItems` |
| `repeated.unique = true` | `uniqueItems` |
| `map.min_pairs`, `map.max_pairs` | `minProperties`, `maxProperties` |
| `repeated.items.<rule>` | whatever the same table gives `<rule>`: protoc-gen-openapiv2 puts a scalar keyword of an array field on its `items` |

Unlike the TypeScript overlay, the exact bound survives. `min_len: 3` is
`minLength: 3`, not a plain "non-empty".

## What is not carried

- **64-bit integer bounds** (`int64`, `uint64`, `fixed64`, and the rest). JSON
  carries them as strings and protoc-gen-openapiv2 types them
  `{"type": "string", "format": "int64"}`, where a `minimum` describes nothing.
- **`bytes` rules.** Their length rules count raw bytes, not the base64 a client
  sends.
- **Enum rules.** None has an entry. The second pass does set
  `omit_enum_default_value=true`, which drops the `UNSPECIFIED` member from every
  enum, but that is a generator flag rather than anything a rule asked for.
- **`const`, `in` and `not_in`,** on every type.
- **CEL rules,** field-level and message-level, and the `oneof` rules. A
  cross-field constraint has nowhere to go in a per-field JSONSchema.
- **A bound of zero.** grpc-gateway's swagger writer tags `minimum` and
  `maximum` `omitempty`, so `minimum: 0` never reaches the output — and the
  `exclusiveMinimum: true` it would leave behind describes nothing. Only a whole
  number escapes this, by stepping to the inclusive bound. The same goes for the
  zero of every other keyword: `max_len: 0` is dropped, `min_len: 0` says
  nothing anyway.
- **`required`.** grpc-gateway hoists a field's `required` into the parent
  message's required list, which reads correctly in a body schema. The same list
  also marks the *flattened query parameter* of a GET required — `minPrice.currency`
  becomes mandatory on a request whose `minPrice` filter is optional. Over-
  narrowing a request is worse than leaving the rule to runtime validation, so
  the rule is not carried at all.
- **Every rule on a field with `ignore` set,** the same call the TypeScript
  overlay makes: the rules do not always apply, so none of them describes the
  schema.

Nothing is reported for what it drops. A swagger has no comment to name a rule
in, which is what makes [Rule coverage: TypeScript](rule-coverage-typescript.md)
and [Rule coverage: Python](rule-coverage-python.md) the readable record of a
rule the type does not carry.

## Two details

**A keyword two rules both claim keeps the first.** `string.len` next to
`string.min_len` is a proto contradicting itself, and a duplicate YAML key would
fail to parse at all. Rules arrive in the parser's order, sorted by rule name.

**Bounds are re-rendered as plain decimals.** protoreflect prints a large float
in exponent form (`1e+06`), which is not a number to the YAML parser grpc-gateway
loads the config with.
