.PHONY: all build fmt vet test coverage tidy clean help

all: help

## build: Verify all packages compile (library-only; produces no binaries)
build:
	go build ./...

## fmt: Format all Go source files
fmt:
	go fmt ./...

## vet: Run go vet static analysis
vet:
	go vet ./...

## test: Run all unit tests
test:
	go test -v ./...

## coverage: Run tests and print a coverage summary
coverage:
	@go test -coverprofile=coverage.out ./cli
	@go tool cover -func=coverage.out
	@rm -f coverage.out

## tidy: Tidy up go.mod and go.sum dependencies
tidy:
	go mod tidy

## clean: Remove test and coverage artifacts
clean:
	@echo "Cleaning artifacts..."
	@rm -f coverage.out

## help: Display available commands
help:
	@echo "Available commands:"
	@sed -n 's/^##//p' $(MAKEFILE_LIST) | column -t -s ':'
