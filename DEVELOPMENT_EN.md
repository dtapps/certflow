# CertFlow Development Guide

English | [中文](DEVELOPMENT.md)

## Overview

CertFlow is an SSL certificate management tool supporting certificate issuance, renewal, and expiration monitoring. Built with Go + Vue 3 cross-platform desktop application architecture.

---

## Tech Stack

### Backend

| Technology | Purpose |
|------------|---------|
| Go 1.26 | Backend language |
| Wails v3 | Desktop application framework (Go + frontend hybrid) |
| Ent (entgo.io/ent) | ORM framework (code-generation based) |
| lego v5 (go-acme/lego) | ACME protocol client (certificate issuance/renewal) |
| gocron/v2 | Scheduled task scheduler (auto-renewal, expiration checks) |
| modernc.org/sqlite | Embedded database (pure Go, no CGO required) |
| golang.org/x/crypto | Cryptography tools (bcrypt, etc.) |
| go-webauthn/webauthn | Passkey / WebAuthn authentication |
| pquerna/otp | TOTP two-factor authentication (2FA) |
| google/uuid | Unique ID generation (v7) |
| spf13/viper | Configuration management |
| fsnotify/fsnotify | Filesystem watching |

### Frontend

| Technology | Purpose |
|------------|---------|
| Vue 3 | Progressive frontend framework |
| vue-router | SPA routing |
| TypeScript | Type-safe language |
| Vite 8 | Build tool / dev server |
| Tailwind CSS v4 | Utility-first CSS framework |
| Naive UI v2 | Vue 3 component library |
| Pinia | State management |
| @vicons/ionicons5 | Icon library |
| @vueuse/core | Vue composition API utilities |
| @wailsio/runtime | Wails frontend-backend communication runtime |
| qrcode | TOTP QR code generation |

### Build & Deploy

| Tool | Purpose |
|------|---------|
| Makefile | Development command wrapper |
| Taskfile.yml | Cross-platform build system |
| golangci-lint v2 | Go code linting |
| Prettier | Frontend code formatting |
| pnpm | Frontend package manager |
| wails3 CLI | Wails build/dev/code generation |

---

## Project Structure

