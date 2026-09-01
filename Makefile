VERSION ?= $(shell git describe --tags --dirty 2>/dev/null || echo v0.0.0)
GIT_COMMIT ?= $(shell git rev-parse --short HEAD 2>/dev/null || echo none)
BUILD_TIME ?= $(shell date '+%Y-%m-%d %H:%M:%S')
GO_VERSION ?= $(shell go env GOVERSION 2>/dev/null || echo unknown)
SWAG_VERSION ?= v1.16.6
SWAG_DIRS ?= cmd/server,internal/config,internal/dao,internal/handler,internal/middleware,internal/migration,internal/model,internal/pkg/auditcontext,internal/pkg/database,internal/pkg/jwt,internal/pkg/ldapdn,internal/pkg/logger,internal/pkg/pagination,internal/pkg/request,internal/pkg/response,internal/pkg/security,internal/router,internal/service/archive,internal/service/auditarchive,internal/service/auditexport,internal/service/authorization,internal/service/backup,internal/service/batchdownload,internal/service/clamav,internal/service/ldap,internal/service/ldapsync,internal/service/lifecycle,internal/service/notification,internal/service/reconcile,internal/service/storagehealth,internal/storage
SERVER_LDFLAGS := -s -w -X 'main.Version=$(VERSION)' -X 'main.GitCommit=$(GIT_COMMIT)' -X 'main.BuildTime=$(BUILD_TIME)' -X 'main.GoVersion=$(GO_VERSION)'

.PHONY: test vet swag frontend-build build migrate migrate-verify admin-password compose-up compose-down

test:
	GOCACHE=/tmp/file-share-manager-go-cache go -C server test ./...

vet:
	GOCACHE=/tmp/file-share-manager-go-cache go -C server vet ./...

swag:
	cd server && go run github.com/swaggo/swag/cmd/swag@$(SWAG_VERSION) init -g main.go -d $(SWAG_DIRS) -o docs --parseInternal

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
