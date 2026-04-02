# CLAUDE.md

This file provides guidance to Claude Code (claude.ai/code) when working with code in this repository.

## Project Overview

Multi-Node Platform (MNP) — a manager/node distributed system for orchestrating Docker-based application groups behind proxy tunnels across remote nodes. Includes account management, remote browser instances (KasmVNC), Telegram bot, AI agent, and a Vue 3 web dashboard.

## Build & Run Commands

### Go Backend
```bash
# Build both binaries
CGO_ENABLED=0 go build -o manager ./cmd/manager
CGO_ENABLED=0 go build -o node ./cmd/node

# Run manager (requires PostgreSQL)
DB_HOST=localhost DB_USER=postgres DB_PASSWORD=password DB_NAME=platform_db ./manager

# Run node (connects to manager gRPC)
MANAGER_IP=localhost ./node
```

### Web Frontend (Vue 3 + Vite + Element Plus)
```bash
cd web
npm install
npm run dev      # dev server (proxies /api to localhost:8080)
npm run build    # production build
```

### Docker Compose (full stack)
```bash
cd deploy
docker compose up --build
```
Services: PostgreSQL (:5432), Manager REST API (:8002→8080), Manager gRPC (:50051), Web UI (:8001→80).

### Protobuf Regeneration
```bash
protoc --go_out=. --go-grpc_out=. api/proto/platform.proto
```

## Architecture

### Two Binaries, One Image
Both `cmd/manager` and `cmd/node` compile into a single Docker image (`deploy/Dockerfile.go`). The entrypoint command determines which binary runs.

### Manager (`cmd/manager/`)
- **main.go** — boots DB, starts 7 background workers + TG Bot, launches gRPC (:50051) and Gin HTTP (:8080).
- **http_server.go** — REST API router + handlers for nodes, proxies, groups. Basic Auth. Proxy allocation uses `SELECT ... FOR UPDATE` inside transactions.
- **http_accounts.go** — AppAccount CRUD with AES-encrypted credentials.
- **http_browsers.go** — BrowserInstance CRUD, VNC proxy info, browser automation actions.
- **http_config.go** — SystemConfig KV CRUD.
- **http_ai.go** — AI chat endpoint, delegates to `pkg/ai`.
- **grpc_server.go** — implements `NodeService`. Register captures node IP from gRPC peer. ReportTaskResult updates GroupRuntime/BrowserInstance status based on task type and result.
- **template_resolver.go** — resolves `AppTemplate.CommandTemplate` with `{{key}}` substitution. V2 version supports AccountBindings (decrypts credentials from AppAccount).

### Node (`cmd/node/`)
- **main.go** — registers with manager, heartbeats every 10s. Handles task types: `start_group`, `stop_group`, `restart_group`, `replace_proxy`, `start_browser`, `stop_browser`, `browser_action`. Supports tunnel types: tun2socks and tun2proxy.

### Background Workers (`pkg/workers/`)
| Worker | Interval | Purpose |
|--------|----------|---------|
| `DriftChecker` | 30s | Marks nodes offline (configurable threshold); releases orphaned proxy leases |
| `ProxyLifecycleChecker` | 60s | TCP-dials observer-pool proxies; promotes to formal on success |
| `RuleEngine` | 2min | Evaluates govaluate expressions against proxy/group metrics |
| `DataCleaner` | 6h | Deletes old operations/tasks (configurable retention) |
| `TaskTimeoutChecker` | 1min | Marks timed-out tasks as failed; updates GroupRuntime/BrowserInstance |
| `BrowserChecker` | 30s | TCP-dials VNC ports of running browsers |
| `AccountStatusChecker` | 10min | Framework for CDP-based account status checking |

### Data Model (`pkg/models/`)
Core entities: `Node`, `ProxyResource`, `ProxyLease`, `GroupSpec`/`GroupRuntime`, `AppTemplate`, `AppAccount` (encrypted credentials), `AccountGroupBinding`, `BrowserInstance`, `Rule`, `Operation`→`Task`, `SystemConfig`, `AIActionLog`. GORM + PostgreSQL + AutoMigrate. Config and app templates seeded on startup.

### Key Packages
- **`pkg/utils/crypto.go`** — AES-256-GCM encrypt/decrypt for credentials. Key from `ENCRYPTION_KEY` env var.
- **`pkg/ai/`** — AI Agent with OpenAI-compatible tool-calling loop. Tools map to DB queries and system operations.
- **`pkg/tgbot/`** — Telegram Bot (long-polling). Commands: /status, /nodes, /groups, /accounts, /browsers, /ai. Admin ID whitelist from SystemConfig.

### Proxy Tunnel Model
Groups use **tun2socks** or **tun2proxy** containers for transparent proxying. App and browser containers attach via `--network container:<tunnel>`.

### Browser Model
Uses KasmVNC-based images (default `kasmweb/chromium:1.16.1`). Browsers share a group's tunnel network. VNC/CDP ports are dynamically mapped and reported back via ReportTaskResult.

### Web Frontend (`web/`)
Vue 3 SPA with Element Plus. Pages: Dashboard, Nodes, Proxies, Deploy Wizard, Groups, App Templates, Accounts, Browsers (with VNC iframe viewer), Rules, System Config, AI Chat, Operations.

## Key Environment Variables

| Variable | Used By | Default |
|----------|---------|---------|
| `DB_HOST`, `DB_USER`, `DB_PASSWORD`, `DB_NAME`, `DB_PORT` | Manager | localhost, postgres, password, platform_db, 5432 |
| `ADMIN_USER`, `ADMIN_PASSWORD` | Manager HTTP | admin, admin |
| `ENCRYPTION_KEY` | Manager (crypto) | mnp-default-key-change-in-prod!! |
| `MANAGER_IP` | Node | localhost |

Additional config (Telegram token, AI API key, browser image, etc.) is managed via `SystemConfig` DB table and the `/config` UI page.

## Conventions

- Go module: `multi_node_platform`
- REST responses: `{"data": ...}` or `{"message": ..., "error": ...}`
- Task flow: HTTP creates `Operation` + `Task(pending)` → Node picks up via heartbeat → Node reports via `ReportTaskResult` → Manager updates GroupRuntime/BrowserInstance
- Credentials: encrypted at rest (AES-GCM), masked in list APIs, full values only in detail APIs
- Comments and logs are in Chinese (中文)
