# Contributing

How to build, test and release `protoc-gen-strict`. For what the plugin does and
how to use it, see the [README](README.md).

## Setup

Requires Go 1.26+ and [`buf`](https://buf.build/docs/installation). `make verify`
also needs npm and python3.

```sh
make tools      # install staticcheck, which `make lint` needs
make build      # produces ./bin/protoc-gen-strict
```

## Targets

```sh
make help       # list every target
make check      # format, lint, tests, and a real end-to-end run — run this before a PR
make generate   # run every plugin over ./proto into ./gen/{typescript,python,openapiv2}
make verify     # compile the generated TypeScript and import the generated Python
make snapshot   # build the release archives into ./dist, without tagging
make clean      # remove bin/, gen/, dist/ and the verify scratch trees
```

The plugin reads a `CodeGeneratorRequest` on stdin, so `go run .` with no stdin
just hangs. That is not a bug; use `make generate`.

## Tests

`make test` includes golden tests that compare every generated file byte for byte
against a committed copy, so a change in the output shows up as a diff. Read the
diff before accepting it, then run `make testdata-update`. A `.proto` change needs
`make testdata` first, so the golden tests run against the new descriptors.

Golden tests prove the output did not change; they do not prove it is valid.
`make verify` does that: it runs `tsc --noEmit` over the real protoc-gen-es output
plus the overlays, and imports the generated Python for real. It needs network,
npm and python3, so it is not part of `make check`.

CI runs both on every push and pull request; see
[`.github/workflows/ci.yml`](.github/workflows/ci.yml).

## Commits

Commit messages follow [Conventional Commits](https://www.conventionalcommits.org):
`feat:`, `fix:`, `docs:`, `refactor:`, `test:`, `chore:`, with `!` or a
`BREAKING CHANGE:` footer for an incompatible change. CI lints every commit in a
PR, not just the title, because merges here are not squashed.

A change to the **generated output** is `feat:` or `fix:`, never `chore:` — every
consumer of this plugin sees it.

## Releases

[release-please](https://github.com/googleapis/release-please) reads those commits
off `main` and keeps a release PR open with the next version and the `CHANGELOG.md`
entry; merging that PR tags the release. GoReleaser then builds the binaries for
Linux, macOS and Windows on amd64 and arm64 and attaches them, with a
`checksums.txt`. Both steps live in
[`.github/workflows/release.yml`](.github/workflows/release.yml).

`make snapshot` runs the same build locally, into `./dist`, without tagging or
publishing anything.

## Internals

[Internals](docs/internals.md) covers the pipeline, the intermediate
representation, and how the narrowing works without the plugin knowing a single
target type.
