.PHONY: all test vet vuln wasm serve clean

GO      ?= $(shell which go || echo /opt/homebrew/bin/go)
GOFLAGS ?=

## Run all checks (same as pre-push)
all: vet test vuln

## Run unit tests with race detector
test:
	$(GO) test ./... -race -count=1

## Run go vet
vet:
	$(GO) vet ./...

## Run govulncheck (must be installed: go install golang.org/x/vuln/cmd/govulncheck@v1.6.0)
vuln:
	govulncheck ./...

## Build WASM demo binary
wasm:
	GOOS=js GOARCH=wasm $(GO) build -o demo/query.wasm ./cmd/demo-wasm/
	cp "$$($(GO) env GOROOT)/lib/wasm/wasm_exec.js" demo/
	@echo "WASM built: demo/query.wasm"

## Serve the demo locally on port 8080
serve: wasm
	@echo "Serving demo at http://localhost:8080"
	cd demo && python3 -m http.server 8080

## Remove generated WASM files
clean:
	rm -f demo/query.wasm demo/wasm_exec.js
