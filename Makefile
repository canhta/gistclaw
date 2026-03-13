.PHONY: tidy test build run vet lint deploy logs

BINARY_DIR := ./bin
GISTCLAW   := $(BINARY_DIR)/gistclaw
HOOK       := $(BINARY_DIR)/gistclaw-hook

tidy:
	go mod tidy

test:
	go test ./... -timeout 120s

build: tidy
	mkdir -p $(BINARY_DIR)
	go build -o $(GISTCLAW) ./cmd/gistclaw
	go build -o $(HOOK) ./cmd/gistclaw-hook

run: build
	$(GISTCLAW)

vet:
	go vet ./...

lint: vet
	golangci-lint run ./...

# Deploy: rsync binaries to VPS and restart the service
# Usage: make deploy VPS=user@your-vps-ip
deploy: build
	rsync -avz $(GISTCLAW) $(HOOK) $(VPS):/usr/local/bin/
	ssh $(VPS) "systemctl restart gistclaw"

# Tail logs on VPS
# Usage: make logs VPS=user@your-vps-ip
logs:
	ssh $(VPS) "journalctl -u gistclaw -f"
