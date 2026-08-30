VERSION ?= $(shell git describe --tags --dirty 2>/dev/null || echo v0.0.0)
GIT_COMMIT ?= $(shell git rev-parse --short HEAD 2>/dev/null || echo none)
BUILD_TIME ?= $(shell date '+%Y-%m-%d %H:%M:%S')
GO_VERSION ?= $(shell go env GOVERSION 2>/dev/null || echo unknown)
SERVER_LDFLAGS := -s -w -X 'main.Version=$(VERSION)' -X 'main.GitCommit=$(GIT_COMMIT)' -X 'main.BuildTime=$(BUILD_TIME)' -X 'main.GoVersion=$(GO_VERSION)'

.PHONY: test vet frontend-build build migrate migrate-verify admin-password compose-up compose-down

test:
	GOCACHE=/tmp/file-share-manager-go-cache go -C server test ./...

vet:
	GOCACHE=/tmp/file-share-manager-go-cache go -C server vet ./...

frontend-build:
	npm --prefix frontend run build

build: frontend-build
	CGO_ENABLED=0 go -C server build -trimpath -ldflags="$(SERVER_LDFLAGS)" ./cmd/server

migrate:
	go -C server run ./cmd/migrate --config configs/config-prod.toml

migrate-verify:
	go -C server run ./cmd/migrate --config configs/config-prod.toml --verify

admin-password:
	go -C server run ./cmd/admin-password --config configs/config-dev.toml

compose-up:
	docker compose up --build -d

compose-down:
	docker compose down
