SHELL = /bin/sh

LOCALBIN ?= .bin
GOIMPORTS = $(LOCALBIN)/goimports
GOLANGCI_LINT = $(LOCALBIN)/golangci-lint
LEFTHOOK = $(LOCALBIN)/lefthook

GOIMPORTS_VERSION ?= v0.43.0
GOLANGCI_LINT_VERSION ?= v2.11.4
LEFTHOOK_VERSION ?= v2.0.4
COVERAGE_MIN ?= 70

.PHONY: dev fmt lint test run hooks-install precommit prepush
.PHONY: coverage
.PHONY: ui-install ui-build ui-check ui-test ui-lint ui-format

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

ui-install:
	cd frontend && bun install

ui-build:
	cd frontend && bun run build

ui-check:
	cd frontend && bun run check

ui-test:
	cd frontend && bun run test:unit -- --run

ui-lint:
	cd frontend && bun run lint

ui-format:
	cd frontend && bun run format

hooks-install: $(LEFTHOOK)
	rm -f .githooks/*.old .githooks/prepare-commit-msg
	chmod +x .githooks/run .githooks/pre-commit .githooks/pre-push
	git config core.hooksPath "$$(git rev-parse --show-toplevel)/.githooks"

precommit: $(GOIMPORTS) $(GOLANGCI_LINT)
	FILES="$(FILES)"; \
	GO_FILES="$$(printf '%s\n' $$FILES | tr ' ' '\n' | grep '\.go$$' || true)"; \
	MODULE_FILES="$$(printf '%s\n' $$FILES | tr ' ' '\n' | grep -E '^(go\.mod|go\.sum)$$' || true)"; \
	FRONTEND_FILES="$$(printf '%s\n' $$FILES | tr ' ' '\n' | grep '^frontend/' || true)"; \
	if [ -z "$$GO_FILES$$MODULE_FILES$$FRONTEND_FILES" ]; then \
		echo "No relevant Go or frontend files staged."; \
		exit 0; \
	fi; \
	if [ -n "$$GO_FILES" ]; then \
		$(GOIMPORTS) -w $$GO_FILES; \
		git add $$GO_FILES; \
		$(GOLANGCI_LINT) run --fast-only $$GO_FILES; \
	fi; \
	if [ -n "$$MODULE_FILES" ]; then \
		$(GOLANGCI_LINT) run --fast-only; \
	fi; \
	if [ -n "$$FRONTEND_FILES" ]; then \
		cd frontend && bun run lint; \
		cd frontend && bun run check; \
	fi

prepush: lint coverage ui-check ui-lint ui-test ui-build

$(LOCALBIN):
	mkdir -p $(LOCALBIN)

$(GOIMPORTS): | $(LOCALBIN)
	GOBIN="$$(pwd)/$(LOCALBIN)" go install golang.org/x/tools/cmd/goimports@$(GOIMPORTS_VERSION)

$(LEFTHOOK): | $(LOCALBIN)
	GOBIN="$$(pwd)/$(LOCALBIN)" go install github.com/evilmartians/lefthook/v2@$(LEFTHOOK_VERSION)

$(GOLANGCI_LINT): | $(LOCALBIN)
	curl -sSfL https://golangci-lint.run/install.sh | sh -s -- -b "$$(pwd)/$(LOCALBIN)" $(GOLANGCI_LINT_VERSION)
