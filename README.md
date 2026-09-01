# FlameGate

**FlameGate** is a self-hostable LLM proxy, intelligent router, and WebAssembly extension runtime. It accepts requests in multiple client dialects (OpenAI, Anthropic, Gemini, plus embeddings, images, audio, web search, and web fetch), applies token-saving transforms and content guardrails, routes each request to the best available provider account, and meters usage against strict budget limits.

You get a single, robust API gateway for all your AI workloads with automatic failover, semantic caching, and a real-time dashboard — without locking your tools or code to a single vendor.

---

## Features

- **Multi-dialect gateway** — One unified `/v1` API that speaks OpenAI, Anthropic (`/v1/messages`), and Gemini (`/v1beta/models`) dialects, plus embeddings, image generation, speech/transcription, web search, and web fetch.
- **Smart routing & failover** — Account selection with round-robin or fill-first strategies, automatic rate-limit cooldowns, and continuous background health probes that route around degraded accounts and models.
- **Token optimization** — Context slimming and dynamic headroom cut prompt tokens and payload size before requests leave the gateway.
- **Semantic response cache** — Repeated or near-identical prompts are served from memory/cache for zero upstream cost, supporting exact hash and embedding-based similarity modes.
- **Content guardrails** — In-flight inspection for PII, toxicity, prompt-injection, bias, and banned topics with native and pluggable detectors.
- **WASM extension runtime** — Install and update provider connectors as WebAssembly modules (`wazero` runtime) without rebuilding the core binary. Extensions hot-reload on file change.
- **Metering & budgeting** — High-throughput buffered usage tracking, per-plan token allocation, and hard spend limits per API key.
- **Per-key rate limiting** — In-memory RPM/TPM and concurrency quotas per key or tier.
- **Built-in dashboard** — React dashboard (Vite, Tailwind, shadcn/ui) with real-time request metrics, key management, system health, and routing policies.
- **Zero-config tunnels** — Built-in Cloudflare and Tailscale tunnel integration for secure remote deployment.
- **Interactive API docs** — Embedded Scalar OpenAPI documentation served by the admin API.

---

## Architecture

```mermaid
flowchart TD
    Client["LLM Clients<br/>OpenAI · Anthropic · Gemini<br/>embeddings · audio · images"] -->|"/v1 (shared :20180 or proxy_port)"| Gateway

    subgraph Gateway["FlameGate :20180"]
        direction TB
        Auth["Auth & API Key · Budget · Rate Limit"]
        Process["Guardrails · Token Slimming · Semantic Cache"]
        Dispatch["Dispatch Strategy · Cooldown · Metering"]
        Auth --> Process --> Dispatch
    end

    Gateway --> POpenAI["OpenAI Provider"]
    Gateway --> PAnthropic["Anthropic Provider"]
    Gateway --> PGemini["Gemini Provider"]
    Gateway --> PExt["WASM Extensions (wazero)"]
```

The admin dashboard, REST API, and proxy endpoints share port `20180` by default. You can set `server.proxy_port` to expose the `/v1` proxy API on a dedicated listener (e.g. `20181`).

---

## Installation & Deployment

### Option A: Shell Installer (macOS & Linux)

Install the pre-compiled binary directly to `~/.local/bin` with one command:

```bash
curl -fsSL https://raw.githubusercontent.com/bobbyunknown/flamegate/main/scripts/install.sh | bash
```

---

### Option B: NPM / NPX

Run instantly with `npx` / `bunx` or install globally using your favorite Node package manager:

```bash
# Run directly without global installation
npx flamegate

# Or install globally
npm install -g flamegate
# or: bun install -g flamegate / pnpm add -g flamegate
```

---

### Option C: Docker & Docker Compose

Official multi-architecture Docker images (`linux/amd64`, `linux/arm64`) are published to **Docker Hub** and **GitHub Container Registry (GHCR)**:

- `bobbyunknown/flamegate:latest`
- `ghcr.io/bobbyunknown/flamegate:latest`

#### 1. Quick Start with Docker Compose

A ready-to-use [`docker-compose.yml`](./docker-compose.yml) is included:

```bash
# Clone the repository
git clone https://github.com/bobbyunknown/flamegate.git
cd flamegate

# Start FlameGate in the background
docker compose up -d

# Generate your initial administrator API key
docker compose exec flamegate flamegate bootstrap
```

Access the dashboard at `http://localhost:20180`.

#### 2. Running with Docker CLI

```bash
# Pull the latest image
docker pull bobbyunknown/flamegate:latest

# Run the container (mapping Dashboard :20180 and Proxy :20181)
docker run -d \
  --name flamegate \
  --restart unless-stopped \
  -p 20180:20180 \
  -p 20181:20181 \
  -v flamegate_data:/root/.flamegate \
  bobbyunknown/flamegate:latest

# Bootstrap initial API key
docker exec -it flamegate flamegate bootstrap
```

---

### Option D: Pre-built Binaries

