# Changelog

## [1.1.2](https://github.com/d0whc3r/protoc-gen-strict/compare/v1.1.1...v1.1.2) (2026-09-24)


### Bug Fixes

* Upgrade CI actions and CEL dependency versions ([7944468](https://github.com/d0whc3r/protoc-gen-strict/commit/79444688ec49eda4cd830199e4a8e59f129ab7e1))

## [1.1.1](https://github.com/d0whc3r/protoc-gen-strict/compare/v1.1.0...v1.1.1) (2026-09-24)


### Bug Fixes

* lower go version to 1.26 ([2e562c4](https://github.com/d0whc3r/protoc-gen-strict/commit/2e562c4398fe389b485b296d0d7950a86f34cee0))

## [1.1.0](https://github.com/d0whc3r/protoc-gen-strict/compare/v1.0.0...v1.1.0) (2026-09-18)


### Features

* Add enum zero exclusion and OUTPUT_ONLY narrowing ([b04cb50](https://github.com/d0whc3r/protoc-gen-strict/commit/b04cb504231ed024b5bdbf3dcee3edb6fef4aae0))
* add plugin entrypoint with lang option dispatch ([7bb4859](https://github.com/d0whc3r/protoc-gen-strict/commit/7bb4859310d6c1d16872253ff7a3c47a348b8195))
* add protovalidate fixture protos for the storefront API ([f0bf587](https://github.com/d0whc3r/protoc-gen-strict/commit/f0bf587cb1d9ded857f501eafbc50daa5d986524))
* Carry Python bounds as annotated_types metadata ([1fea7bb](https://github.com/d0whc3r/protoc-gen-strict/commit/1fea7bbe00e2c991d9319044f4923020b34cfad3))
* **generator:** emit TypeScript and Python strict overlays and the OpenAPI config ([1546a09](https://github.com/d0whc3r/protoc-gen-strict/commit/1546a09e3562a8b778bfcbaaad7a4ab933e66baa))
* **parser:** parse buf.validate rules and CEL expressions into the IR ([7b00b8c](https://github.com/d0whc3r/protoc-gen-strict/commit/7b00b8cd7df8f4ba8d76baeb748326753a61b92f))
* Split generators by target and tighten CEL handling ([473f665](https://github.com/d0whc3r/protoc-gen-strict/commit/473f665f8d81233849413e9679f3363c0f1533f3))


### Bug Fixes

* Avoid Python alias re-export name collisions ([3ef0482](https://github.com/d0whc3r/protoc-gen-strict/commit/3ef0482037265cbe56172e5f92e784796cb0c0af))
* **deps:** update google.golang.org/genproto/googleapis/api digest to eeb232e ([#2](https://github.com/d0whc3r/protoc-gen-strict/issues/2)) ([928fb0b](https://github.com/d0whc3r/protoc-gen-strict/commit/928fb0b051121d4c45cadf825e5706b82a2a4ab5))
* Stop making OUTPUT_ONLY fields readonly in TS ([0111d60](https://github.com/d0whc3r/protoc-gen-strict/commit/0111d6043f20b07e9ba4d7f53bdc18fdcf2625dc))

## 1.0.0 (2026-09-18)


### Features

* Add enum zero exclusion and OUTPUT_ONLY narrowing ([b04cb50](https://github.com/d0whc3r/protoc-gen-strict/commit/b04cb504231ed024b5bdbf3dcee3edb6fef4aae0))
* add plugin entrypoint with lang option dispatch ([7bb4859](https://github.com/d0whc3r/protoc-gen-strict/commit/7bb4859310d6c1d16872253ff7a3c47a348b8195))
* add protovalidate fixture protos for the storefront API ([f0bf587](https://github.com/d0whc3r/protoc-gen-strict/commit/f0bf587cb1d9ded857f501eafbc50daa5d986524))
* Carry Python bounds as annotated_types metadata ([1fea7bb](https://github.com/d0whc3r/protoc-gen-strict/commit/1fea7bbe00e2c991d9319044f4923020b34cfad3))
* **generator:** emit TypeScript and Python strict overlays and the OpenAPI config ([1546a09](https://github.com/d0whc3r/protoc-gen-strict/commit/1546a09e3562a8b778bfcbaaad7a4ab933e66baa))
* **parser:** parse buf.validate rules and CEL expressions into the IR ([7b00b8c](https://github.com/d0whc3r/protoc-gen-strict/commit/7b00b8cd7df8f4ba8d76baeb748326753a61b92f))
* Split generators by target and tighten CEL handling ([473f665](https://github.com/d0whc3r/protoc-gen-strict/commit/473f665f8d81233849413e9679f3363c0f1533f3))


### Bug Fixes

* Avoid Python alias re-export name collisions ([3ef0482](https://github.com/d0whc3r/protoc-gen-strict/commit/3ef0482037265cbe56172e5f92e784796cb0c0af))
* **deps:** update google.golang.org/genproto/googleapis/api digest to eeb232e ([#2](https://github.com/d0whc3r/protoc-gen-strict/issues/2)) ([928fb0b](https://github.com/d0whc3r/protoc-gen-strict/commit/928fb0b051121d4c45cadf825e5706b82a2a4ab5))
* Stop making OUTPUT_ONLY fields readonly in TS ([0111d60](https://github.com/d0whc3r/protoc-gen-strict/commit/0111d6043f20b07e9ba4d7f53bdc18fdcf2625dc))
