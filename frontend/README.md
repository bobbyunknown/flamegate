# FlameGate Dashboard

The web control surface for FlameGate, built with React 19, Vite, TypeScript,
and Tailwind CSS 4.

## Development

```bash
npm install
npm run dev      # starts on http://localhost:5180, proxies /api and /v1 to :20180
```

Run the Go backend separately (`cd ../backend && go run ./cmd/flamegate`) so the
proxied API calls resolve.

## Build

```bash
npm run build    # type-checks then emits static assets to dist/
```

In production, the backend serves these assets from a standard install path
(`/usr/local/share/flamegate/frontend/dist` by default) or from the Docker image.

## Design

The UI uses a calm, warm-neutral palette (zinc) with a single muted sage/teal
accent — deliberately avoiding the violet/fuchsia gradients common to generic AI
dashboards. Tokens live in `src/index.css`; primitives in `src/components/ui.tsx`.

## Structure

```
src/
  main.tsx           entry: providers + router
  App.tsx            routes
  lib/api.ts         typed admin API client
  components/        Layout + reusable UI primitives
  pages/             Overview, Providers, Accounts, Chains, Keys, Budgets
