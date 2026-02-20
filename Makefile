.PHONY: build test test-race test-cover test-coverprofile vet validate-flows demo

build:
	CGO_ENABLED=0 go build -o ./bin/flowk ./cmd/flowk/main.go

test:
	go test ./...

test-race:
	go test -race ./...

test-cover:
	go test -cover ./...

test-coverprofile:
	go test -coverprofile=coverage.out ./...
	go tool cover -html=coverage.out -o coverage.html

vet:
	go vet ./...

validate-flows: build
	./scripts/validate_flows.sh ./flows

demo: build
	./bin/flowk run -flow ./flows/http_test.json -validate-only
	./bin/flowk run -flow ./flows/http_test.json
