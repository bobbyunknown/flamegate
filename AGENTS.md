# FlameGate — Agent Knowledge Base

**Generated:**2026-07-04
**Module:** `github.com/bobbyunknown/flamegate`
**Stack:** Go1.26 + React/Vite frontend + SQLite/Postgres

## OVERVIEW

FlameGate is an LLM proxy/router. It accepts requests in multiple client dialects (OpenAI, Anthropic, Gemini), applies token-saving transforms, routes to the best available provider account, and meters usage.

## STRUCTURE

```
flamegate/
├── cmd/flamegate/           # Entrypoint, CLI (start/bootstrap/status/version)
├── internal/
│   ├── domain/              # Pure types — ZERO external deps
│   │   ├── shared/          # Value objects, errors, enums, message, request/response
│   │   ├── provider/        # Connector interfaces, credentials, media types
│   │   ├── key/             # APIKey aggregate entity
│   │   ├── account/         # Account aggregate entity
│   │   ├── plan/            # Plan aggregate entity
│   │   ├── budget/          # Budget aggregate entity
│   │   ├── guardrail/       # Guardrail policy entity
│   │   ├── usage/           # Usage entry/snapshot types
│   │   ├── routing/         # Target, Attempt, Chain types
│   │   └── core.go          # Compatibility shim (re-exports for callers)
│   ├── application/         # Orchestration layer
│   │   ├── ports/           # Repository & service interfaces (17 repos + services)
│   │   ├── dto/             # Data Transfer Objects (request/response types)
│   │   ├── query/           # Read-only query services (models, usage, system)
│   │   └── usecases/        # Per-aggregate use cases (key, account, chat, plan, etc.)
│   ├── infrastructure/      # Adapters
│   │   ├── http/            # Huma + Chi HTTP layer
│   │   │   ├── handlers/    # HTTP handler methods (chat, accounts, guardrails, etc.)
│   │   │   ├── middleware/  # Auth, CORS, rate limit, panic recovery
│   │   │   ├── openapi/     # Huma OpenAPI config + security schemes
│   │   │   └── router/      # Route registration (wires handlers + middleware)
│   │   ├── persistence/     # GORM repos + schema models (24 tables) + UnitOfWork
│   │   ├── cache/           # Memory cache, Redis, embedder
│   │   ├── connectors/      # LLM provider adapters
│   │   │   └── qoder/       # Qoder connector
│   │   ├── guardrails/      # Content safety (PII, toxicity, injection, bias, topics)
│   │   ├── transform/       # Dialect translation (OpenAI ↔ Anthropic ↔ Gemini)
│   │   ├── tunnel/          # Cloudflare & Tailscale tunnels
│   │   ├── capability/      # Provider capability profiles
│   │   ├── proxy/           # Outbound proxy resolution
│   │   ├── pipeline/        # Request orchestration (validate → guardrails → route → proxy → meter)
│   │   ├── dispatch/        # Account selection, fallback, cooldown
│   │   ├── auth/            # Dashboard auth, JWT session management
│   │   ├── identity/        # API key creation & rotation
│   │   ├── meter/           # Usage metering & async batching
│   │   ├── budget/          # Budget engine (spend limits)
│   │   ├── oauth/           # OAuth2 flows for provider accounts
│   │   ├── healthcheck/     # Background account health probes
│   │   └── update/          # Version update checker
│   ├── shared/              # Cross-cutting utilities (crypto, vault, observ, httputil, etc.)
│   │   └── usagehub/        # Real-time usage hub
│   ├── cli/                 # CLI tool integrations (clitools)
│   ├── config/              # Config loading (TOML + env vars, prefixed FLAMEGATE_)
│   └── app/                 # Bootstrap wiring (Build, Run)
├── frontend/                # React dashboard (Vite, Tailwind, shadcn)
├── skills/                  # OpenCode skills (flamegate, flamegate-chat, etc.)
├── deploy/                  # Dockerfile
├── scripts/                 # install.sh, quickstart.sh, hooks
├── Formula/                 # Homebrew formula
└── .github/workflows/       # CI (ci.yml, release.yml)
```

### Dependency Rule

```
infrastructure/ → application/ → domain/ (NEVER reverse)
shared/ importable by all layers
config/ importable by all layers
```

## WHERE TO LOOK

