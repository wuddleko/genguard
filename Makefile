.PHONY: build test lint install validate-examples

build:
	go build -o bin/regen ./cmd/regen

install:
	go install ./cmd/regen

test:
	go test ./...

lint:
	go vet ./...

validate-examples:
	go test ./internal/config/ -run TestExampleYAMLTemplatesLoad -count=1
