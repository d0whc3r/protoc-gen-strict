# protoc-gen-strict-schema

Reads `buf.validate` rules and emits runtime schemas that enforce them: JSON
Schema modules on top of protoschema-jsonschema's output, and Zod modules
checked by protovalidate-es where Zod cannot say a rule exactly.

← [README](../../README.md) · [Rule coverage: runtime schemas](../../docs/rule-coverage-schema.md)

## Install

```sh
go install github.com/d0whc3r/protoc-gen-strict/cmd/protoc-gen-strict-schema@latest
```

Or, in a JavaScript project, from npm:

```sh
npm install --save-dev @d0whc3r/protoc-gen-strict-schema
```

npm installs the prebuilt binary for the machine, Linux, macOS or Windows on x64
or arm64, and links `protoc-gen-strict-schema` into `node_modules/.bin`. Run buf
from an npm script or as `npx buf generate`, which put that directory on buf's
`PATH`.

Or take it from the [release archive](https://github.com/d0whc3r/protoc-gen-strict/releases),
which carries both binaries. Keep the name when you put it on your `PATH`: buf
resolves the plugin by it.

Requires [`buf`](https://buf.build/docs/installation), plus Go 1.26+ if you
install with `go install`.

`protoc-gen-strict-schema --version` prints the version. Anything else is buf's
job: the plugin reads a `CodeGeneratorRequest` on stdin.

`target=json` also needs protoschema-plugins' `protoc-gen-jsonschema` at the
version the plugin was written against, on the `PATH` buf runs it with. The npm
package does not carry it:

```sh
go install github.com/bufbuild/protoschema-plugins/cmd/protoc-gen-jsonschema@v0.6.0
```

Other plugins ship under the same name (chrusty/protoc-gen-jsonschema,
cerbos/protoc-gen-jsonschema). The plugin asks for `--version` and fails the
generation unless the answer is `v0.6.0`; `jsonschema_plugin=<path>` points it
at the right one.

## Configure

```yaml
# buf.gen.yaml
version: v2
managed:
  enabled: true
  override:
    - file_option: go_package_prefix
      value: github.com/you/yourrepo/gen
plugins:
  - remote: buf.build/bufbuild/es:v2.15.0
    out: gen/typescript
    opt: target=ts

  - local: protoc-gen-strict-schema
    out: gen/typescript
    opt: target=json+zod
    strategy: all
```

For JavaScript with declarations, give both entries the same output and import
extension:

```yaml
  - remote: buf.build/bufbuild/es:v2.15.0
    out: gen/js
    opt: [target=js+dts, import_extension=js]
  - local: protoc-gen-strict-schema
    out: gen/js
    opt: [target=json+zod, output=js+dts, import_extension=js]
    strategy: all
```

protoc-gen-strict's `lang=typescript` entry can write into the same
`gen/typescript`; no file names collide.

| Setting                  | Where        | Effect                                                                            |
| ------------------------ | ------------ | --------------------------------------------------------------------------------- |
| `target=json`            | `opt:`       | Emit `<file>.schema.ts` and `strict/jsonschema.ts`                                |
| `target=zod`             | `opt:`       | Emit `<file>.zod.ts` for Zod 4, `strict/protovalidate.ts` and `strict/wkt.zod.ts` |
| `target=zod3`            | `opt:`       | `<file>.zod3.ts` for Zod 3, `strict/protovalidate.ts` and `strict/wkt.zod3.ts`    |
| `target=` omitted        | `opt:`       | `json+zod`                                                                        |
| `output=ts`, `js`, `dts` | `opt:`       | Write each module as `.ts`, `.js`, `.d.ts`, joined with `+`; `ts` when omitted    |
| `import_extension=js`    | `opt:`       | Add `.js` to every relative import, for Node's ESM loader; `none` by default      |
| `jsonschema_plugin=`     | `opt:`       | The protoc-gen-jsonschema executable `target=json` runs; `protoc-gen-jsonschema`  |
| `strategy: all`          | plugin entry | **Required.** One plugin process for the whole request                            |
| `managed: enabled: true` | top level    | **Required.** Supply the Go import path protogen insists on resolving             |

Targets are joined with `+`, the syntax protoschema-jsonschema uses for its own
`target`: `target=json+zod3`. Repeating `target=` adds to the set.
`target=zod+zod3` writes both majors into one tree: their modules are named
apart and share `strict/protovalidate.ts`. An unknown or empty value fails the
generation rather than emitting nothing.

The output mirrors the `.proto` path, `example/v1/user.zod.ts` for
`example/v1/user.proto`, next to protoc-gen-es's `example/v1/user_pb.ts`: every
module imports by relative path (`./user_pb`, `../../strict/protovalidate`,
`../../jsonschema/example.v1.User.jsonschema.bundle.json`). `paths=` has no
effect.

Why the two settings are required:

- **`strategy: all`.** The `strict/` modules are written once per run, and
  `strict/jsonschema.ts` holds the fixes of every message the run's bundles
  reach. Under directory sharding each shard writes its own.
- **`managed`.** protogen resolves a Go import path for every file, though the
  plugin emits no Go.

### What each target needs

| Target | In the same tree                                                                             | Runtime dependencies of the generated code                            |
| ------ | -------------------------------------------------------------------------------------------- | --------------------------------------------------------------------- |
| `json` | nothing: the plugin runs protoc-gen-jsonschema itself and writes the bundles under `jsonschema/` | none                                                                  |
| `zod`  | protoc-gen-es output (`*_pb.ts`)                                                             | `zod@^4.6`, `@bufbuild/protovalidate@^1.3`, `@bufbuild/protobuf@^2.8` |
| `zod3` | protoc-gen-es output (`*_pb.ts`)                                                             | `zod@^3.25` or `zod@^4.6`, and the same protovalidate and protobuf    |

- `@bufbuild/protobuf@^2.8` is protovalidate-es's peer range. `make verify`
  pins zod 4.6.5 and 3.25.76, protovalidate-es 1.3.0 and protobuf-es 2.15.0.
- For `json`, the plugin runs `protoc-gen-jsonschema` over the same request with
  `target=json-bundle` and writes its bundles under `jsonschema/`, failing the
  generation when it cannot trust them: see
  [`<message>JsonSchema`](../../docs/rule-coverage-schema.md#messagejsonschema).
- The `json` modules import each bundle with an import attribute,
  `with { type: "json" }`, which Node's ESM loader requires and bundlers accept.
  The consumer's `tsconfig` needs `resolveJsonModule: true` and a `module`
  setting that accepts import attributes: `esnext`, `nodenext` or `preserve`.
- Relative imports carry no file extension by default, as protoc-gen-es's own
  do, which bundlers and `moduleResolution: bundler` resolve. Node's ESM loader
  needs `import_extension=js`, on both entries.
- `output=js` writes code without types, `output=dts` the declarations, so a
  JavaScript project gets the same checks as a TypeScript one. The generator
  writes the `.d.ts` itself; `make verify` checks it matches what tsc infers for
  the `.ts`.
- The Zod targets need no protoschema-jsonschema. The `zod3` modules import
  `zod/v3`, which zod@^3.25 and zod@^4 both ship, so `target=zod+zod3` runs
  with zod 4 installed. The `zod` modules import `zod`, and need zod 4.

### What lands where

```
gen/typescript/example/v1/user_pb.ts                               protoc-gen-es
gen/typescript/example/v1/user.schema.ts                           target json
gen/typescript/example/v1/user.zod.ts                              target zod
gen/typescript/example/v1/user.zod3.ts                             target zod3
gen/typescript/strict/jsonschema.ts                                target json, once per run
gen/typescript/strict/protovalidate.ts                             target zod | zod3, once per run
gen/typescript/strict/wkt.zod.ts                                   target zod, once per run
gen/typescript/strict/wkt.zod3.ts                                  target zod3, once per run
gen/typescript/jsonschema/example.v1.User.jsonschema.bundle.json   target json: protoc-gen-jsonschema's bundle
```

- One module per `.proto`, mirroring its path. A file without a top-level
  message gets no `.schema.ts`; one without a message or an enum gets no
  `.zod.ts` or `.zod3.ts`.
- protoc-gen-jsonschema names every bundle after its `$id`, flat. The plugin
  writes them under `jsonschema/`, which keeps them in one place.
- With `output=js` or `output=dts` the same modules end in `.js` or `.d.ts`.
- `google/protobuf/*` gets no module, even under `--include-imports`:
  protobuf-es writes none for it, so there is no descriptor to import.
  `strict/wkt.zod.ts` and `strict/wkt.zod3.ts` serve the well-known types.
- Identifiers follow protoc-gen-es's local names: `Warehouse.Address` becomes
  `warehouseAddressZod`.
- The modules export schemas and nothing else. The `strict/` helpers are
  implementation detail, not API.

## Using the output

`<message>JsonSchema` is a JSON Schema 2020-12 document. Hand it to any 2020-12
validator, here ajv:

```ts
import Ajv2020 from "ajv/dist/2020";
import addFormats from "ajv-formats";
import { userJsonSchema } from "./gen/typescript/example/v1/user.schema";

const ajv = new Ajv2020({ strict: false });
addFormats(ajv);
const validateUser = ajv.compile(userJsonSchema);
```

`<message>Zod` is the schema to parse with; `<message>ZodObject` is the one to
compose from:

```ts
import { z } from "zod";
import { userZod, userZodObject } from "./gen/typescript/example/v1/user.zod";

const user = userZod.parse(json); // every rule, the message-wide ones included
type User = z.infer<typeof userZod>;

const Profile = userZodObject.pick({ displayName: true, roles: true });
```

What each export carries, and what it leaves to protovalidate:
[Rule coverage: runtime schemas](../../docs/rule-coverage-schema.md).
