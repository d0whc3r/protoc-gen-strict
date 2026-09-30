BIN := bin/protoc-gen-strict
SCHEMA_BIN := bin/protoc-gen-strict-schema
# Lazily evaluated, and scoped to the module: `go list` skips dot-directories,
# so installed agent skills under .agents/ never reach the formatters.
GO_DIRS = $(shell go list -f '{{.Dir}}' ./...)

.DEFAULT_GOAL := help

.PHONY: help
help: ## Display this help.
	@awk 'BEGIN {FS = ":.*##"; printf "\nUsage:\n  make \033[36m<target>\033[0m\n"} /^[a-zA-Z_0-9\/-]+:.*?##/ { printf "  \033[36m%-15s\033[0m %s\n", $$1, $$2 } /^##@/ { printf "\n\033[1m%s\033[0m\n", substr($$0, 5) } ' $(MAKEFILE_LIST)

##@ Build & generate

.PHONY: build
build: ## Compile both plugin binaries into ./bin.
	go build -o $(BIN) ./cmd/protoc-gen-strict
	go build -o $(SCHEMA_BIN) ./cmd/protoc-gen-strict-schema

.PHONY: snapshot
snapshot: ## Build the release archives into ./dist — no tag, no publish.
	goreleaser release --snapshot --clean

.PHONY: generate
generate: build install/protoc-gen-jsonschema ## Run every plugin over ./proto into ./gen/{typescript,python,openapiv2}.
	@rm -rf gen  # buf's `clean` only empties each `out`, not a tree a layout change left behind
	PATH="$(CURDIR)/bin:$$PATH" buf generate
	# Second pass: protoc-gen-openapiv2 reads the config the first pass wrote.
	buf generate --template buf.gen.openapi.yaml
	@echo "--- generated files ---"
	@find gen -type f | sort

VERIFY := internal/testdata/verify
# Zod 3 and Zod 4 cannot both be installed as `zod`, so the zod3 target has a
# project of its own, on zod 3. $(VERIFY) type-checks it against zod 4's zod/v3.
VERIFY_ZOD3 := internal/testdata/verify-zod3

.PHONY: verify
verify: build install/protoc-gen-jsonschema ## Compile the generated TypeScript and JavaScript, run the schema checks and import the generated Python — needs network, npm and python3.
	@rm -rf $(VERIFY)/gen $(VERIFY)/gen-js $(VERIFY)/venv $(VERIFY_ZOD3)/gen $(VERIFY_ZOD3)/gen-js
	PATH="$(CURDIR)/bin:$$PATH" buf generate --include-imports -o $(VERIFY)
	buf generate --template $(VERIFY)/buf.gen.json.yaml -o $(VERIFY)
	PATH="$(CURDIR)/bin:$$PATH" buf generate --include-imports --template $(VERIFY)/buf.gen.js.yaml -o $(VERIFY)
	cd $(VERIFY) && npm install --silent --no-audit --no-fund && npx tsc --noEmit && npx tsc --noEmit -p tsconfig.dts.json && npx tsx assert.schema.ts && node assert.node.mjs
	PATH="$(CURDIR)/bin:$$PATH" buf generate --include-imports --template $(VERIFY_ZOD3)/buf.gen.yaml -o $(VERIFY_ZOD3)
	PATH="$(CURDIR)/bin:$$PATH" buf generate --include-imports --template $(VERIFY_ZOD3)/buf.gen.js.yaml -o $(VERIFY_ZOD3)
	cd $(VERIFY_ZOD3) && npm install --silent --no-audit --no-fund && npx tsc --noEmit && npx tsc --noEmit -p tsconfig.dts.json && npx tsx assert.ts && node assert.node.mjs
	python3 -m venv $(VERIFY)/venv
	$(VERIFY)/venv/bin/pip install -q protobuf annotated_types
	$(VERIFY)/venv/bin/python $(VERIFY)/assert.py

##@ Test

.PHONY: test
test: ## Run Go unit tests, including the hermetic golden tests.
	go test ./...

