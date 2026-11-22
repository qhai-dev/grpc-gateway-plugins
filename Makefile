# Integration checks that span the three plugins.
#
# Each plugin has its own module and its own unit tests. What cannot be tested
# inside a single project is the thing most likely to break: whether the three
# generated files compile together. This file regenerates all three from one
# descriptor set, compiles the result under strict TypeScript, and runs the
# gateway conformance check.

# ---------------------------------------------------------------------------
# Configuration
# ---------------------------------------------------------------------------

GO ?= go
PROTOC ?= protoc
NODE ?= node
NPM ?= npm

# The fixture protos and their google/api dependency.
PROTO_DIR := protoc-gen-grpc-gateway-react-query/testdata/proto
PROTODEPS := _protodeps
PROTOC_INCLUDE ?= /usr/include

# Generated output and its scratch node_modules live here, outside every project.
WORK := .verify
GEN := $(WORK)/generated
DEPS := $(WORK)/deps

PLUGINS := protoc-gen-grpc-gateway-typescript \
           protoc-gen-grpc-gateway-axios \
           protoc-gen-grpc-gateway-react-query

FIXTURES := user/v1/user.proto common/v1/address.proto shipping/v1/shipping.proto

# Contract options shared by all three plugins. Changing one here changes the
# imports the other two emit, which is exactly why they are defined once.
CONTRACT_OPTS := import_extension=.js

.PHONY: all test build generate typecheck runtime conformance example-check clean

all: test typecheck runtime conformance

# ---------------------------------------------------------------------------
# Per-plugin unit tests
# ---------------------------------------------------------------------------

test:
	@for p in $(PLUGINS); do \
		echo "=== $$p"; \
		(cd $$p && $(GO) test ./...) || exit 1; \
	done

# ---------------------------------------------------------------------------
# Generation
# ---------------------------------------------------------------------------

build:
	@mkdir -p $(WORK)/bin
	@for p in $(PLUGINS); do \
		echo "=== building $$p"; \
		(cd $$p && $(GO) build -o $(CURDIR)/$(WORK)/bin/$$p ./cmd/$$p) || exit 1; \
	done

generate: build
	@rm -rf $(GEN) && mkdir -p $(GEN)
	$(PROTOC) \
		-I $(PROTO_DIR) -I $(PROTODEPS) -I $(PROTOC_INCLUDE) \
		--plugin=protoc-gen-ts=$(CURDIR)/$(WORK)/bin/protoc-gen-grpc-gateway-typescript \
		--plugin=protoc-gen-api=$(CURDIR)/$(WORK)/bin/protoc-gen-grpc-gateway-axios \
		--plugin=protoc-gen-rq=$(CURDIR)/$(WORK)/bin/protoc-gen-grpc-gateway-react-query \
		--ts_out=$(CONTRACT_OPTS):$(GEN) \
		--api_out=$(CONTRACT_OPTS):$(GEN) \
		--rq_out=$(CONTRACT_OPTS):$(GEN) \
		$(FIXTURES)

# ---------------------------------------------------------------------------
# Strict compilation of the three artifacts together
# ---------------------------------------------------------------------------

typecheck: generate
	@mkdir -p $(DEPS)
	@cd $(DEPS) && [ -d node_modules/axios ] || $(NPM) install --silent --no-save \
		axios@1.7.9 @tanstack/react-query@5.62.7 react@18.3.1 @types/react@18.3.12 \
		typescript@5.7.3
	@cp -r $(DEPS)/node_modules $(GEN)/node_modules
	@printf '%s\n' '{ "type": "module", "private": true }' > $(GEN)/package.json
	@printf '%s\n' \
		'{' \
		'  "compilerOptions": {' \
		'    "strict": true,' \
		'    "exactOptionalPropertyTypes": true,' \
		'    "noUncheckedIndexedAccess": true,' \
		'    "verbatimModuleSyntax": true,' \
		'    "module": "NodeNext",' \
		'    "moduleResolution": "NodeNext",' \
		'    "target": "ES2022",' \
		'    "jsx": "react-jsx",' \
		'    "noEmit": true,' \
		'    "skipLibCheck": true' \
		'  },' \
		'  "include": ["**/*.ts"]' \
		'}' > $(GEN)/tsconfig.json
	@cd $(GEN) && $(DEPS)/node_modules/.bin/tsc -p tsconfig.json
	@echo "tsc: OK"

# ---------------------------------------------------------------------------
# Runtime behaviour of the generated client
# ---------------------------------------------------------------------------
#
# The acceptance harness imports the SDK as ./generated/..., so it is run from a
# directory that holds the generated tree under that name.

runtime: generate
	@rm -rf $(WORK)/harness && mkdir -p $(WORK)/harness
	@cp -r $(GEN) $(WORK)/harness/generated
	@cd $(DEPS) && $(NPM) install --silent --no-save axios@1.7.9 tsx@4.19.2
	@cp -r $(DEPS)/node_modules $(WORK)/harness/node_modules
	@printf '%s\n' '{ "type": "module", "private": true }' > $(WORK)/harness/package.json
	@cp protoc-gen-grpc-gateway-react-query/acceptance/runtime.ts $(WORK)/harness/
	@cd $(WORK)/harness && ./node_modules/.bin/tsx runtime.ts

# ---------------------------------------------------------------------------
# grpc-gateway query conformance
# ---------------------------------------------------------------------------

conformance:
	@cd protoc-gen-grpc-gateway-axios/conformance && $(GO) run .

# ---------------------------------------------------------------------------
# The example project
# ---------------------------------------------------------------------------
# The example project
# ---------------------------------------------------------------------------
#
# It has its own Node toolchain, its own lockfile and its own Buf module, so it is
# driven through npm rather than Make. `npm run generate` regenerates the SDK with Buf;
# `example-check` is the CI-shaped subset, and fails if the committed generated code has
# drifted from what the current plugins produce.

example-check:
	@cd example && npm install --silent
	@cd example && npm run generate
	@cd example && npm run typecheck
	@cd example && npm run test

# ---------------------------------------------------------------------------
# Buf
# ---------------------------------------------------------------------------
#
# The example drives Buf itself (example/buf.gen.yaml runs the plugins with `go run`),
# so the canonical command is `npm run generate` in example/. These targets cover the
# repository-root perspective: exercising the plugins with Buf without the example's
# Node toolchain, and asserting that the two entry points agree.

BUF ?= buf
BUF_DIR := .buf

.PHONY: buf-build buf-generate buf-lint buf-check

buf-build:
	@mkdir -p $(BUF_DIR)/bin
	@for p in $(PLUGINS); do \
		echo "=== building $$p"; \
		(cd $$p && $(GO) build -o $(CURDIR)/$(BUF_DIR)/bin/$$p ./cmd/$$p) || exit 1; \
	done

buf-lint:
	$(BUF) lint
	@cd example && $(BUF) lint

buf-generate: buf-build
	@rm -rf $(BUF_DIR)/gen
	$(BUF) generate

# buf-check fails when the two Buf entry points disagree, which would mean the root
# module and example/buf.yaml have drifted apart. It also catches a stale src/gen, since
# the comparison is against the committed example SDK.
buf-check: buf-generate
	@diff -r $(BUF_DIR)/gen example/src/gen > /dev/null \
		&& echo "root Buf module and example agree" \
		|| { echo "root Buf output and example/src/gen disagree:"; \
		     diff -r $(BUF_DIR)/gen example/src/gen | head -30; exit 1; }

clean:
	@rm -rf $(WORK) $(BUF_DIR)
