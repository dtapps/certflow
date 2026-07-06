# CertFlow Development Guide

English | [中文](DEVELOPMENT.md)

## Overview

CertFlow is an SSL certificate management tool supporting certificate issuance, renewal, and expiration monitoring. Built with Go + Vue 3 cross-platform desktop application architecture.

---

## Tech Stack

### Backend

| Technology | Purpose |
|------------|---------|
| Go 1.27 | Backend language |
| Wails v3 | Desktop application framework (Go + frontend hybrid) |
| Ent (entgo.io/ent) | ORM framework (code-generation based) |
| lego v5 (go-acme/lego) | ACME protocol client (certificate issuance/renewal) |
| gocron/v2 | Scheduled task scheduler (auto-renewal, expiration checks) |
| modernc.org/sqlite | Embedded database (pure Go, no CGO required) |
| golang.org/x/crypto | Cryptography tools (bcrypt, etc.) |

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
├── main.go                    # Entry point
├── system_service.go          # System service (theme/window management)
├── cert_service.go            # Certificate service (Wails wrapper)
├── ca_service.go              # CA management service
├── dns_service.go             # DNS provider service
├── auth_service.go            # Authentication service
├── scheduler_service.go       # Scheduled task service
├── monitor_service.go         # Domain monitoring service
├── notification_service_wrapper.go  # Notification service
├── settings_service.go        # Settings service
├── logging_service_wrapper.go # Logging service
├── ent/                       # Ent ORM generated code
│   ├── schema/                # Database model definitions
│   │   ├── ca.go              # CA entity
│   │   ├── certificate.go     # Certificate entity
│   │   ├── dns_provider.go    # DNS provider entity
│   │   ├── monitored_domain.go # Monitored domain entity
│   │   ├── notification.go    # Notification entity
│   │   └── renewal_log.go     # Renewal log entity
│   └── ...
├── internal/                  # Internal implementations
│   ├── auth/                  # Authentication service
│   ├── ca/                    # CA management
│   ├── certificate/           # Certificate issuance/renewal/revocation
│   ├── dnsprovider/           # DNS provider management
│   ├── db/                    # Database initialization
│   ├── i18n/                  # Internationalization (embedded locale files)
│   ├── logging/               # Logging system (rotation/compression)
│   ├── monitor/               # Domain monitoring
│   ├── network/               # Network utilities (DNS/proxy)
│   ├── notification/          # Notification service
│   ├── scheduler/             # Scheduled task scheduler
│   └── settings/              # Application settings
├── frontend/                  # Frontend application
│   ├── src/
│   │   ├── views/             # Page views
│   │   │   ├── Dashboard.vue
│   │   │   ├── Certificates.vue
│   │   │   ├── CertApply.vue
│   │   │   ├── CertDetail.vue
│   │   │   ├── CAConfig.vue
│   │   │   ├── DNSProviders.vue
│   │   │   ├── Monitor.vue
│   │   │   ├── Settings.vue
│   │   │   ├── PersonalCenter.vue
│   │   │   └── LogViewer.vue
│   │   ├── components/        # Shared components
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
│   ├── darwin/                # macOS build
│   ├── windows/               # Windows build
│   └── linux/                 # Linux build
├── .github/workflows/         # GitHub Actions workflows
│   └── release.yml            # Six-platform release
├── .cnb/workflows/            # CNB workflows
│   └── release.yml            # Download from GitHub and release
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
make package           # Package macOS app (make package VERSION=1.0.0)
make go-build          # Quick compile (Go backend only)
make lint              # Go code linting (golangci-lint)
make lint-fix          # Go code linting (auto-fix)
make check             # Frontend TypeScript type checking
make test              # Go backend tests
make bindings          # Generate Wails TypeScript bindings
make ent               # Generate Ent ORM code
make format            # Format all code (Go + Vue/TS)
make format-go         # Format Go code
make format-frontend   # Format frontend code (Prettier)
make clean             # Clean build artifacts
```

---

## Version Injection

Build-time injection via Taskfile BUILD_FLAGS injects version, build time, and Git commit ID:

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

---

## Cross-Platform Build

| Platform | Command | Output |
|----------|---------|--------|
| macOS | `wails3 task package` | `bin/certflow.app` |
| Windows | `wails3 task build` | `bin/certflow.exe` |
| Linux | `wails3 task build` | `bin/certflow` |

### Release Workflow

- **GitHub Actions**: Manual trigger with version input, six-platform parallel build, publishes GitHub Release
- **CNB**: Manual trigger, downloads artifacts from GitHub Release and publishes locally

---

## Database

SQLite is used as an embedded database, managed via Ent ORM. Entity models are defined in `ent/schema/`:

- **CA** — Certificate authority configuration
- **Certificate** — SSL certificate information
- **DNSProvider** — DNS provider configuration
- **MonitoredDomain** — Monitored domains
- **Notification** — Notification records
- **RenewalLog** — Certificate renewal logs

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
make lint              # Run golangci-lint
make lint-fix          # Auto-fix
```

Enabled linters: errcheck, govet, staticcheck, ineffassign, misspell, unconvert, nilerr, errorlint, bodyclose, contextcheck, noctx, gosec.

---

## Testing

```bash
make test              # Run all tests
```

12 packages with test coverage: auth, ca, certificate, db, dnsprovider, i18n, logging, monitor, network, notification, scheduler, settings.

---

## Notes

1. **Ent code generation**: After modifying `ent/schema/`, run `make ent` to regenerate ORM code
2. **Binding generation**: After modifying Go services, run `make bindings` to regenerate frontend bindings
3. **`wails3 build` does not support `-ldflags`**: Version is injected via Taskfile BUILD_FLAGS
4. **Linux cross-compilation**: Cannot cross-compile Linux from macOS (requires CGO + webkit2gtk), use GitHub Actions instead
5. **Naive UI is auto-imported**: Use `<n-xxx>` components directly in templates, no global registration needed
6. **Pinia state management**: Use `useXxxStore()` to get store, `storeToRefs()` for reactive properties