.PHONY: testdata
testdata: install/protoc-gen-jsonschema ## Rebuild the descriptor set and the JSON Schema bundles the golden tests run against.
	buf build --as-file-descriptor-set -o internal/testdata/descriptors.binpb
	# What protoc-gen-jsonschema writes for the same files, so the golden tests
	# of target=json need no executable.
	buf generate --template internal/testdata/buf.gen.jsonschema.yaml

.PHONY: testdata-update
testdata-update: testdata ## Rebuild the descriptor set and rewrite the golden files.
	go test ./internal/generator -update

.PHONY: check
check: fmt-check lint test generate ## Format, lint, tests, and a real end-to-end run — before a PR.

##@ Format & lint

.PHONY: fmt
fmt: bootstrap ## Rewrite Go sources with gofmt, goimports and gci.
	$(GOLANGCI_LINT) fmt $(GO_DIRS)

.PHONY: fmt-check
fmt-check: bootstrap ## Fail if any Go source is unformatted.
	$(GOLANGCI_LINT) fmt --diff $(GO_DIRS)

.PHONY: lint
lint: bootstrap ## Run golangci-lint and buf lint.
	$(GOLANGCI_LINT) run
	buf lint

##@ Dependencies

.PHONY: tidy
tidy: ## Sync go.mod/go.sum with the imports actually used.
	go mod tidy

.PHONY: update
update: ## Bump Go modules and buf deps to their latest versions.
	go get -u ./...
	go mod tidy
	buf dep update

## Location to install dependencies to
LOCALBIN ?= $(shell pwd)/bin
$(LOCALBIN):
	mkdir -p $(LOCALBIN)

## Tool Binaries
GOLANGCI_LINT = $(LOCALBIN)/golangci-lint
# The executable protoc-gen-strict-schema runs for target=json. The version is
# jsonSchemaVersion in internal/generator/schemagen/bundles.go, which the
# plugin requires.
PROTOC_GEN_JSONSCHEMA = $(LOCALBIN)/protoc-gen-jsonschema

## Tool Versions
GOLANGCI_LINT_VERSION ?= v2.12.2
PROTOC_GEN_JSONSCHEMA_VERSION ?= v0.6.0

.PHONY: bootstrap
bootstrap: install/golangci-lint install/protoc-gen-jsonschema ## Install required dependencies to work with this project.

.PHONY: install/golangci-lint
install/golangci-lint: $(GOLANGCI_LINT) ## Download golangci-lint locally if necessary.
$(GOLANGCI_LINT): $(LOCALBIN)
	$(call go-install-tool,$(GOLANGCI_LINT),github.com/golangci/golangci-lint/v2/cmd/golangci-lint,$(GOLANGCI_LINT_VERSION))

.PHONY: install/protoc-gen-jsonschema
install/protoc-gen-jsonschema: $(PROTOC_GEN_JSONSCHEMA) ## Download protoc-gen-jsonschema locally if necessary.
$(PROTOC_GEN_JSONSCHEMA): $(LOCALBIN)
	$(call go-install-tool,$(PROTOC_GEN_JSONSCHEMA),github.com/bufbuild/protoschema-plugins/cmd/protoc-gen-jsonschema,$(PROTOC_GEN_JSONSCHEMA_VERSION))

# copied from kube-builder
# go-install-tool will 'go install' any package with custom target and name of binary, if it doesn't exist
# $1 - target path with name of binary
# $2 - package url which can be installed
# $3 - specific version of package
define go-install-tool
@[ -f "$(1)-$(3)" ] || { \
set -e; \
package=$(2)@$(3) ;\
echo "Downloading $${package}" ;\
rm -f $(1) || true ;\
GOBIN=$(LOCALBIN) go install $${package} ;\
mv $(1) $(1)-$(3) ;\
} ;\
ln -sf $(1)-$(3) $(1)
endef

##@ Housekeeping

.PHONY: clean
clean: ## Remove build and generation artifacts.
	rm -rf bin gen dist $(VERIFY)/gen $(VERIFY)/gen-js $(VERIFY)/venv $(VERIFY)/node_modules $(VERIFY_ZOD3)/gen $(VERIFY_ZOD3)/gen-js $(VERIFY_ZOD3)/node_modules
