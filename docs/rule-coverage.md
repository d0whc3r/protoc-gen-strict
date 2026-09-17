# Rule coverage

What the plugin does with each `buf.validate` rule, and where the per-target
detail lives.

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

## Per target

| | TypeScript | Python | OpenAPI |
|---|---|---|---|
| Output | `<file>.strict.ts` | `<file>_strict.py` | `openapi_config.yaml`, one per run |
| Layered on | protoc-gen-es | protoc-gen-python, protoc-gen-pyi | protoc-gen-openapiv2, in a second pass |
| What it emits | `<Message>Strict`, the generated type with its constrained fields narrowed | one `typing.Annotated` alias per constrained field | a JSONSchema per constrained field, keyed by fully qualified name |
| Rules it carries | those with a structural equivalent: brands, `Extract`/`Exclude`, non-empty tuples, required keys | none; the type stays what protoc-gen-pyi declared | those with a JSONSchema keyword, bound included |
| Every other rule | named in the JSDoc, under "Left to runtime validation" | metadata on the alias, alongside the carried ones | dropped silently; a swagger has no comment to name it in |
| Which rules, exactly | [TypeScript](rule-coverage-typescript.md) | [Python](rule-coverage-python.md) | [OpenAPI](rule-coverage-openapi.md) |

The two overlays never drop a rule without saying so, which is what makes them
the readable record. OpenAPI is the exception by format, so a rule it cannot
express is only visible in the TypeScript or Python output.

## Adding a narrowing

A new narrowing lands in the parser first, as IR, then in each generator. It is
not done until:

- the rule has a row in the doc of every target that carries it,
- `internal/testdata/verify/assert.ts` asserts the new type actually bites,
- and a target that does not carry it still prints it, where it can.

See [Internals](internals.md) for the pipeline and the layering rule behind that
order.
