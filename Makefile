SHELL = /bin/sh

LOCALBIN ?= .bin
GOIMPORTS = $(LOCALBIN)/goimports
GOLANGCI_LINT = $(LOCALBIN)/golangci-lint
LEFTHOOK = $(LOCALBIN)/lefthook
AIR = $(LOCALBIN)/air

GOIMPORTS_VERSION ?= v0.43.0
GOLANGCI_LINT_VERSION ?= v2.11.4
LEFTHOOK_VERSION ?= v2.0.4
AIR_VERSION ?= v1.64.5
COVERAGE_MIN ?= 70
DEV_API_ORIGIN ?= http://127.0.0.1:8080

.PHONY: dev dev-tools fmt lint test run hooks-install precommit prepush
.PHONY: coverage
.PHONY: ui-install ui-build ui-check ui-test ui-lint ui-format

dev-tools: $(GOIMPORTS) $(GOLANGCI_LINT) $(LEFTHOOK) $(AIR) ui-install

dev: dev-tools
	DEV_API_ORIGIN="$(DEV_API_ORIGIN)"; \
	echo "Starting Go on 127.0.0.1:8080 and Vite on 127.0.0.1:5173"; \
	echo "Open http://127.0.0.1:5173"; \
	GO_PID=""; \
	UI_PID=""; \
	STATUS=0; \
	record_status() { if [ "$$1" -ne 0 ] && [ "$$STATUS" -eq 0 ]; then STATUS=$$1; fi; }; \
	trap 'kill $$GO_PID $$UI_PID 2>/dev/null || true' INT TERM EXIT; \
	$(AIR) -c .air.toml & \
	GO_PID=$$!; \
	cd frontend && VITE_GISTCLAW_API_ORIGIN="$$DEV_API_ORIGIN" bun run dev & \
	UI_PID=$$!; \
	while kill -0 $$GO_PID 2>/dev/null && kill -0 $$UI_PID 2>/dev/null; do \
		sleep 1; \
	done; \
	if ! kill -0 $$GO_PID 2>/dev/null; then \
		wait $$GO_PID; \
		record_status $$?; \
	fi; \
	if ! kill -0 $$UI_PID 2>/dev/null; then \
		wait $$UI_PID; \
		record_status $$?; \
	fi; \
	kill $$GO_PID $$UI_PID 2>/dev/null || true; \
	wait $$GO_PID 2>/dev/null; \
	record_status $$?; \
	wait $$UI_PID 2>/dev/null; \
	record_status $$?; \
	exit $$STATUS

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

$(AIR): | $(LOCALBIN)
	GOBIN="$$(pwd)/$(LOCALBIN)" go install github.com/air-verse/air@$(AIR_VERSION)
