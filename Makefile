.PHONY: build test lint install validate-examples

build:
	go build -o bin/genguard ./cmd/genguard

install:
	go install ./cmd/genguard

test:
	go test ./...

lint:
	go vet ./...

validate-examples:
	go test ./internal/config/ -run TestExampleYAMLTemplatesLoad -count=1
