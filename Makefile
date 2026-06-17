# rtbeat — local verification matching CI.
#
# Run `make verify` before pushing. It runs the same checks the CI
# pipeline does (lint, test, build, tidy, action-pin validation) so you
# don't find out from a red PR.

# Versions — keep in sync with .github/workflows/ci.yml.
GOLANGCI_LINT_VERSION ?= v2.12.1
GO_VERSION_REQUIRED   ?= 1.26

# Local cache for tooling we install on demand.
TOOLS_BIN := $(CURDIR)/.tools/bin
GOLANGCI_LINT := $(TOOLS_BIN)/golangci-lint

GO ?= go

.DEFAULT_GOAL := verify

.PHONY: verify
verify: check-go-version tidy-check lint test build validate-actions
	@echo ""
	@echo "==> verify: all checks passed"

.PHONY: check-go-version
check-go-version:
	@have=$$($(GO) env GOVERSION | sed 's/^go//'); \
	want=$(GO_VERSION_REQUIRED); \
	case "$$have" in \
	  $$want|$$want.*) echo "==> go $$have (matches $$want.x)";; \
	  *) echo "ERROR: local go is $$have, CI uses $$want.x. Install matching toolchain." >&2; exit 1;; \
	esac

# Verify go.mod / go.sum are tidy without rewriting in place.
.PHONY: tidy-check
tidy-check:
	@echo "==> go mod tidy (check)"
	@tmp=$$(mktemp -d); cp go.mod go.sum "$$tmp/"; \
	$(GO) mod tidy; \
	if ! diff -q "$$tmp/go.mod" go.mod >/dev/null || ! diff -q "$$tmp/go.sum" go.sum >/dev/null; then \
	  echo "ERROR: go.mod/go.sum are not tidy. Run 'go mod tidy' and commit." >&2; \
	  diff -u "$$tmp/go.mod" go.mod || true; \
	  diff -u "$$tmp/go.sum" go.sum || true; \
	  cp "$$tmp/go.mod" "$$tmp/go.sum" .; \
	  rm -rf "$$tmp"; \
	  exit 1; \
	fi; \
	rm -rf "$$tmp"

.PHONY: tidy
tidy:
	$(GO) mod tidy

.PHONY: build
build:
	@echo "==> go build"
	CGO_ENABLED=0 $(GO) build -o rtbeat .

.PHONY: test
test:
	@echo "==> go test (race + coverage, matches CI)"
	$(GO) test -race -coverprofile=coverage.txt -covermode=atomic ./...

# End-to-end durability tests: build the real binary and drive it against a
# controllable lumberjack (logstash) server. Tag-gated so the default test run
# stays fast and offline.
.PHONY: e2e
e2e:
	@echo "==> go test -tags e2e (end-to-end durability)"
	$(GO) test -tags e2e -count=1 -timeout 300s ./test/e2e/...

# Install the exact golangci-lint version CI uses, into a local cache.
$(GOLANGCI_LINT):
	@echo "==> installing golangci-lint $(GOLANGCI_LINT_VERSION)"
	@mkdir -p $(TOOLS_BIN)
	@curl -sSfL https://raw.githubusercontent.com/golangci/golangci-lint/main/install.sh \
		| sh -s -- -b $(TOOLS_BIN) $(GOLANGCI_LINT_VERSION)

.PHONY: lint
lint: $(GOLANGCI_LINT)
	@echo "==> golangci-lint $(GOLANGCI_LINT_VERSION)"
	$(GOLANGCI_LINT) run --timeout=5m

.PHONY: validate-actions
validate-actions:
	@echo "==> validating GitHub Action SHA pins"
	./scripts/validate-action-shas.sh

.PHONY: clean
clean:
	rm -rf coverage.txt rtbeat dist $(TOOLS_BIN) .tools

.PHONY: help
help:
	@echo "Targets:"
	@echo "  verify           lint + test + build + tidy-check + action pins"
	@echo "  lint             golangci-lint (auto-installs $(GOLANGCI_LINT_VERSION) to .tools/)"
	@echo "  test             go test -race with coverage profile"
	@echo "  e2e              end-to-end durability tests (go test -tags e2e)"
	@echo "  build            CGO_ENABLED=0 go build -o rtbeat ."
	@echo "  tidy             go mod tidy"
	@echo "  tidy-check       fail if go.mod/go.sum are not tidy"
	@echo "  validate-actions check GitHub Action SHA pins"
	@echo "  clean            remove build output and tool cache"
