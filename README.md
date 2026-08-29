# FlameGate

**FlameGate** is a self-hostable LLM proxy and router. It accepts requests in multiple client dialects (OpenAI, Anthropic, Gemini, plus embeddings, image, audio, search and web-fetch endpoints), applies token-saving transforms and content guardrails, routes each request to the best available provider account, and meters usage against budget limits.

You get one OpenAI-compatible entry point for all your models, with failover, caching, and a real-time dashboard — without locking your clients to a single vendor.

---

## Features

- **Multi-dialect gateway** — one `/v1` API that speaks OpenAI, Anthropic (`/v1/messages`), and Gemini (`/v1beta/models`) dialects, plus embeddings, image generation, speech/transcription, web search, and web fetch.
- **Smart routing & failover** — account selection with round-robin/fill-first strategies, automatic rate-limit cooldowns, and continuous background health probes that route around degraded accounts and models.
- **Token optimization** — context slimming and dynamic headroom controls cut upstream prompt tokens and bandwidth cost before requests leave the gateway.
- **Semantic response cache** — repeated or near-identical prompts are served from cache for zero upstream cost, with hash-based exact-match or optional embedding-based near-match modes.
- **Content guardrails** — in-flight inspection for PII, toxicity, prompt-injection, bias, and topic filtering. Native engines by default, with optional external engines (Presidio, OpenAI Moderation).
- **WASM extension architecture** — install new provider connectors as WebAssembly modules without rebuilding the binary. Extensions run in a sandboxed `wazero` runtime and hot-reload on file change. See [`flamegate-ext`](https://github.com/bobbyunknown/flamegate-ext).
- **Metering & budgeting** — buffered usage tracking, per-plan allocation, and hard spend limits per API key.
- **Per-key rate limiting** — RPM/TPM/concurrency quotas with an in-process memory backend.
- **Management dashboard** — built-in React dashboard (Vite, Tailwind, shadcn/ui) with real-time request metrics, key management, system health, and provider routing.
- **Zero-config tunnels** — built-in Cloudflare and Tailscale tunnel support for secure remote deployment.
- **Scalar API docs** — interactive OpenAPI documentation served by the admin API.

---

## Architecture

```mermaid
flowchart TD
    Client["LLM Clients<br/>OpenAI · Anthropic · Gemini<br/>embeddings · audio · images"] -->|"/v1 (proxy port, or shared :20180)"| Gateway

    subgraph Gateway["FlameGate :20180"]
        direction TB
        Auth["Auth & API key · Budget · Rate limit"]
        Process["Guardrails · Token slimming · Semantic cache"]
        Dispatch["Dispatch & cooldown · Metering"]
        Auth --> Process --> Dispatch
    end

    Gateway --> POpenAI["OpenAI provider"]
    Gateway --> PAnthropic["Anthropic provider"]
    Gateway --> PGemini["Gemini provider"]
    Gateway --> PExt["WASM extensions (wazero)"]
```

The admin dashboard, REST API, and (by default) the proxy API share port `20180`. Set `server.proxy_port` to expose the `/v1` proxy API on a dedicated listener (e.g. `20181`).

---

## Quick start

### Prerequisites

- **Go** 1.26+
- **Node.js** 18+ and `bun` (for building the frontend dashboard)

### 1. Build

```bash
git clone https://github.com/bobbyunknown/flamegate.git
cd flamegate

go build ./cmd/flamegate                    # backend binary → ./flamegate
cd frontend && bun install && bun run build # React dashboard bundle
```

`go build ./cmd/flamegate` produces the `./flamegate` binary in the repo root.

### 2. Bootstrap an API key

```bash
./flamegate bootstrap
```

This creates the initial database and prints an administrator API key (prefixed `fg_`) once.

### 3. Start the server

```bash
./flamegate start
```

The server listens on `http://localhost:20180` and opens the dashboard in your browser. `flamegate status` reports whether it is running.

> [!TIP]
> For development with hot reload: run `air` for the Go backend (see `.air.toml`) and `cd frontend && bun run dev` for the Vite dev server on `:5180`.

### 4. Send your first request

```bash
curl http://localhost:20180/v1/chat/completions \
  -H "Authorization: Bearer fg_your_api_key_here" \
  -H "Content-Type: application/json" \
  -d '{
    "model": "gpt-4o",
    "messages": [{"role": "user", "content": "Hello FlameGate!"}]
  }'
```

FlameGate validates your key and budget, runs active guardrails, slims the prompt, dispatches to the best active provider account, and records usage in real time.

---

## Configuration

Configuration is resolved in increasing order of precedence: **defaults → TOML file → environment variables**.

- The default config file is `~/.flamegate/flamegate.toml`. Override it with `-c <path>`/`--config`.
- Environment variables are prefixed `FLAMEGATE_` and use double underscores for nesting — `FLAMEGATE_SERVER__PORT=8080` sets `server.port`.

Reference config is available at [`flamegate.example.toml`](./flamegate.example.toml).

| Setting | Description | Default |
| --- | --- | --- |
| `server.host` / `server.port` | Admin API + dashboard listener | `127.0.0.1:20180` |
| `server.proxy_port` | Dedicated `/v1` proxy listener (`0` = share server port) | `0` |
| `database.driver` | `sqlite` or `postgres` | `sqlite` |
| `database.dsn` | Connection string; SQLite defaults to `<data_dir>/flamegate.db` | empty |
| `security.master_key` | Base64 32-byte key for credential encryption; auto-generated if empty | empty |
| `security.jwt_secret` | Signs dashboard session tokens; auto-generated if empty | empty |
| `security.bind_loopback_only` | Reject non-loopback access to dashboard/admin API | `true` |
| `log.level` / `log.format` | `debug` \| `info` \| `warn` \| `error`; `text` \| `json` | `info` / `text` |
| `meter.async` | Buffered/batched usage writes | `true` |
| `cache.enabled` | Semantic response cache | `false` |
| `limits.enabled` | Per-key rate limiter | `false` |
| `health.enabled` | Background account/model probes | `true` |

> [!NOTE]
> Data (database, master key, extensions) lives under `data.dir`, which defaults to `~/.flamegate` (`%APPDATA%/flamegate` on Windows).

---

## WASM extensions

Provider connectors that are not built in can be installed as WebAssembly modules without touching the core binary. Extensions run in FlameGate's sandboxed `wazero` runtime (no CGO), route network calls through host imports, and hot-reload when the `.wasm` file changes on disk.

```bash
./flamegate ext install ./path-to-extension   # dir containing schema.json + <slug>.wasm
./flamegate ext list
./flamegate ext enable  <slug>
./flamegate ext disable <slug>
./flamegate ext uninstall <slug>
```

Official extensions live in the [`flamegate-ext`](./flamegate-ext) monorepo. See [`flamegate-ext/README.md`](./flamegate-ext/README.md) to build new ones; any language targeting `wasm32` works.

---

## Project layout

```
flamegate/
├── cmd/flamegate/          # CLI entrypoint (start, status, bootstrap, ext, version)
├── internal/
│   ├── domain/             # Pure domain types, value objects, connectors (zero external deps)
│   ├── application/        # Use cases, DTOs, repository ports, read queries
│   └── infrastructure/     # HTTP API (Huma + Chi), GORM persistence, connectors,
│                           #   WASM engine, guardrails, dispatch, meter, transform
├── frontend/               # React dashboard (Vite, Tailwind, shadcn/ui)
├── flamegate-ext/          # WASM extension monorepo (template + provider modules)
├── skills/                 # Agent skills for the CLI
├── spec/                   # Design specs and ADRs
└── db/                     # GORM schemas & Atlas migrations
```

---

## Testing

```bash
go test ./...            # full test suite
go test -race ./...      # with race detector
go test -run TestFoo ./internal/infrastructure/http/   # single package
golangci-lint run        # lint suite
go vet ./...             # static analysis
```

---

## Related & Acknowledgements

- [`flamegate-ext`](https://github.com/bobbyunknown/flamegate-ext) — official WASM extension repository
- FlameGate was originally inspired by [KeiRouter](https://github.com/mydisha/keirouter). It has since been completely re-architected with an extension-first WASM plugin system, GORM persistence, Huma OpenAPI layer, and a modernized React dashboard.
