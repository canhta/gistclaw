.PHONY: tidy test

tidy:
	go mod tidy

test:
	go test ./...
