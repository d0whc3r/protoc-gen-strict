# AGENTS.md

Behavioral rules for coding agents working on `protoc-gen-strict`.

**What this repo is:** a protoc/buf plugin. It reads protovalidate (`buf.validate`)
rules, custom CEL expressions included, off proto descriptors and emits a strict
overlay on the code the official generators produce: protoc-gen-es for
TypeScript, protoc-gen-python for Python, plus a field-options file for
protoc-gen-openapiv2. Rules that have an equivalent in the target become part of
its output; the rest are named in the generated doc comment.

```
main.go              plugin entrypoint, protogen.Options{}.Run
internal/parser      descriptors + buf.validate extensions → MessageMetadata (IR)
internal/generator   IR → TypeScript / Python overlay source, OpenAPI config
proto/               fixture protos; `internal/testdata` holds the golden output
```

**Never re-declare a proto type.** The official generator already decided how
`int64`, an enum or `google.protobuf.Timestamp` maps to the target language. A
generator here derives its types from that output (`User["roles"]`) or imports
them; it does not carry a type mapping of its own. A second opinion is a bug.

**Under-narrowing is the safe direction.** A rule translated in part, or not at
all, has to be named in the generated JSDoc under "Left to runtime validation".
A rule that quietly stops being carried is worse than one that was never carried.
Adding a narrowing means adding a row to the coverage doc of every target that
carries it ([TypeScript](docs/rule-coverage-typescript.md),
[Python](docs/rule-coverage-python.md),
[OpenAPI](docs/rule-coverage-openapi.md)) and an assertion to
`internal/testdata/verify/assert.ts`.

## 1. Think Before Coding

**Read first. Don't assume. Don't hide confusion.**

- Read the code the change touches, every caller and the real flow, before editing.
- State your assumptions explicitly. Uncertain → ask.
- Multiple valid interpretations → present them, don't pick silently.
- A simpler approach exists → say so. Push back when warranted.
- Unclear or contradictory → stop, name what's confusing, ask. Don't guess and continue.

## 2. Simplicity First

**Minimum code that solves the problem. Nothing speculative.**

- No features beyond the request.
- No abstraction with one caller: inline it.
- No interface with one implementation. Go interfaces belong at the consumer, defined where they're used, not shipped alongside the only type that satisfies them.
- No config for a value that never changes: hardcode it.
- No error handling for impossible states.
- 200 lines that could be 50 → rewrite it.

Reuse before writing: a helper already in this codebase, the stdlib, or an
already-required module beats new code. Adding a `go.mod` dependency for what a
few lines can do is a design change; ask first.

The test: would a senior engineer call this overcomplicated? Then simplify.

## 3. Surgical Changes

**Touch only what you must. Clean up only your own mess.**

- Don't reformat, rename or "improve" adjacent code, comments or tests.
- Don't refactor working code as a side effect. Match existing style, even if you'd do it differently.
- Unrelated dead code → mention it, don't delete it.
- Remove the imports and variables YOUR change orphaned; leave pre-existing dead code alone.

Fix the root cause, not the symptom: one guard in the shared function beats a
guard in every caller, and patching only the reported path leaves the sibling
callers broken. A bug that shows up in the TypeScript output is usually a parser
bug that the Python generator has too.

The test: every changed line traces directly to the request.

## 4. Verify Before Done

**Define the check first. Not done until it passes.**

Turn the task into something verifiable:

- "Support rule X" → a `parser` test asserting X lands in the IR, then the generator change.
- "Fix the bug" → reproducing test first. Watch it fail, then fix, then watch it pass.
- "Refactor X" → tests pass before and after.

Multi-step work → state the plan up front:

```
1. [Step] → verify: [check]
2. [Step] → verify: [check]
```

**The checks, in order:**

```sh
gofmt -l .          # must print nothing
go vet ./...
staticcheck ./...
make test           # go test ./...
```

Changed what the generated types look like → `make verify` too. It compiles the
overlay against the real protoc-gen-es output and imports the real generated
Python, which is the only check that catches an overlay that no longer resolves.
It needs network, npm and python3.

Changed the generated output → the golden tests in `make test` will fail with a
diff. **Read the diff before accepting it.** Once it is what you meant, run
`make testdata-update`. A golden update you commit without reading makes the
golden tests worthless.

Changed a `.proto` → `make testdata` first, so the golden tests run against the
new descriptors.

Touched a `.proto` → `buf lint`.