| Task | Start Here | Notes |
|---|---|---|
| Add new LLM provider | `internal/infrastructure/connectors/` | Implement `domain.Connector` interface, register in `registry.go` |
| Add new endpoint | `internal/infrastructure/http/handlers/` | Add handler method, register in `router/router.go` |
| Change routing logic | `internal/infrastructure/pipeline/` + `internal/dispatch/` | Pipeline orchestrates, dispatch selects account |
| Add guardrail detector | `internal/infrastructure/guardrails/` | Implement `Detector` interface, wire in `app.go` |
| Change config | `internal/config/config.go` | Struct + `koanf` loading. Env prefix: `FLAMEGATE_` |
| Add token-saving feature | `internal/shared/slimmer/` or `internal/shared/headroom/` | Pipeline calls these before upstream |
| Change auth flow | `internal/infrastructure/auth/` + `internal/infrastructure/http/auth_handlers.go` | Session cookie: `fg_session` |
| Add CLI tool integration | `internal/cli/clitools/` | Register in registry, implement auto-config |
| Database migration | `internal/infrastructure/persistence/` | GORM schema models + Atlas migration (`atlas.hcl`) |
| Prometheus metrics | `internal/shared/observ/metrics.go` | Prefix: `flamegate_` |
| Frontend change | `frontend/src/` | React + Tailwind + shadcn/ui |

## GO CODING RULES

These are project-specific rules, not generic advice. Violations will be rejected.

### Naming

- **Package names:** short, lowercase, no underscores. `store`, `cache`, `pipeline` — NOT `dataStore`, `cache_manager`
- **Interfaces:** prefer -er suffix for single-method: `Reader`, `Flusher`. Multi-method: domain noun `Connector`, `Store`
- **No `Manager`/`Helper`/`Util`/`Service`** suffixes unless the domain literally uses that term
- **Exported names:** only when needed outside the package. Unexport aggressively
- **Error variables:** `ErrNotFound`, `ErrUnauthorized` (sentinel). `ValidationError` (type with fields)
- **Constants:** `CamelCase` for exported, `camelCase` for unexported. Group with `const ( ... )`

### Functions

- **Do one thing.** `validateAndSave` → split into `validate` + `save`
- **≤4 parameters.** Beyond that → options struct or group into a request struct
- **Parameter order:** `context.Context` first, then inputs, then output destinations
- **Return order:** result, error. Always
- **No naked returns** except in 1-3 line functions where the return is obvious
- **No side effects** — functions should not secretly modify global state
- **Constructor naming:** `NewFoo(params)` returns `*Foo`. Group options with `WithFoo(val)` functional options when >3 params

### Error Handling

- **NEVER discard errors.** `_, _ = fn()` is forbidden. Handle or document why safe
- **Wrap with `%w`:** `fmt.Errorf("load config %s: %w", path, err)` — lowercase, no punctuation, add context
- **Single handling rule:** error is EITHER logged OR returned, NEVER both
- **`errors.Is`/`errors.As`** for checking, never `err.Error() == "string"`
- **No panic for normal errors.** Panic only for programmer bugs or impossible states
- **Sentinel errors** for expected conditions: `var ErrNotFound = errors.New("not found")`
- **Custom error types** when callers need to branch on fields: `type ValidationError struct { Field string }`
- **`logrus` for logging,** not `fmt.Println` or `log.Printf`. Structured, with fields

### Types & Structs

- **Make zero value useful:** `var buf bytes.Buffer` should work without init
- **Composite literals with field names:** `&Foo{ID: 1, Name: "x"}`, never positional
- **Embedding for composition:** `type Server struct { *Logger; addr string }`
- **Pointer receivers** for methods that mutate. Value receivers for read-only on small types
- **Be consistent** within a type — don't mix value and pointer receivers

### Interfaces

- **Small:** 1-3 methods. Compose larger interfaces from smaller ones
- **Define at consumer:** `internal/infrastructure/pipeline/` defines what it needs from `Store`, not the other way around
- **Accept interfaces, return structs:** `func Process(r io.Reader) (*Result, error)`
- **Don't preemptively abstract** — extract interface when there are 2+ implementations

### Control Flow

- **Early return:** handle errors first, happy path at minimal indentation
- **No unnecessary `else`:** drop `else` when `if` ends with `return`
- **`switch` over `if-else` chains** when comparing the same variable
- **Named booleans** for complex conditions: `isAdmin := user.Role == RoleAdmin`
- **Reduce nesting:** guard clauses, early returns

### Slices & Maps

- **Always initialize:** `users := []User{}`, `m := map[string]int{}`. Nil maps panic; nil slices serialize to `null`
- **Preallocate when size known:** `make([]T, 0, len(items))`
- **Don't preallocate speculatively** — `make([]T, 0, 1000)` wastes memory when common case is 10

### Concurrency

- **`context.Context` as first parameter** — never in a struct
- **Buffered channels** when sender shouldn't block: `ch := make(chan T, 1)`
- **`sync.WaitGroup`** for simple fan-out. `errgroup` for coordinated goroutines with error propagation
- **No goroutine leaks:** every goroutine must have a cancellation path via `ctx.Done()`
- **`sync.Pool`** for frequent allocations in hot paths

