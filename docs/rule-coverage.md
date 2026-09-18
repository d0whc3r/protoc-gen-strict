# Rule coverage

What the plugin reads, what it does with each `buf.validate` rule, where the
per-target detail lives, and what it does not carry.

← [README](../README.md)

## The model

The parser reads rules reflectively, so the IR carries all of them: the standard
field rules flattened to dotted names (`string.min_len`, `repeated.items.string.uuid`),
the CEL rules with their parsed AST, and the oneof rules. What differs by target
is how much of that its output can hold.

- **A rule with an equivalent in the target becomes part of the output**, be it a
  narrowed TypeScript type or a JSONSchema keyword.
- **Every other rule is named in the generated file**, verbatim, wherever the
  target has somewhere to name it. A rule that stops being carried after a proto
  edit then shows up as a diff in `make generate` instead of quietly
  disappearing.
- **Under-narrowing is the safe direction.** A type that admits a value
  protovalidate rejects costs the runtime error that would have happened anyway.
  A type that rejects a value protovalidate accepts is a bug: it stops a caller
  from expressing something the schema allows.

protovalidate still validates everything at runtime, whatever the target. The
overlay moves what it can to the compiler; it does not replace the validator.

One annotation outside `buf.validate` is carried too:
`(google.api.field_behavior) = OUTPUT_ONLY` becomes metadata text in Python and
a `<Message>OutputOnlyFields` list of dotted proto paths in both TypeScript and
Python — the subtree under a server-assigned message field included — for the
code that has to subtract them from an update mask. The OpenAPI target leaves it alone,
since protoc-gen-openapiv2 already reads it.

## Per target

| | TypeScript | Python | OpenAPI |
|---|---|---|---|
| Output | `<file>.strict.ts` | `<file>_strict.py` | `openapi_config.yaml`, one per run |
| Layered on | protoc-gen-es | protoc-gen-python, protoc-gen-pyi | protoc-gen-openapiv2, in a second pass |
| What it emits | `<Message>Strict`, the generated type with its constrained fields narrowed | one `typing.Annotated` alias per constrained field | a JSONSchema per constrained field, keyed by fully qualified name |
| Rules it carries | those with a structural equivalent: string shape types, `Extract`/`Exclude`, non-empty tuples, required keys | those `annotated_types` spells: lengths and numeric bounds. The type itself stays what protoc-gen-pyi declared | those with a JSONSchema keyword, bound included |
| Every other rule | named in the JSDoc, under "Left to runtime validation" | metadata on the alias, alongside the carried ones | dropped silently; a swagger has no comment to name it in |
| Which rules, exactly | [TypeScript](rule-coverage-typescript.md) | [Python](rule-coverage-python.md) | [OpenAPI](rule-coverage-openapi.md) |

The two overlays never drop a rule without saying so, which is what makes them
the readable record. OpenAPI is the exception by format, so a rule it cannot
express is only visible in the TypeScript or Python output.

## What the parser reads

- **Every standard `buf.validate` rule.** Constraints are read reflectively, so
  a new protovalidate rule comes through without a code change — as documentation
  where it has no equivalent in the target.
- **Custom CEL rules**, both the full `(buf.validate.field).cel` form and the
  `cel_expression` shorthand. Message-level rules become narrowings where their
  shape allows it; see [TypeScript](rule-coverage-typescript.md#message-rules-bufvalidatemessagecel).
- **`oneof` exclusivity**, both real `oneof` blocks and
  `(buf.validate.message).oneof` declarations.
- **Nested messages**, including those declared inside another message.
- **`(google.api.field_behavior) = OUTPUT_ONLY`**, as described above.

A CEL expression that fails to parse is reported inline as
`cel[...] UNPARSEABLE: <reason>`. One broken rule does not abort generation for
the rest of the file.

## Current limitations

- Python carries only the bounds and lengths `annotated_types` spells; every
  other rule is a string there, and nothing reads it.
- `repeated.min_items = 3` narrows to "not empty", not to a 3-element tuple.
- Rules on `repeated.items` describe the element; the element type is left alone.
- A field with `ignore` set gets no narrowing at all, since under-narrowing is
  the safe direction.
- The OpenAPI config carries no CEL, enum, `oneof` or `required` rule, and a
  swagger has no comment to name what was dropped. The zero enum member is still
  dropped there, by protoc-gen-openapiv2's own `omit_enum_default_value=true`.
- The Python overlay carries the excluded zero as the enum's own alias. A bare
  enum field, and any field whose enum comes from another proto file, keeps the
  class protoc-gen-python declared.
- Only TypeScript, Python and OpenAPI. Another target means another generator.

## Adding a narrowing

A new narrowing lands in the parser first, as IR, then in each generator. It is
not done until:

- the rule has a row in the doc of every target that carries it,
- `internal/testdata/verify/assert.ts` asserts the new type actually bites,
- and a target that does not carry it still prints it, where it can.

See [Internals](internals.md) for the pipeline and the layering rule behind that
order.