```
certflow/
├── main.go                    # Entry point (version/build-info injection, app assembly, updater)
├── *_service.go               # Wails service wrappers (one file per domain)
│   ├── system_service.go      # System service (theme/window/menu)
│   ├── cert_service.go        # Certificate service
│   ├── ca_service.go          # CA management service
│   ├── dns_service.go         # DNS provider service
│   ├── auth_service.go        # Authentication service
│   ├── scheduler_service.go   # Scheduled task service
│   ├── monitor_service.go     # Domain monitoring service
│   ├── scanner_service.go     # Certificate scanning service
│   ├── deploy_service.go      # Certificate deployment service (cloud CDN/WAF/LB, etc.)
│   ├── deploy_credential_service.go # Deployment credential management service
│   ├── notification_service_wrapper.go  # Notification service
│   ├── settings_service.go    # Settings service
│   ├── logging_service_wrapper.go # Logging service
│   ├── clipboard_service.go   # Clipboard service
│   ├── browser_service.go     # Open-in-browser service
│   ├── file_service.go        # File picker service
│   ├── dock_service.go        # macOS Dock service
│   ├── window_service.go      # Window service
│   ├── systray_service.go     # System tray service
│   └── autostart_service.go   # Auto-start at login service
├── internal/                  # Internal implementations
│   ├── ent/                   # Ent ORM generated code (moved from root ent/)
│   │   ├── schema/            # Database model definitions
│   │   │   ├── ca.go          # CA entity
│   │   │   ├── certificate.go # Certificate entity
│   │   │   ├── cert_upload.go # Uploaded certificate entity
│   │   │   ├── dns_provider.go # DNS provider entity
│   │   │   ├── deploy_target.go # Deploy target entity (provider/service/region config)
│   │   │   ├── deploy_credential.go # Deploy credential entity
│   │   │   ├── deploy_log.go  # Deploy log entity
│   │   │   ├── provider_types.go # Provider/service type enum definitions
│   │   │   ├── monitored_domain.go # Monitored domain entity
│   │   │   ├── notification.go # Notification entity
│   │   │   ├── renewal_log.go # Renewal log entity
│   │   │   ├── auth_method.go # Auth method entity
│   │   │   ├── totp_credential.go # TOTP credential entity (2FA)
│   │   │   ├── passkey_credential.go # Passkey credential entity (WebAuthn)
│   │   │   └── scan_result.go # Scan result entity
│   │   └── ...
│   ├── httplog/               # HTTP request logging (sqlc-generated, separate DB)
│   ├── auth/                  # Authentication service (password/TOTP/Passkey)
│   ├── ca/                    # CA management
│   ├── certificate/           # Certificate issuance/renewal/revocation/upload
│   ├── db/                    # Database initialization
│   ├── deploy/                # Certificate deployment (per-provider: aliyun/tencent/huawei/baidu/ctyun/volcengine)
│   ├── deploycredential/      # Deployment credential management
│   ├── dnsprovider/           # DNS provider management
│   ├── events/                # Frontend-backend event definitions
│   ├── i18n/                  # Internationalization (embedded locale files)
│   ├── logging/               # Logging system (rotation/compression)
│   ├── monitor/               # Domain monitoring
│   ├── network/               # Network utilities (DNS/proxy)
│   ├── notification/          # Notification service
│   ├── scanner/               # Certificate scanning
│   ├── scheduler/             # Scheduled task scheduler
│   └── settings/              # Application settings
├── frontend/                  # Frontend application
│   ├── src/
│   │   ├── views/             # Page views
│   │   │   ├── Dashboard.vue          # Dashboard (overview/stats)
│   │   │   ├── Certificates.vue        # Certificate list (manage/renew/revoke)
│   │   │   ├── CertApply.vue           # Certificate application (form/submit)
│   │   │   ├── CertDetail.vue          # Certificate detail (view/export/deploy)
│   │   │   ├── CAConfig.vue            # CA configuration (private CA management)
│   │   │   ├── Providers.vue           # DNS providers entry
│   │   │   ├── DNSProviders.vue        # DNS providers (credentials/verification)
│   │   │   ├── DeployTargets.vue       # Deploy target list (manage/execute deployment)
│   │   │   ├── DeployTargetForm.vue    # Deploy target create/edit form
│   │   │   ├── DeployCredentials.vue   # Deploy credentials entry
│   │   │   ├── CredentialList.vue      # Deploy credential list (management)
│   │   │   ├── Monitor.vue             # Domain monitoring (health/expiry alerts)
│   │   │   ├── Scan.vue                # Certificate scanning (aggregate/validity)
│   │   │   ├── Settings.vue            # Settings (app/update/about)
│   │   │   ├── PersonalCenter.vue      # Personal center (account/2FA/Passkey)
│   │   │   └── LogViewer.vue           # Log viewer (runtime/renewal logs)
│   │   ├── components/        # Shared components
│   │   │   ├── TitleBar.vue   # Custom title bar (window controls)
│   │   │   ├── Sidebar.vue    # Sidebar navigation
│   │   │   ├── TopBar.vue     # Top toolbar
│   │   │   └── LoginDialog.vue # Login dialog
│   │   ├── router/            # Route configuration
│   │   ├── stores/            # Pinia state management
│   │   │   ├── i18n.ts        # Internationalization
│   │   │   ├── theme.ts       # Theme management
│   │   │   └── notifications.ts # Notification management
│   │   ├── utils/             # Utility functions
│   │   ├── style.css          # Global styles
│   │   └── main.ts            # Entry point
│   ├── package.json
│   ├── .prettierrc            # Prettier config
│   └── vite.config.ts
├── build/                     # Build configuration
│   ├── config.yml             # Wails app config
│   ├── Taskfile.yml           # Common build tasks
│   ├── darwin/                # macOS build/sign
│   ├── windows/               # Windows build/sign
│   └── linux/                 # Linux build/sign
├── .github/workflows/         # GitHub Actions workflows
│   ├── release.yml            # Stable release (six platforms)
│   └── nightly.yml            # Daily build (nightly pre-release)
├── .cnb/workflows/            # CNB workflows
│   └── release.yml            # Download artifacts from GitHub Release and publish to CNB
├── .golangci.yml              # Go linting configuration
├── Makefile                   # Make commands
├── Taskfile.yml               # Task build entry point
└── go.mod                     # Go module definition
```

