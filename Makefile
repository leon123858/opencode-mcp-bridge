.PHONY: help deps fmt vet test build build-windows build-linux run e2e check clean

POWERSHELL := powershell.exe -NoLogo -NoProfile -ExecutionPolicy Bypass

help:
	@$(POWERSHELL) -Command "Write-Host 'Targets: deps fmt vet test build run e2e check clean'"

deps:
	go mod download
	go mod tidy

fmt:
	gofmt -w cmd client config handlers mcpbridge server types

vet:
	go vet ./...

test:
	$(POWERSHELL) -File scripts/test.ps1

build:
	go build -o bin/opencode-mcp-bridge.exe ./cmd

build-windows:
	$(POWERSHELL) -Command "$$env:GOOS='windows'; $$env:GOARCH='amd64'; go build -o bin/opencode-mcp-bridge-windows-amd64.exe ./cmd"

build-linux:
	$(POWERSHELL) -Command "$$env:GOOS='linux'; $$env:GOARCH='amd64'; go build -o bin/opencode-mcp-bridge-linux-amd64 ./cmd"

run:
	go run ./cmd

e2e:
	$(POWERSHELL) -File scripts/e2e.ps1

check: fmt vet test build

clean:
	$(POWERSHELL) -Command "Remove-Item -Recurse -Force -ErrorAction SilentlyContinue bin, .run"
