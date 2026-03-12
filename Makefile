.PHONY: tidy vet test lint

tidy:
	go mod tidy

vet:
	go vet ./...

test: vet
	go test ./...

lint:
	golangci-lint run ./...