### Performance

- **`strings.Builder`** in loops, not `+` concatenation
- **`strconv`** for simple conversions, `fmt.Sprintf` for complex formatting
- **`sync.Pool`** for hot-path allocations (buffers, etc.)
- **Avoid `reflect`** unless strictly necessary
- **"A little copying is better than a little dependency"** — minimize external deps

### Code Organization

- **File order:** package doc → imports → constants → types → constructors → methods → helpers
- **One primary type per file** when it has significant methods
- **Blank imports** (`_`) only in `main` and test packages
- **No dot imports** in library code
- **Lines ≤120 chars.** Break at semantic boundaries, not column counts
- **4+ function args → one per line**

## ERROR MESSAGES

```
# Format: lowercase, no punctuation, context prefix
fmt.Errorf("load config %s: %w", path, err)         # GOOD
fmt.Errorf("Load config: %w", err)                    # BAD: capital
fmt.Errorf("load config: %w.", err)                    # BAD: punctuation
fmt.Errorf("error loading config: %w", err)            # BAD: redundant "error"
```

## TESTING

- **Table-driven tests** with `t.Run` subtests
- **`testify/assert`** and **`testify/require`** for assertions
- **SQLite in-memory** (`:memory:`) for database tests
- **`net/http/httptest`** for HTTP handler tests
- **Test file location:** same package as code, `_test.go` suffix
- **Test naming:** `TestFunctionName_Scenario` — e.g. `TestGenerateAPIKey_InvalidInput`
- **`t.Parallel()`** when tests don't share mutable state
- **Mock interfaces** from `internal/application/ports/` or define locally in test files
- **No test suites** unless testing complex stateful setups (use `testify/suite` sparingly)

## ANTI-PATTERNS (FORBIDDEN)

| Pattern | Why | Do Instead |
|---|---|---|
| `_, _ = fn()` | Swallowed error | Handle or `//nolint:errcheck` with reason |
| `panic(err)` for IO/network | Crashes server | `return ..., fmt.Errorf("...: %w", err)` |
| `log.Printf` in library code | Unstructured | `log.WithField("key", val).Error("msg")` |
| `err.Error() == "..."` | Fragile string match | `errors.Is(err, ErrFoo)` |
| Global `var db *sql.DB` | Untestable | Dependency injection via struct |
| `interface{}` / `any` everywhere | Loses type safety | Use generics or concrete types |
| Comment explaining bad code | Hides tech debt | Rewrite the code |
| `time.Sleep` in tests | Flaky | Use channels, `sync.WaitGroup`, or polling |
| Positional struct literals | Breaks on field reorder | Always use field names |
| Context in struct field | Breaks context semantics | First function parameter |
| `init()` for side effects | Hidden coupling | Explicit initialization in `app.go` |

## COMMANDS

```bash
# Development
make dev                    # Backend (:20180) + Frontend (:5180) hot reload
air                         # Go hot reload only (needs .air.toml)

# Build
make build                  # Backend binary + frontend assets
go build ./cmd/flamegate    # Backend only

# Test
make test                   # All tests
go test ./...               # Same, via go
go test -race ./...         # Race detector
go test -run TestFoo ./internal/infrastructure/http/ # Single package

# Lint
golangci-lint run           # Full lint suite (.golangci.yml)
go vet ./...                # Quick static analysis
go vet ./... ./cmd/...      # Include cmd

# Database
go run ./cmd/flamegate bootstrap    # Create initial API key

# Docker
docker build -f deploy/Dockerfile -t flamegate:latest .

# Frontend
cd frontend && npm run dev          # Dev server :5180
cd frontend && npm run build        # Production build
```

## NOTES

- **Module path:** `github.com/bobbyunknown/flamegate` (was `mydisha/keirouter/backend`)
- **Env prefix:** `FLAMEGATE_` (was `KEIROUTER_`). Example: `FLAMEGATE_SERVER__PORT=20180`
- **API key prefix: `fg_`
- **Session cookie:** `fg_session`
- **Prometheus prefix:** `flamegate_` (was `keirouter_`)
- **Config path:** `~/.flamegate/`
- **Database default:** `~/.flamegate/flamegate.db`
- **`internal/domain/` has ZERO external deps** — compiler enforces this via `internal/` visibility
- **Clean Architecture**: `infrastructure/` → `application/` → `domain/` (never reverse)
- **GORM persistence**: `internal/infrastructure/persistence/` — 17 repos,24 schema models, `schema/` type hub
- **`internal/infrastructure/http/`** — handlers/ (61 files), middleware/, openapi/, router/
- **Logging**: `logrus` (slog residual being cleaned up in prettylog)
- **Auth**: JWT via `golang-jwt/jwt/v5` (migrated from HMAC SHA-256)