---

## Development Commands

```bash
make help              # View all commands
make deps              # Install all dependencies (Go + frontend)
make dev               # Start Wails development mode
make build             # Build production package (make build VERSION=1.0.0)
make package           # Package application (make package VERSION=1.0.0)
make go-build          # Quick compile (Go backend only)
make lint-go           # Go code linting (golangci-lint)
make lint-go-fix       # Go code linting (auto-fix)
make lint-frontend     # Frontend TypeScript type checking (vue-tsc)
make check             # Lint and test (lint-go + lint-frontend + test-go)
make test              # Go backend tests
make bindings          # Generate Wails TypeScript bindings
make ent               # Generate Ent ORM code
make sqlc              # Generate sqlc code (internal/httplog)
make format            # Format all code (Go + Vue/TS)
make format-go         # Format Go code
make format-frontend   # Format frontend code (Prettier)
make clean             # Clean build artifacts
```

---

## Version Injection

Build-time injection uses each platform's Taskfile `-ldflags -X` to set the version, build time, Git commit ID, and update token (defaults `dev` / empty). Controlled via `wails3 task build VERSION=x BUILD_TIME=... GIT_COMMIT=... GITHUB_TOKEN=...`:

```bash
# Via wails3 task (recommended)
wails3 task build VERSION=1.0.0

# Via Makefile
make build VERSION=1.0.0
```

Injected variables (`main.go`):
- `currentVersion` — Version string (default `dev`)
- `buildTime` — Build timestamp (UTC)
- `gitCommit` — Git short commit hash
- `githubToken` — GitHub token for the updater (injected by CI; empty locally)

---

## Cross-Platform Build

| Platform | Command | Output |
|----------|---------|--------|
| macOS | `wails3 task package` | `bin/certflow.app` |
| Windows | `wails3 task build` | `bin/certflow.exe` |
| Linux | `wails3 task build` | `bin/certflow` |

### Release Workflow

- **GitHub Actions (Release)**: Manual trigger with version input, six-platform parallel build (macOS arm64 / amd64, Linux amd64 / arm64, Windows amd64 / arm64), publishes GitHub Release
- **GitHub Actions (Nightly)**: Automatic daily build of `X.Y.Z-nightly` pre-release at UTC 00:00 (tag `nightly`)
- **CNB**: Manual trigger, downloads artifacts from GitHub Release and publishes locally

### Code Signing

After packaging, CI signs the artifacts for each platform. **Steps run only when the corresponding secrets are configured; otherwise they auto-skip without breaking the build**:

- **macOS**: Apple Developer ID certificate + notarization (`darwin:sign:notarize`), requires `APPLE_DEVELOPER_ID_CERTIFICATE` / `APPLE_DEVELOPER_ID_CERTIFICATE_PASSWORD` / `APPLE_NOTARIZER_KEYCHAIN_PROFILE` secrets. Without signing+notarization, Gatekeeper blocks the app ("cannot verify developer / file is damaged")
- **Windows**: Authenticode signing — installer first (`windows:sign:installer`), then the standalone exe (`windows:sign`), requires `WINDOWS_SIGN_CERTIFICATE` (base64 of .pfx) / `WINDOWS_SIGN_CERTIFICATE_PASSWORD` / `WINDOWS_SIGN_THUMBPRINT` / `WINDOWS_TIMESTAMP_SERVER` secrets, to reduce SmartScreen warnings
- **Linux**: OS does not enforce verification, so no code signing is required

