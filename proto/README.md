# Test protos

Fixtures for the plugin. `shop/` is an invented storefront API whose only
purpose is to exercise every protovalidate rule family and every proto
construct the parser and the generators must handle.

| File | What it covers |
| --- | --- |
| `example/v1/user.proto` | Smallest end-to-end example: a few string/int rules, one full CEL rule, one `cel_expression` shorthand. |
| `shop/common/v1/common.proto` | Shared types. Enums, message-level CEL comparing sibling fields, map rules on keys and values, `optional` scalars, `enum.not_in`. |
| `shop/catalog/v1/product.proto` | Realistic CRUD resource. Read-only fields (`google.api.field_behavior`), cross-file type references, `FieldMask` partial update, a required `oneof` lookup key, pagination, `reserved` ranges, a service with `google.api.http` annotations, and message-level CEL guarding read-only fields on create. |
| `shop/inventory/v1/warehouse.proto` | Structure. Nested message definitions, real `oneof` with `(buf.validate.oneof).required`, message-level `oneof` rule over fields that are not in a oneof, `map` of messages, repeated messages, and the duration/timestamp rule families. |
| `shop/coverage/v1/rules.proto` | Not a realistic API. One message per rule family so a regression in the rule walk shows up in a golden file: every string well-known format, all twelve numeric types, bytes, bool, enum, the collection rules, the `ignore` modes, the well-known types, and every CEL shape (several rules on one field, both shorthands, macros, and the conjunct forms that become part of the generated type). Also the two RPC shapes the overlay has to resolve: a request declared in this file, and one declared in a file the plugin is never asked to generate. |

## Working with them

```sh
make generate        # build the plugin and run buf generate into ./gen
make test            # hermetic golden tests, no protoc or buf needed
make testdata-update # rebuild descriptors.binpb and rewrite the golden files
```

`make test` reads `internal/testdata/descriptors.binpb`, a checked-in
`FileDescriptorSet` built from this directory, so the Go tests do not depend on
`buf` or on network access to the BSR. Regenerate it with `make testdata`
whenever a `.proto` here changes, then review the golden diff.
