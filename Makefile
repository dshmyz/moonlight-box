# Makefile
.PHONY: build run clean test lint

APP_NAME=moonlight-registry
VERSION=$(shell git describe --tags --always --dirty 2>/dev/null || echo "dev")
BUILD_TIME=$(shell date -u +"%Y-%m-%dT%H:%M:%SZ")
GIT_COMMIT=$(shell git rev-parse --short HEAD 2>/dev/null || echo "unknown")
GO_VERSION=$(shell go version | awk '{print $$3}')

LDFLAGS=-ldflags "-s -w -X main.version=${VERSION} -X main.buildTime=${BUILD_TIME} -X main.gitCommit=${GIT_COMMIT}"

build:
	go build ${LDFLAGS} -o bin/${APP_NAME} ./cmd/registry

run:
	go run ./cmd/registry serve

test:
	go test -v ./...

test-coverage:
	go test -v -coverprofile=coverage.out ./...
	go tool cover -html=coverage.out -o coverage.html

lint:
	golangci-lint run ./...

clean:
	rm -rf bin/ coverage.out coverage.html

dev:
	AIR_CONFIG=.air.toml air

# 嵌入前端到二进制
embed-web:
	cd web && npm run build
	rm -rf cmd/registry/dist
	cp -r web/dist cmd/registry/dist
