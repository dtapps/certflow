# CertFlow

English | [中文](README.md)

SSL certificate management tool with support for certificate issuance, renewal, and expiration monitoring.

Built with Go + Vue 3 cross-platform desktop application architecture, powered by Wails v3.

## Features

- **Certificate Management** — Issue, renew, and revoke SSL certificates with 30+ DNS provider auto-verification
- **Domain Monitoring** — HTTPS/HTTP health checks and certificate expiration alerts
- **Auto Renewal** — Scheduled tasks to automatically renew expiring certificates
- **Multi-Platform** — Supports macOS, Windows, and Linux

## Quick Start

```bash
# Install dependencies
make deps

# Start development mode
make dev

# Build production package (with version)
make build VERSION=1.0.0

# Package macOS application
make package VERSION=1.0.0
```

## Development Commands

```bash
make help              # View all commands
make lint              # Go code linting
make lint-fix          # Go code linting (auto-fix)
make check             # Frontend TypeScript type checking
make test              # Go backend tests
make bindings          # Generate Wails TypeScript bindings
make ent               # Generate Ent ORM code
make format            # Format all code (Go + Vue/TS)
make format-go         # Format Go code
make format-frontend   # Format frontend code
```

## Tech Stack

- **Backend**: Go 1.27 + Wails v3 + Ent ORM + lego v5 (ACME) + SQLite
- **Frontend**: Vue 3 + TypeScript + Vite 8 + Tailwind CSS v4 + Naive UI v2 + Pinia

## Repositories

| Platform | URL |
|----------|-----|
| CNB | https://cnb.cool/dtapp/certflow |
| GitHub | https://github.com/dtapps/certflow |
| Gitea | https://gitea.com/dtapps/certflow |
| GitLab | https://gitlab.com/dtapps/certflow |
| Gitee | https://gitee.com/dtapps/certflow |
| GitCode | https://gitcode.com/dtapp/certflow |

## Release

Trigger the manual workflow on any platform with a version number. Build artifacts are downloaded from GitHub Actions.

## Development Documentation

See [DEVELOPMENT_EN.md](DEVELOPMENT_EN.md)
