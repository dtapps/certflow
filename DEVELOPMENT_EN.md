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
├── ent/                       # Ent ORM generated code
│   ├── schema/                # Database model definitions
│   │   ├── ca.go              # CA entity
│   │   ├── certificate.go     # Certificate entity
│   │   ├── dns_provider.go    # DNS provider entity
│   │   ├── monitored_domain.go # Monitored domain entity
│   │   ├── notification.go    # Notification entity
│   │   ├── renewal_log.go     # Renewal log entity
│   │   ├── auth_method.go     # Auth method entity
│   │   ├── totp_credential.go # TOTP credential entity (2FA)
│   │   ├── passkey_credential.go # Passkey credential entity (WebAuthn)
│   │   └── scan_result.go     # Scan result entity
│   └── ...
├── internal/                  # Internal implementations
│   ├── auth/                  # Authentication service (password/TOTP/Passkey)
│   ├── biometric/             # Biometric helper binaries (Touch ID/Windows Hello)
│   ├── ca/                    # CA management
│   ├── certificate/           # Certificate issuance/renewal/revocation
│   ├── db/                    # Database initialization
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
│   │   │   ├── DNSProviders.vue        # DNS providers (credentials/verification)
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
make lint-go           # Run golangci-lint
make lint-go-fix       # Auto-fix
```

Enabled linters: errcheck, govet, staticcheck, ineffassign, misspell, unconvert, nilerr, errorlint, bodyclose, contextcheck, noctx, gosec.

---

## Testing

```bash
make test              # Run all tests
```

13 packages with test coverage: auth, ca, certificate, db, dnsprovider, i18n, logging, monitor, network, notification, scanner, scheduler, settings.

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

1. **Ent code generation**: After modifying `ent/schema/`, run `make ent` to regenerate ORM code
2. **Binding generation**: After modifying Go services, run `make bindings` to regenerate frontend bindings
3. **Version injection**: `wails3 task build` does not accept a bare `-ldflags` flag; pass `VERSION=` etc. as Task variables, and each platform Taskfile assembles `-ldflags "-X main.currentVersion=..."`
4. **Linux cross-compilation**: Cannot cross-compile Linux from macOS (requires CGO + webkit2gtk), use GitHub Actions instead
5. **Naive UI is auto-imported**: Use `<n-xxx>` components directly in templates, no global registration needed
6. **Pinia state management**: Use `useXxxStore()` to get store, `storeToRefs()` for reactive properties
