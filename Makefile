.PHONY: help deps fmt vet test build run e2e check clean

POWERSHELL := powershell.exe -NoLogo -NoProfile -ExecutionPolicy Bypass
GOCACHE := $(CURDIR)/.cache/go-build
export GOCACHE

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

run:
	go run ./cmd

e2e:
	$(POWERSHELL) -File scripts/e2e.ps1

check: fmt vet test build

clean:
	$(POWERSHELL) -Command "Remove-Item -Recurse -Force -ErrorAction SilentlyContinue bin, .run"
