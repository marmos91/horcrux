BINARY_NAME=hrcx
VERSION?=0.1.0
LDFLAGS=-ldflags "-s -w -X main.version=$(VERSION)"

.PHONY: build test lint clean cross-compile

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
	go vet ./...

clean:
	rm -f $(BINARY_NAME)
	rm -rf dist/

cross-compile: clean
	GOOS=darwin GOARCH=arm64 go build $(LDFLAGS) -o dist/$(BINARY_NAME)-darwin-arm64 .
	GOOS=darwin GOARCH=amd64 go build $(LDFLAGS) -o dist/$(BINARY_NAME)-darwin-amd64 .
	GOOS=linux GOARCH=amd64 go build $(LDFLAGS) -o dist/$(BINARY_NAME)-linux-amd64 .
	GOOS=windows GOARCH=amd64 go build $(LDFLAGS) -o dist/$(BINARY_NAME)-windows-amd64.exe .