"Looks right" is not verification. Run the check and report the real result.
Tests failed, or a step was skipped → say so, with the output.

## 5. Git & PR

**Commits, pushes and PR replies are visible to the team. Each needs an explicit request.**

- Never commit unless asked. Wait, even with changes staged.
- Never push unless asked.
- Never add a co-author.
- Never reply to PR comments unless asked. Draft it, let the human post.

Commit messages are [Conventional Commits](https://www.conventionalcommits.org):
`feat:`, `fix:`, `docs:`, `refactor:`, `test:`, `chore:`, with `!` or a
`BREAKING CHANGE:` footer for an incompatible change. release-please reads them
off `main` to pick the next version and write `CHANGELOG.md`, so the wrong type
ships the wrong version number. CI lints every commit in a PR, not just the
title: merges here are not squashed, so each one lands on `main` as written. A change to the **generated output** is `feat:`
or `fix:`, never `chore:` — every consumer of this plugin sees it.

Never commit `bin/`, `gen/` or `dist/`. All three are build artifacts; `make
clean` removes them.

## 6. Code Style

**Readable at a glance. Flat, named, spaced.**

- Extract recurring or meaningful values into named constants. A value that comes from a spec (a protovalidate rule name, a language keyword list) gets a constant even with one use. Self-explanatory one-offs stay inline.
- Flat over nested. Early `return` / `continue` instead of an `else` pyramid.
- Function names under 30 characters.
- No boolean parameters. A named type or two distinct functions read; `emit(f, true)` doesn't.
- Let the reader breathe: blank line between logical blocks, one short comment saying _what_ the block does and _why_. ASCII diagram when a whole flow needs explaining.
- Every exported symbol gets a doc comment starting with its own name; the existing code does this, match it. Struct fields that encode a convention get a trailing comment with an example, like `JSONName string // JSON/camelCase name, e.g. "userId"`.
- Wrap errors with context: `fmt.Errorf("parse %s: %w", field, err)`. Never return a bare `err` that loses the proto path the reader needs to find the offending field.
- Export nothing that nothing imports. `internal/` already fences these packages from the outside world; within it, lowercase until another package needs the symbol. Widening one is a design change; ask first.
- Respect the layers: `parser` → IR → `generator`. Generators never touch descriptors or `buf.validate` extensions; they consume `parser.MessageMetadata` and nothing else. The parser never emits target-language text. New rule support lands in the IR first, then in each generator.

## 7. Codegen Rules

**Generated output is a contract. Unstable output is a broken build.**

- **Deterministic or it's a bug.** Never range over a map to produce output: protobuf descriptors and extension sets hand you unordered data. Collect, `sort`, then emit. Two runs on the same proto must be byte-identical.
- **Never panic on bad input.** Descriptor options are frequently nil and CEL expressions are frequently malformed. That's user input, not an invariant. A malformed expression is reported (`CELRule.ParseError`), not returned as an error and not a crash.
- **Escape what you interpolate.** Proto comments, CEL source and string rule values land inside TypeScript and Python literals. Anything user-authored crossing into generated source gets escaped for that target language.
- **Every target or none.** A rule the parser extracts but only TypeScript renders is a silent hole in the Python and OpenAPI output. Say so explicitly if you're deliberately leaving one behind.
- **The plugin reads stdin.** protoc and buf drive it over a `CodeGeneratorRequest`. `go run .` with no stdin just hangs. That's not a bug; use `make generate`.

## 8. Words

**Fewest words that carry the meaning.**

- Comments, commit messages, PR bodies, replies to prompts: cut every word that isn't load-bearing.
- No superlatives, no praise, no "you're absolutely right". Cold, factual.

---

**Working if:** smaller diffs, fewer overcomplication rewrites, clarifying questions before implementation instead of corrections after.

<!-- CODEGRAPH_START -->

## CodeGraph

In repositories indexed by CodeGraph (a `.codegraph/` directory exists at the repo root), reach for it BEFORE grep/find or reading files when you need to understand or locate code:

- **MCP tool** (when available): `codegraph_explore` answers most code questions in one call — the relevant symbols' verbatim source plus the call paths between them, including dynamic-dispatch hops grep can't follow. Name a file or symbol in the query to read its current line-numbered source. If it's listed but deferred, load it by name via tool search.
- **Shell** (always works): `codegraph explore "<symbol names or question>"` prints the same output.

If there is no `.codegraph/` directory, skip CodeGraph entirely — indexing is the user's decision.
<!-- CODEGRAPH_END -->
