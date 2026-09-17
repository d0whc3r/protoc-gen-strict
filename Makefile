BIN := bin/protoc-gen-strict
# Lazily evaluated, and scoped to the module: `go list` skips dot-directories,
# so installed agent skills under .agents/ never reach gofmt.
GO_DIRS = $(shell go list -f '{{.Dir}}' ./...)

.DEFAULT_GOAL := help
.PHONY: help build snapshot generate verify test testdata testdata-update check fmt fmt-check lint tidy update tools clean

## help: list every target
help:
	@grep -hE '^## ' $(MAKEFILE_LIST) | sed 's/^## /  /' | sort

# --- build & generate -------------------------------------------------------

## build: compile the plugin binary into ./bin
build:
	go build -o $(BIN) .

## snapshot: build the release archives into ./dist — no tag, no publish
snapshot:
	goreleaser release --snapshot --clean

## generate: run every plugin over ./proto into ./gen/{typescript,python,openapiv2}
generate: build
	@rm -rf gen  # buf's `clean` only empties each `out`, not a tree a layout change left behind
	PATH="$(CURDIR)/bin:$$PATH" buf generate
	# Second pass: protoc-gen-openapiv2 reads the config the first pass wrote.
	buf generate --template buf.gen.openapi.yaml
	@echo "--- generated files ---"
	@find gen -type f | sort

VERIFY := internal/testdata/verify

## verify: compile the generated TypeScript and import the generated Python — needs network, npm and python3
verify: build
	@rm -rf $(VERIFY)/gen $(VERIFY)/venv
	PATH="$(CURDIR)/bin:$$PATH" buf generate --include-imports -o $(VERIFY)
	cd $(VERIFY) && npm install --silent --no-audit --no-fund && npx tsc --noEmit
	python3 -m venv $(VERIFY)/venv
	$(VERIFY)/venv/bin/pip install -q protobuf annotated_types
	$(VERIFY)/venv/bin/python $(VERIFY)/assert.py

# --- test -------------------------------------------------------------------

## test: run Go unit tests, including the hermetic golden tests
test:
	go test ./...

## testdata: rebuild the descriptor set the golden tests run against
testdata:
	buf build --as-file-descriptor-set -o internal/testdata/descriptors.binpb

## testdata-update: rebuild the descriptor set and rewrite the golden files
testdata-update: testdata
	go test ./internal/generator -update

## check: format, lint, tests, and a real end-to-end run — before a PR
check: fmt-check lint test generate

# --- format & lint ----------------------------------------------------------

## fmt: rewrite Go sources with gofmt
fmt:
	gofmt -w -s $(GO_DIRS)

## fmt-check: fail if any Go source is unformatted
fmt-check:
	@unformatted=$$(gofmt -l -s $(GO_DIRS)); \
	if [ -n "$$unformatted" ]; then \
		echo "gofmt needed:"; echo "$$unformatted"; exit 1; \
	fi

## lint: go vet, staticcheck, and buf lint
lint:
	go vet ./...
	@command -v staticcheck >/dev/null || { echo "staticcheck not found — run 'make tools'"; exit 1; }
	staticcheck ./...
	buf lint

# --- dependencies -----------------------------------------------------------

## tidy: sync go.mod/go.sum with the imports actually used
tidy:
	go mod tidy

## update: bump Go modules and buf deps to their latest versions
update:
	go get -u ./...
	go mod tidy
	buf dep update

## tools: install the dev tools the lint targets expect
tools:
	go install honnef.co/go/tools/cmd/staticcheck@latest

# --- housekeeping -----------------------------------------------------------

## clean: remove build and generation artifacts
clean:
	rm -rf bin gen dist $(VERIFY)/gen $(VERIFY)/venv $(VERIFY)/node_modules