Download standalone, zero-dependency native binaries from [GitHub Releases](https://github.com/bobbyunknown/flamegate/releases):

- **Linux**: `flamegate-linux-amd64`, `flamegate-linux-arm64`
- **macOS**: `flamegate-darwin-arm64` (Apple Silicon), `flamegate-darwin-amd64` (Intel)
- **Windows**: `flamegate-windows-amd64.exe`, `flamegate-windows-arm64.exe`

```bash
# Example for macOS (Apple Silicon)
chmod +x flamegate-darwin-arm64
mv flamegate-darwin-arm64 flamegate

# Bootstrap key and run
./flamegate bootstrap
./flamegate
```

---

### Option E: Build from Source

**Prerequisites:**
- **Go** 1.26+
- **Bun** (or Node.js 18+)

```bash
git clone https://github.com/bobbyunknown/flamegate.git
cd flamegate

# Build React UI assets and compile standalone binary
make build-ui

# Bootstrap initial admin key
./bin/flamegate bootstrap

# Run the server
./bin/flamegate
```

---

## Quick Usage

### 1. Access Dashboard

Open `http://localhost:20180` in your browser. Log in using the admin API key generated from `./flamegate bootstrap` (or `docker compose exec flamegate flamegate bootstrap`).

### 2. Send a Request

Use any OpenAI-compatible client, SDK, or `curl` pointing to the LLM proxy port (`20181`):

```bash
curl http://localhost:20181/v1/chat/completions \
  -H "Authorization: Bearer fg_your_api_key_here" \
  -H "Content-Type: application/json" \
  -d '{
    "model": "gpt-4o",
    "messages": [
      {"role": "user", "content": "Hello FlameGate!"}
    ]
  }'
```

---

## CLI Reference

```
Usage:
  flamegate [flags]
  flamegate [command]

Commands:
  status                   Check whether local server is running and print its URL
  bootstrap                Create an initial API key and print it once
  ext                      Manage WASM extensions (install, list, enable, disable, uninstall)
  version                  Print version and commit metadata
  help                     Show help message

Flags:
  -c, --config <path>      Path to a TOML config file (default: ~/.flamegate/flamegate.toml)
  -k, --key-name <name>    (bootstrap) Name for the created API key (default: default)
  -bootstrap               Create an initial API key and exit
  -healthcheck             Perform local health check and exit with status code
```

---

## WASM Extensions

FlameGate utilizes a WebAssembly runtime (`wazero`) to support custom providers and dialect translators without modifying or recompiling the core application.

```bash
# Install an extension from a local folder (must contain schema.json + <slug>.wasm)
./flamegate ext install ./path-to-extension

# View installed extensions
./flamegate ext list

# Enable or disable extensions dynamically
./flamegate ext enable  <slug>
./flamegate ext disable <slug>

# Remove an extension
./flamegate ext uninstall <slug>
```

Official extensions and developer guides are maintained in the [`flamegate-ext`](./flamegate-ext) directory.

---

## Configuration

Configuration is loaded from **defaults → TOML file → environment variables**.

- Default file location: `~/.flamegate/flamegate.toml` (or `%APPDATA%\flamegate\flamegate.toml` on Windows).
- Environment variables use the `FLAMEGATE_` prefix with double underscores for nested keys (e.g. `FLAMEGATE_SERVER__PORT=8080`).

Reference configuration template: [`flamegate.example.toml`](./flamegate.example.toml).

| Setting | Description | Default |
| --- | --- | --- |
| `server.host` / `server.port` | Admin API + dashboard listener | `127.0.0.1:20180` |
| `server.proxy_port` | Dedicated `/v1` proxy listener (`0` = share server port) | `0` |
| `database.driver` | `sqlite` or `postgres` | `sqlite` |
| `database.dsn` | Connection string (SQLite defaults to `<data_dir>/flamegate.db`) | empty |
| `security.master_key` | Base64 32-byte key for credential encryption; generated if empty | empty |
| `security.jwt_secret` | Signs dashboard session tokens; generated if empty | empty |
| `security.bind_loopback_only` | Restrict dashboard and admin API to loopback interfaces | `true` |
| `log.level` / `log.format` | `debug` \| `info` \| `warn` \| `error`; `text` \| `json` | `info` / `text` |
| `meter.async` | Asynchronous buffered usage writes | `true` |
| `cache.enabled` | Semantic and exact response cache | `false` |
| `limits.enabled` | Per-key rate limiting engine | `false` |
| `health.enabled` | Background health probe service | `true` |

---

## Development & Testing

```bash
# Run backend with hot reload
make dev

# Run frontend dev server with Vite (:5180)
make ui-dev

# Run all unit and integration tests
make test

# Run tests with race detector
make test-race

# Run linter
make lint
```

---

## Acknowledgements

- Built with [Wazero](https://wazero.io/) for pure Go WebAssembly execution.
- Inspired by [KeiRouter](https://github.com/mydisha/keirouter). FlameGate features a redesigned microkernel extension architecture, GORM persistence, Huma typed OpenAPI handlers, and a modernized React dashboard.
