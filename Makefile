BINARY_NAME=hrcx
VERSION?=0.1.0
LDFLAGS=-ldflags "-s -w -X main.version=$(VERSION)"

.PHONY: build test lint clean cross-compile fmt fmt-check setup-hooks

build:
	go build $(LDFLAGS) -o $(BINARY_NAME) .

test:
	go test ./...

test-unit:
	go test ./internal/...

test-e2e:
	go test ./tests/ -run TestE2E -timeout 10m

test-stress:
	go test ./tests/ -run TestStress -timeout 30m

bench:
	go test ./tests/ -bench . -benchmem -benchtime 3x -timeout 30m

lint:
	@if command -v golangci-lint &>/dev/null; then \
		golangci-lint run ./...; \
	else \
		echo "golangci-lint not found, falling back to go vet"; \
		go vet ./...; \
	fi

fmt:
	gofmt -w .

fmt-check:
	@unformatted=$$(gofmt -l .); \
	if [ -n "$$unformatted" ]; then \
		echo "Unformatted files:"; \
		echo "$$unformatted"; \
		exit 1; \
	fi

setup-hooks:
	git config core.hooksPath .githooks
	@echo "Git hooks path set to .githooks"

clean:
	rm -f $(BINARY_NAME)
	rm -rf dist/

cross-compile: clean
	GOOS=darwin GOARCH=arm64 go build $(LDFLAGS) -o dist/$(BINARY_NAME)-darwin-arm64 .
	GOOS=darwin GOARCH=amd64 go build $(LDFLAGS) -o dist/$(BINARY_NAME)-darwin-amd64 .
	GOOS=linux GOARCH=amd64 go build $(LDFLAGS) -o dist/$(BINARY_NAME)-linux-amd64 .
	GOOS=windows GOARCH=amd64 go build $(LDFLAGS) -o dist/$(BINARY_NAME)-windows-amd64.exe .