> Auto-update relies only on `SHA256SUMS` checksum verification from the GitHub Release (native wails support); update packages are not ed25519 signature-verified

---

## Database

SQLite is used as an embedded database, managed via Ent ORM. Entity models are defined in `internal/ent/schema/`:

- **CA** — Certificate authority configuration
- **Certificate** — SSL certificate information
- **CertUpload** — Uploaded external certificates
- **DNSProvider** — DNS provider configuration
- **DeployTarget** — Deploy target (provider/service/region)
- **DeployCredential** — Deploy credentials
- **DeployLog** — Deployment execution logs
- **MonitoredDomain** — Monitored domains
- **Notification** — Notification records
- **RenewalLog** — Certificate renewal logs
- **ScanResult** — Certificate scan results

> HTTP request logs use a standalone sqlc-implemented `internal/httplog/` (single `http_log` table) stored in a separate database `dataDir/data/httplog.db`, enabled only at DEBUG level.

### SQLite Driver Benchmark

Test environment: Apple M1, Go 1.24+, in-memory (`:memory:`), `benchtime=3s`. Benchmark code in `internal/sqlitetest/`.

#### Latency Comparison (ns/op, lower is better)

| Operation | modernc (pure Go) | mattn (CGO) | mattn advantage |
|------|:---:|:---:|:---:|
| **Insert** (single row) | 8,603 | 5,095 | 1.69x faster |
| **InsertBulk** (100 rows/txn) | 382,696 | 194,203 | 1.97x faster |
| **Select** (by primary key) | 5,024 | 3,343 | 1.50x faster |
| **SelectRows** (scan 100 rows) | 41,587 | 38,949 | 1.07x faster |
| **Update** | 4,848 | 3,065 | 1.58x faster |
| **Delete** | 7,968 | 6,489 | 1.23x faster |
| **QueryLike** (5K rows LIKE) | 2,415,319 | 1,785,577 | 1.35x faster |

#### Memory Allocation Comparison (B/op | allocs/op)

| Operation | modernc | mattn |
|------|:---:|:---:|
| **Insert** | 344 / 14 | 400 / 14 |
| **InsertBulk** | 31,886 / 1309 | 30,419 / 1118 |
| **Select** | 735 / 27 | 798 / 30 |
| **SelectRows** | 6,952 / 517 | 7,024 / 520 |
| **Update** | 176 / 7 | 248 / 8 |
| **Delete** | 184 / 9 | 248 / 10 |
| **QueryLike** | 397,722 / 29663 | 397,770 / 29664 |

#### Conclusions

- **mattn (CGO) is faster in all operations**, especially writes (InsertBulk 97% faster), because it directly calls the C SQLite amalgamation while modernc is a pure-Go translation with emulation overhead.
- **Memory allocation is roughly equivalent** — modernc slightly better in single-row ops (~20–30B less), mattn ~5% less in bulk writes.
- **Heavy read gap narrows** (SelectRows only 1.07x) as driver overhead is diluted by SQL execution cost.
- **Current strategy**: default to modernc (pure Go, zero CGO, simplest cross-compilation); mattn available via build tag `sqlite_mattn` + `CGO_ENABLED=1`; windows/arm64 unsupported, CI falls back to modernc. This strategy remains sound.

---

## Certificate Deployment

Deploy issued/uploaded certificates to various cloud services. Implemented in `internal/deploy/`, split per provider (`aliyun.go`, `tencent.go`, `huawei.go`, `baidu.go`, `ctyun.go`, `volcengine.go`), with `service.go` dispatching.

Supported providers and services:

| Provider | Supported Services |
|----------|--------------------|
| Alibaba Cloud (aliyun) | CDN, DCDN, ESA, GA |
| Tencent Cloud (tencentcloud) | CDN, EdgeOne, ECDN |
| Huawei Cloud (huawei) | CDN, WAF, ELB |
| Baidu Cloud (baiducloud) | CDN, DRCDN |
| CTYun (ctyun) | CTCDN, ICDN, AccessOne |
| Volcengine (volcengine) | CDN, DCDN |

