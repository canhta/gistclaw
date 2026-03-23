SHELL = /bin/sh

LOCALBIN ?= .bin
GOIMPORTS = $(LOCALBIN)/goimports
GOLANGCI_LINT = $(LOCALBIN)/golangci-lint
LEFTHOOK = $(LOCALBIN)/lefthook

GOIMPORTS_VERSION ?= v0.43.0
GOLANGCI_LINT_VERSION ?= v2.11.4
LEFTHOOK_VERSION ?= v2.0.4
COVERAGE_MIN ?= 70

.PHONY: dev fmt lint test run hooks-install precommit
.PHONY: coverage

dev: $(GOIMPORTS) $(GOLANGCI_LINT) $(LEFTHOOK)

fmt: $(GOIMPORTS)
	FILES="$$(git ls-files '*.go')"; \
	if [ -n "$$FILES" ]; then \
		$(GOIMPORTS) -w $$FILES; \
	fi

lint: $(GOLANGCI_LINT)
	$(GOLANGCI_LINT) run

test:
	go test ./...

coverage:
	go test ./... -coverprofile=coverage.out
	@COVERAGE_MIN="$(COVERAGE_MIN)"; \
	go tool cover -func=coverage.out | \
	awk -v min="$$COVERAGE_MIN" '/^total:/ { \
		gsub(/%/, "", $$3); \
		printf "total coverage: %s%% (minimum %s%%)\n", $$3, min; \
		if (($$3 + 0) < (min + 0)) exit 1; \
	}'

run:
	go run ./cmd/gistclaw $(ARGS)

hooks-install: $(LEFTHOOK)
	$(LEFTHOOK) install

precommit: $(GOIMPORTS) $(GOLANGCI_LINT)
	FILES="$(FILES)"; \
	GO_FILES="$$(printf '%s\n' $$FILES | tr ' ' '\n' | grep '\.go$$' || true)"; \
	MODULE_FILES="$$(printf '%s\n' $$FILES | tr ' ' '\n' | grep -E '^(go\.mod|go\.sum)$$' || true)"; \
	if [ -z "$$GO_FILES$$MODULE_FILES" ]; then \
		echo "No relevant Go files staged."; \
		exit 0; \
	fi; \
	if [ -n "$$GO_FILES" ]; then \
		$(GOIMPORTS) -w $$GO_FILES; \
		git add $$GO_FILES; \
		$(GOLANGCI_LINT) run --fast-only $$GO_FILES; \
	fi; \
	if [ -n "$$MODULE_FILES" ]; then \
		$(GOLANGCI_LINT) run --fast-only; \
	fi

$(LOCALBIN):
	mkdir -p $(LOCALBIN)

$(GOIMPORTS): | $(LOCALBIN)
	GOBIN="$$(pwd)/$(LOCALBIN)" go install golang.org/x/tools/cmd/goimports@$(GOIMPORTS_VERSION)

$(LEFTHOOK): | $(LOCALBIN)
	GOBIN="$$(pwd)/$(LOCALBIN)" go install github.com/evilmartians/lefthook/v2@$(LEFTHOOK_VERSION)

$(GOLANGCI_LINT): | $(LOCALBIN)
	curl -sSfL https://golangci-lint.run/install.sh | sh -s -- -b "$$(pwd)/$(LOCALBIN)" $(GOLANGCI_LINT_VERSION)
