ARG GO_VERSION=1.26.5

FROM golang:${GO_VERSION}-alpine AS builder

ARG GO_VERSION
ARG VERSION=v0.0.0
ARG GIT_COMMIT=none
ARG BUILD_TIME=unknown

WORKDIR /src
COPY go.mod go.sum ./
RUN go mod download
COPY . .
RUN CGO_ENABLED=0 GOOS=linux go build -trimpath -ldflags="-s -w -X 'main.Version=${VERSION}' -X 'main.GitCommit=${GIT_COMMIT}' -X 'main.BuildTime=${BUILD_TIME}' -X 'main.GoVersion=go${GO_VERSION}'" -o /out/fs-server ./cmd/server
RUN CGO_ENABLED=0 GOOS=linux go build -trimpath -ldflags='-s -w' -o /out/fs-migrate ./cmd/migrate

FROM alpine:3.22
RUN addgroup -S fileshare && adduser -S -G fileshare fileshare
WORKDIR /app
COPY --from=builder /out/fs-server /app/fs-server
COPY --from=builder /out/fs-migrate /app/fs-migrate
COPY configs /app/configs
RUN mkdir -p /data/fileshare /data/fileshare/staging /app/logs && chown -R fileshare:fileshare /data /app
USER fileshare
EXPOSE 29000
ENTRYPOINT ["/app/fs-server"]