- Credential source supports two modes: a standalone **deploy credential** (`internal/deploycredential/`) or **reusing a DNS provider credential** (`credential_source`).
- Frontend pages: `DeployTargets.vue` (list/execute), `DeployTargetForm.vue` (create/edit), `DeployCredentials.vue` + `CredentialList.vue` (credential management).

---

## HTTP Request Logging

`internal/httplog/` records outbound HTTP traffic to a separate SQLite database (sqlc-implemented, single `http_log` table) under the DEBUG log level:

- Provides `WrapTransport(base)` / `WrapClient(client)` helpers — wraps under DEBUG, passes through otherwise.
- Already injected into `internal/network.BuildHTTPClient` and cloud SDK clients, covering scanner / monitor / certificate issuance (lego) / certificate deployment outbound requests.
- Storage: `schema.sql` (DDL) + `query.sql` (INSERT/DELETE) + `sqlc.yaml` (sqlc config), generated into `internal/httplog/db/` via `make sqlc` (`cd internal/httplog && sqlc generate`).
- Connection model: a long-lived `*sql.DB` is append-only (INSERT only); `Cleanup` (periodic deletion of old logs) opens a temporary independent connection from the saved DSN, runs DELETE, then closes it.

---

## Internationalization

- **Backend**: `internal/i18n/` uses go-i18n, locale files embedded in binary (`//go:embed`)
- **Frontend**: `frontend/src/stores/i18n.ts` with built-in Chinese and English (Pinia store)

All user-visible text must use `i18n.T()` (Go) or `t()` (Vue).

---

## Code Formatting

```bash
make format            # Format all code
make format-go         # Format Go code (gofmt)
make format-frontend   # Format frontend code (Prettier)
```

Frontend code uses Prettier for formatting, config at `frontend/.prettierrc`.

---

## Code Linting

```bash
make lint-go           # Run golangci-lint
make lint-go-fix       # Auto-fix
```

Enabled linters: errcheck, govet, staticcheck, ineffassign, misspell, unconvert, nilerr, errorlint, bodyclose, contextcheck, noctx, gosec.

---

## Testing

```bash
make test              # Run all tests
```

14 packages with test coverage: auth, ca, certificate, db, deploy, dnsprovider, i18n, logging, monitor, network, notification, scanner, scheduler, settings.

---

## Debugging Tips

### Title Bar Platform Testing

The title bar automatically adapts to macOS / Windows / Linux styles. During development, you can force a specific platform style via localStorage:

```js
// Simulate macOS title bar
localStorage.setItem('debug-platform', 'mac')

// Simulate Windows title bar
localStorage.setItem('debug-platform', 'win32')

// Simulate Linux title bar
localStorage.setItem('debug-platform', 'linux')

// Restore auto-detection
localStorage.removeItem('debug-platform')
```

Refresh the page after setting to see the effect.

---

## Notes

1. **Ent code generation**: After modifying `internal/ent/schema/`, run `make ent` to regenerate ORM code
2. **HTTP log code generation**: After modifying `schema.sql` / `query.sql` under `internal/httplog/`, run `make sqlc` to regenerate
3. **Binding generation**: After modifying Go services, run `make bindings` to regenerate frontend bindings
3. **Version injection**: `wails3 task build` does not accept a bare `-ldflags` flag; pass `VERSION=` etc. as Task variables, and each platform Taskfile assembles `-ldflags "-X main.currentVersion=..."`
4. **Linux cross-compilation**: Cannot cross-compile Linux from macOS (requires CGO + webkit2gtk), use GitHub Actions instead
5. **Naive UI is auto-imported**: Use `<n-xxx>` components directly in templates, no global registration needed
6. **Pinia state management**: Use `useXxxStore()` to get store, `storeToRefs()` for reactive properties
