[简体中文](README.md) | English

<div align="center"><img src="frontend/public/logo.png" alt="File Share Manager" width="180" /></div>

<p align="center">
  <b>File Share Manager - An open-source file sharing and governance platform for team workspaces</b><br/>
  Built with Go/Gin/GORM, Vue 3, Element Plus, and MySQL 8, it provides workspace file management, chunked uploads, version management, external-link sharing, permission auditing, backup and recovery, and storage health governance.
</p>

<p align="center">
  <a target="_blank" href="https://github.com/ghl1024/file-share-manager">
    <img src="https://img.shields.io/github/stars/ghl1024/file-share-manager" alt="GitHub stars" />
  </a>
  <a target="_blank" href="https://github.com/ghl1024/file-share-manager">
    <img src="https://img.shields.io/github/forks/ghl1024/file-share-manager" alt="GitHub forks" />
  </a>
  <a target="_blank" href="https://www.apache.org/licenses/LICENSE-2.0">
    <img src="https://img.shields.io/badge/License-Apache_2.0-green.svg" alt="Apache 2.0" />
  </a>
  <a target="_blank" href="https://gitee.com/ghl1024/file-share-manager">
    <img src="https://gitee.com/ghl1024/file-share-manager/badge/star.svg?theme=dark" alt="Gitee star" />
  </a>
</p>

<div align="center">
  <img src="https://img.shields.io/badge/Vue-3.5-brightgreen.svg" alt="Vue">
  <img src="https://img.shields.io/badge/Go-1.26.5-blue.svg" alt="Go">
  <img src="https://img.shields.io/badge/Gin-1.12-blue.svg" alt="Gin">
  <img src="https://img.shields.io/badge/Vite-6.4-purple.svg" alt="Vite">
  <img src="https://img.shields.io/badge/Element_Plus-2.14-blue.svg" alt="Element Plus">
  <img src="https://img.shields.io/badge/MySQL-8.x-orange.svg" alt="MySQL">
</div>

---

## Table of Contents

- [1. Project Overview](#section-1)
- [2. System Architecture](#section-2)
- [3. Feature Modules](#section-3)
- [4. System Preview](#section-4)
- [5. Project Documentation](#section-5)
- [6. Deployment Options](#section-6)
- [7. Login and Verification](#section-7)
- [8. Runtime Configuration](#section-8)
- [9. Contributing](#section-9)
- [10. License](#section-10)

---

<a id="section-1"></a>

## 1. Project Overview

### 1.1 Problems File Share Manager Solves

- Centrally manages files, directories, members, user groups, roles, and button-level permissions within workspace boundaries.
- Supports chunked uploads for large files, resumable uploads, version-conflict checks for overwrite uploads, file-version recovery, and secure downloads.
- Creates immutable file versions or directory snapshots for external-link shares. Anonymous access does not expose real paths, workspaces, or object-storage addresses.
- Checks uploaded content against an extension allowlist, Magic Numbers, Office archive structures, VBA macro consistency, and ZIP security rules.
- Optionally integrates with ClamAV to scan uploaded files and manually rescanned files for viruses, retry failures, and display scan status.
- Maintains audit-log stream sequence numbers and hash chains globally or per workspace, with filtering, export, chain verification, and encrypted archiving.
- Reduces the risks of accidental deletion and storage failures through backup chains, recovery drills, incremental compaction, quarantine review, and storage health probes.
- Uses a notification Outbox, background Workers, and persistent task tables for batch downloads, audit exports, lifecycle cleanup, and reliable delivery.

### 1.2 Use Cases for File Share Manager

- Team-, department-, or project-level file sharing that requires member permissions and workspace isolation.
- Auditable records of uploads, downloads, shares, deletions, recoveries, and administrator operations.
- Internal platforms with governance requirements for file extensions, archive structures, virus scanning, external-link expiration, and download limits.
- Operations scenarios that need to retain online file objects, local or object-storage backups, archives, and recovery-drill capabilities at the same time.
- Small- and medium-scale deployments that already use MySQL, Nginx, Docker Compose, S3/MinIO/OSS, or LDAP.

### 1.3 Open-Source Repositories

| Platform | URL |
| --- | --- |
| GitHub | [github.com/ghl1024/file-share-manager](https://github.com/ghl1024/file-share-manager) |
| Gitee | [gitee.com/ghl1024/file-share-manager](https://gitee.com/ghl1024/file-share-manager) |
| CNB | [cnb.cool/ghl1024/file-share-manager](https://cnb.cool/ghl1024/file-share-manager) |
| GitCode | [gitcode.com/haydenguo/file-share-manager](https://gitcode.com/haydenguo/file-share-manager) |
| Author's website | [hayden.pub](https://hayden.pub) |

---

<a id="section-2"></a>

## 2. System Architecture

[![Runtime architecture](docs/架构/runtime-architecture.visual-check.1440x900.light.png)](docs/架构/runtime-architecture.html)

### 2.1 Architecture Overview

| Module | Description | Detailed Documentation |
| --- | --- | --- |
| Frontend | Vue 3 management console with login, dashboard, workspace, file-directory, sharing, audit, and system-management pages | [frontend/README.en.md](frontend/README.en.md) |
| Server | Go/Gin backend providing the `/api/fileshare/v1` API, authentication and authorization, business orchestration, background Workers, and data persistence | [docs/架构/runtime-architecture.md](docs/架构/runtime-architecture.md) |
| MySQL | Stores users, workspaces, role permissions, file versions, shares, audits, backup tasks, and the notification Outbox | [server/configs/config-dev.toml](server/configs/config-dev.toml) |
| POSIX Storage | Stores staged upload chunks, standard file objects, batch-download ZIP files, quarantined objects, and local backup directories | [server/internal/storage/posix.go](server/internal/storage/posix.go) |
| Backup Storage | Supports local, S3, MinIO, and OSS as object backends for backups, archives, and audit archives | [server/internal/storage/backup_factory.go](server/internal/storage/backup_factory.go) |

### 2.2 Technology Stack

| Layer | Main Technologies |
| --- | --- |
| Frontend | Vue 3.5, Vite 6.4, Vue Router 4, Pinia 3, Element Plus 2.14, Axios |
| Server | Go 1.26.5, Gin 1.12, GORM, MySQL Driver, zap, OpenTelemetry Trace |
| Authentication and permissions | JWT HS256, HttpOnly Cookie, bcrypt, LDAP, workspace RBAC, menu/button permissions |
| Files and security | POSIX atomic writes, chunked uploads, SHA-256, MIME checks, ZIP security checks, ClamAV |
| Tasks and reliability | MySQL persistent queues, in-process Workers, notification Outbox, task claim/requeue, exponential backoff |
| Backup and archiving | Local directories, S3, MinIO, OSS, AES-256-GCM manifests, backup-chain verification, recovery drills |

### 2.3 Core Data Flows

1. A user accesses `/fileshare/`. Nginx serves Vue static assets, and the frontend accesses the backend through `/api/fileshare/v1`.
2. After a successful login, the backend issues a `fileshare_session` HttpOnly Cookie. Management API requests pass through authentication, auditing, workspace-context, and permission middleware.
3. A file upload first creates a MySQL upload session, then writes chunks to staging. On completion, the chunks are merged into `objects/<workspace_id>/<uuid>`, and the SHA-256 digest and content structure are verified.
4. DAOs/GORM write file versions, nodes, quotas, share snapshots, and audit events to MySQL, while binary file objects remain in POSIX or archive storage.
5. Batch downloads, lifecycle cleanup, audit export/archiving, notification delivery, storage health checks, backup compaction, ClamAV retries, and LDAP synchronization run in Workers within the same Server process.
6. Backup and archive tasks read MySQL metadata and file objects, write them to local/S3/MinIO/OSS storage, and verify recoverability through manifests, digests, and chain anchors.

---

<a id="section-3"></a>

## 3. Feature Modules

| Domain | Main Capabilities |
| --- | --- |
| Workspaces | Workspace creation, member management, member quotas, available-user queries, cross-workspace access auditing |
| Users and organization | Local accounts, LDAP configuration and synchronization, user status, password resets, user groups, and memberships |
| Files and directories | Root directories, directory trees, folder creation, renaming, moving, favorites, recent access, recycle bin |
| Uploads and versions | Chunk initialization, chunk upload, status queries, merge completion, cancellation, overwrite versions, version recovery |
| Downloads and previews | Single-file downloads, version reads, text/binary preview limits, background generation of batch-download ZIP files |
| External-link sharing | File versions or directory snapshots, optional passwords, expiration times, maximum download counts, revocation, anonymous downloads |
| ACL and RBAC | Workspace roles, permission definitions, folder ACLs, inheritance switches, button-level permission checks |
| Collaboration | Node activity, comments, mention candidates, shared-with-me items, collaboration notifications |
| Audit center | Operation logs, security events, hash-chain verification, asynchronous CSV/JSON exports, encrypted archiving |
| Backup and recovery | Baseline backups, incremental backups, chain verification, recovery drills, workspace recovery, backup-chain compaction |
| Storage governance | Orphan-object scans, quarantine, reviewed recovery, expiration cleanup, capacity and writability probes |
| System governance | ClamAV health, notification channels, reliable-delivery Outbox, menu permissions, system-configuration checks |

| Component | Responsibilities | Out of Scope |
| --- | --- | --- |
| Frontend | Pages, routes, forms, permission presentation, upload interactions, and user experience | Does not store JWTs, connect to MySQL, or serve as the final security boundary |
| Server | Authentication and authorization, APIs, transaction orchestration, auditing, background tasks, object I/O, and health checks | Does not include a highly available database or replace external Secret management |
| MySQL | Metadata, permissions, task status, audit streams, Outbox records, and migration receipts | Does not store binary file content or act as object storage |
| POSIX/Backup Storage | File objects, staged chunks, backup objects, archive objects, and export files | Does not store business permission decisions or provide final audit semantics |

---

<a id="section-4"></a>

## 4. System Preview

This repository provides an interactive runtime architecture diagram. Click the image below to open the HTML version with guided views, search, focus, theme switching, and export capabilities.

[![Runtime architecture preview](docs/架构/runtime-architecture.visual-check.2048x1320.light.png)](docs/架构/runtime-architecture.html)

No public demo site is currently available. You can quickly start the complete system by following [Local Development - Binary Deployment](#61-local-development---binary-deployment) or [Local Development - Docker Compose](#62-local-development---docker-compose).

---

<a id="section-5"></a>

## 5. Project Documentation

| Document | Contents |
| --- | --- |
| [Runtime Architecture](docs/架构/runtime-architecture.md) | Runtime units, request paths, background Workers, data boundaries, and object boundaries |
| [Interactive Architecture Diagram](docs/架构/runtime-architecture.html) | Standalone HTML architecture diagram generated by Archify |
| [Production Release and Rollback](docs/PRODUCTION_RUNBOOK.md) | Production prerequisites, migrations, Secrets, monitoring, rollback, and items still requiring confirmation |
| [Frontend Documentation](frontend/README.en.md) | Frontend runtime, build, request, and permission conventions |
| [Contributor Guide](CONTRIBUTING.md) | Development workflow, commit conventions, and checks |
| [Changelog](CHANGELOG.md) | Version history and unreleased changes |

---

<a id="section-6"></a>

## 6. Deployment Options

The project supports the following four deployment modes, based on whether the environment is for development or production and whether it runs as processes or containers:

| Mode | Use Case | Artifact Source | Current Support Status |
| --- | --- | --- | --- |
| Local development binary deployment | Source development, debugging, and single-component troubleshooting | Local `go run` / `npm run dev` | Ready to use |
| Local development Docker Compose | Quickly try the complete MySQL, Server, and Web stack | Locally built images | Ready to use |
| Production binary deployment | VMs, physical machines, systemd, and existing Nginx environments | Release or trusted build | Follow the production runbook |
| Production container deployment | Container platforms, production Compose, or orchestration systems | Pinned-version images | Configure for the actual environment |

> `server/configs/config-dev.toml` and `docker-compose.yml` contain development defaults and may only be used in controlled development environments. Production environments must use separate Secrets, strong passwords, absolute storage paths, and a formal migration process.

### 6.1 Local Development - Binary Deployment

Prerequisites: Go 1.26.5, Node.js 22, npm, and MySQL 8.x.

#### 6.1.1 Prepare MySQL

You can use a local MySQL installation or quickly start a development database with Docker. By default, the development configuration connects to `127.0.0.1:3306` with database name `fileshare`, user `fileshare`, and password `fileshare123`.

```bash
docker run -d \
  --name fileshare-mysql-dev \
  -e MYSQL_ROOT_PASSWORD='fileshare_root' \
  -e MYSQL_DATABASE='fileshare' \
  -e MYSQL_USER='fileshare' \
  -e MYSQL_PASSWORD='fileshare123' \
  -p 3306:3306 \
  mysql:8.4

docker exec fileshare-mysql-dev mysqladmin ping -h 127.0.0.1 -uroot -pfileshare_root
```

#### 6.1.2 Start the Server

```bash
git clone https://github.com/ghl1024/file-share-manager.git
cd file-share-manager/server
go mod download

export FILESHARE_BOOTSTRAP_ADMIN_PASSWORD='Admin123456789!'
go run ./cmd/server --env=dev
```

The backend listens on `http://127.0.0.1:29000` by default. The development configuration enables `auto_migrate`; on its first start, the server automatically creates tables, seeds permissions and menus, and creates the `admin` user if no super administrator exists.

```bash
curl -fsS http://127.0.0.1:29000/healthz
curl -fsS http://127.0.0.1:29000/readyz
```

#### 6.1.3 Start the Frontend

Open another terminal:

```bash
cd file-share-manager/frontend
npm install
npm run dev
```

Visit [http://127.0.0.1:39000/fileshare/](http://127.0.0.1:39000/fileshare/). The Vite development proxy forwards `/api/fileshare/v1` to `http://localhost:29000`. If the port is already in use, Vite automatically selects the next available port.

#### 6.1.4 Build and Run Locally

```bash
cd file-share-manager
mkdir -p bin
go -C server build -trimpath -o ../bin/fileshare-server ./cmd/server
go -C server build -trimpath -o ../bin/fs-migrate ./cmd/migrate
npm --prefix frontend run build

FILESHARE_BOOTSTRAP_ADMIN_PASSWORD='Admin123456789!' ./bin/fileshare-server --env=dev
```

### 6.2 Local Development - Docker Compose

Prerequisites: Docker Engine and Docker Compose v2.

```bash
git clone https://github.com/ghl1024/file-share-manager.git
cd file-share-manager

export MYSQL_ROOT_PASSWORD="$(openssl rand -base64 32)"
export FILESHARE_DB_PASSWORD="$(openssl rand -base64 32)"
export FILESHARE_JWT_SECRET="$(openssl rand -base64 48)"
export FILESHARE_BACKUP_MANIFEST_KEY="$(openssl rand -base64 32)"
export FILESHARE_BOOTSTRAP_ADMIN_PASSWORD="$(openssl rand -base64 24)"

make compose-up
docker compose ps
```

Compose does not provide defaults for the production database password, JWT Secret, MySQL root password, or backup-manifest key. If any variable is missing, startup fails before the services run. After the initial administrator has been created, subsequent starts no longer need `FILESHARE_BOOTSTRAP_ADMIN_PASSWORD`.

Standard GitHub and CNB `push` / `pull_request` checks, as well as Docker builds triggered by regular branches, are automatically skipped when a commit contains only changes under `.github/`, `.cnb/`, `.cnb.yml`, `.goreleaser.yaml`, or `push.sh`. Checks still run normally whenever the same commit also includes business-code changes.

Compose first waits for MySQL to become ready, then runs versioned migrations in a one-shot `fileshare-migrate` container. After migration succeeds, it starts `fileshare-server`; the `fileshare-web` frontend waits for the backend `/readyz` endpoint to pass before serving the application.

| Service | URL |
| --- | --- |
| Management console | [http://localhost:39000/fileshare/](http://localhost:39000/fileshare/) |
| Server liveness check | [http://localhost:29000/healthz](http://localhost:29000/healthz) |
| Server readiness check | [http://localhost:29000/readyz](http://localhost:29000/readyz) |

Stop the services while preserving data:

```bash
make compose-down
```

Run the following only when you need to destroy test data:

```bash
docker compose down --volumes
```

### 6.3 Production - Binary Deployment

In production, plan the Frontend static assets, Server process, MySQL, primary storage, and backup storage as separate failure domains. Before release, read the complete [Production Release and Rollback Runbook](docs/PRODUCTION_RUNBOOK.md).

Production release sequence:

1. Prepare MySQL 8, with separate accounts planned for migrations, business requests, and audit archiving.
2. Prepare the primary-storage, staging, and backup directories. Use absolute paths and confirm support for atomic rename, reliable fsync, and POSIX permissions.
3. Inject `FILESHARE_DB_PASSWORD`, `FILESHARE_JWT_SECRET`, `FILESHARE_BACKUP_MANIFEST_KEY`, and the first-start-only `FILESHARE_BOOTSTRAP_ADMIN_PASSWORD` through a Secret-management system.
4. Run versioned migrations, then run `--verify`.
5. Start the Server and check `/healthz` and `/readyz`.
6. Publish the Frontend build artifacts to Nginx, serve them under `/fileshare/` with History fallback, and proxy `/api/` to the Server.
7. Validate login, workspace switching, upload and download, sharing, permissions, backups, the audit chain, and recovery drills.

Example commands:

```bash
cd server
FILESHARE_DB_PASSWORD='...' FILESHARE_JWT_SECRET='...' FILESHARE_BACKUP_MANIFEST_KEY='...' \
  go run ./cmd/migrate --config configs/config-prod.toml

FILESHARE_DB_PASSWORD='...' FILESHARE_JWT_SECRET='...' FILESHARE_BACKUP_MANIFEST_KEY='...' \
  go run ./cmd/migrate --config configs/config-prod.toml --verify

FILESHARE_DB_PASSWORD='...' FILESHARE_JWT_SECRET='...' FILESHARE_BACKUP_MANIFEST_KEY='...' \
  go run ./cmd/server --env=prod
```

The migration command uses a MySQL advisory lock and records the version, name, SHA-256 checksum, and elapsed time in `schema_migrations`. Repeated runs safely skip completed versions. When the production backend starts, it verifies the current migration version and critical schema again, and refuses to serve traffic if anything is missing or a receipt does not match.

### 6.4 Production - Container Deployment

Production container deployments must meet these requirements:

- Use pinned-version images, not the untraceable `latest` tag.
- Use external MySQL or independently managed highly available MySQL; do not rely on the development Compose default password.
- Inject the database password, JWT Secret, backup-manifest key, notification-credential encryption key, and object-storage credentials through Secrets.
- Mount the same primary storage on every Server instance, or use a consistent cloud-mount strategy. The backup directory must not be inside the primary-storage directory.
- Provide HTTPS, SPA History fallback, and an `/api/` reverse proxy at the Frontend entry point.
- Configure `/readyz` as the readiness probe. Monitor MySQL; primary-, staging-, and backup-storage capacity; audit archiving; backup chains; and the notification Outbox.
- Keep automatic archiving and audit archiving disabled at first. Enable them according to the organization's retention policy only after completing a baseline backup and a recovery drill.

---

<a id="section-7"></a>

## 7. Login and Verification

When starting the Server against a new database for the first time, create the super administrator with the following variable:

```bash
export FILESHARE_BOOTSTRAP_ADMIN_PASSWORD='Replace-With-A-Strong-Admin-Password!'
```

The default administrator username is `admin`. After the administrator has been created, remove the one-time password variable from the runtime environment. To explicitly change the password in an existing environment, use:

```bash
cd server
FILESHARE_ADMIN_PASSWORD='Replace-With-A-Strong-Admin-Password!' \
  go run ./cmd/admin-password --config configs/config-dev.toml
```

After deployment, verify at least the following:

```bash
curl -fsS http://127.0.0.1:29000/healthz
curl -fsS http://127.0.0.1:29000/readyz
curl -fsSI http://127.0.0.1:39000/fileshare/
```

`/healthz` only checks whether the process is alive; `/readyz` actually pings MySQL. When audit archiving is enabled, `/readyz` also checks the audit-archive database connection.

Common check commands:

```bash
make test
make vet
make swag
make frontend-build
```

---

<a id="section-8"></a>

## 8. Runtime Configuration

### 8.1 Security and Sessions

- The API prefix is `/api/fileshare/v1/`.
- Sessions use the `fileshare_session` HttpOnly Cookie; the frontend neither reads nor persists JWTs.
- Cookie-authenticated write requests must include `X-Requested-With: XMLHttpRequest`.
- By default, Gin does not trust any forwarded proxy headers. When deployed behind a reverse proxy, inject exact proxy IPs/CIDRs through `FILESHARE_TRUSTED_PROXIES`, separated by commas. Do not configure an unrestricted address range.
- Use a random value of at least 32 bytes for the JWT Secret, for example: `openssl rand -base64 48`.
- Production configuration disables `database.auto_migrate`; a separate migration process is required.

### 8.2 Swagger API Documentation

- After changing API routes, requests, or responses, run `make swag` and commit the generated `server/docs/docs.go`, `swagger.json`, and `swagger.yaml`.
- Swagger is disabled by default. It is available at `http://127.0.0.1:29000/swagger/index.html` only after setting `FILESHARE_ENABLE_SWAGGER=true` in debug mode.
- Release mode never registers Swagger routes, even when `FILESHARE_ENABLE_SWAGGER=true` is set.
- Swagger UI supports `Authorization: Bearer <token>`; routine login through the browser frontend still uses the `fileshare_session` HttpOnly Cookie.
- After using the login endpoint in Swagger UI to establish a Cookie session, POST, PUT, and DELETE requests must also set `X-Requested-With` to `XMLHttpRequest`.

### 8.3 Uploads, Sharing, and Content Checks

- The upload extension allowlist is controlled by `[upload].allowed_extensions` or `FILESHARE_ALLOWED_EXTENSIONS`.
- The per-file size limit is controlled by `[upload].max_file_bytes` or `FILESHARE_UPLOAD_MAX_FILE_BYTES`. It defaults to 100 GiB and may range from 1 MiB to 1 TiB.
- Chunk sizes must be between 1 MiB and 64 MiB. The system also limits each upload session to at most 1,048,576 chunks.
- After changing upload-session constraints, run versioned migrations in production before starting the Server.
- By default, the system validates Magic Numbers for common formats, Office archive structures, VBA macro/extension consistency, and ZIP entry counts, decompressed sizes, compression ratios, nesting depth, and path safety.
- Basic content/extension matching is skipped only when `allow_mime_mismatch = true` is explicitly set. ZIP structural security checks always run.
- Share links freeze a file version or directory snapshot; the raw token is returned only in the successful creation response.

### 8.4 ClamAV

ClamAV is an optional dependency. After `[clamav].host` or `FILESHARE_CLAMAV_HOST` is configured, files that fail upload scans or manual rescans enter a persistent automatic retry queue. By default, the system retries up to three times with a five-minute base interval and exponential backoff.

### 8.5 Storage Health and Lifecycle

- By default, storage health checks perform real write, `fsync`, and delete probes and read capacity for primary storage, staging, and local backup directories every five minutes.
- The system raises a warning when the available ratio falls below 20%, and escalates it to high priority below 10% or when available capacity is less than 5 GiB.
- Lifecycle tasks periodically clean up expired upload sessions, expired shares, recycle-bin objects past their retention period, expired batch-download ZIP files, and audit-export files.
- Orphan objects first enter quarantine, where they are retained for seven days by default. At expiration, the system checks file versions, external-link snapshots, and batch-download snapshots again, and automatically restores any object that is still referenced.

### 8.6 Auditing, Backup, and Archiving

- The audit center maintains independent stream sequence numbers and hash chains globally or per workspace, and supports filtering by subject, type, risk, result, target, IP, request ID, and time.
- Audit exports are asynchronous CSV/JSON tasks; exported files are retained for 24 hours by default.
- Audit archiving is disabled by default. When enabled, a separate `[audit_database]` account is required, and service startup verifies that the business account cannot update or delete audit events.
- Backup-chain compaction is disabled by default. After `[backup].compaction_enabled` is enabled, a Worker generates and verifies a new independent baseline when the number of consecutive incremental backups reaches the threshold.
- Generate the backup-manifest key with `openssl rand -base64 32`. Historical encrypted backups cannot be recovered if the key is lost.

---

<a id="section-9"></a>

## 9. Contributing

Issues, documentation improvements, and Pull Requests are welcome. Before starting development, read the [Contributor Guide](CONTRIBUTING.md) and ensure that the relevant tests and builds pass:

```bash
make test
make vet
make frontend-build
```

---

<a id="section-10"></a>

## 10. License

- Copyright (c) 2026 HaydenGuo.
- This project is open source under the [Apache License 2.0](LICENSE). You may use, modify, and distribute it in compliance with the license; retain the license and applicable copyright notices when distributing it.
