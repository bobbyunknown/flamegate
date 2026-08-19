package admin

import (
	"github.com/go-chi/chi/v5"

	"github.com/bobbyunknown/flamegate/internal/infrastructure/persistence/schema"
)

// mountAdmin registers the dashboard admin endpoints on the given router. These
// manage API keys, provider accounts, routing chains, budgets, and usage.
func (s *Handler) MountAdmin(r chi.Router) {
	// Streaming endpoints (SSE) stay as raw Chi
	r.Get("/usage/stream", s.adminUsageStream)
	r.Get("/console", s.adminConsoleLog)
	r.Delete("/console", s.adminConsoleClear)
	r.Get("/console/stream", s.adminConsoleStream)

	// Settings endpoints (not yet migrated)
	r.Get("/settings/database", s.adminExportDatabase)
	r.Post("/settings/database", s.adminImportDatabase)
	r.Get("/settings/sqlite", s.adminSQLiteStatus)
	r.Get("/settings/sqlite/backup", s.adminSQLiteBackup)
	r.Post("/settings/sqlite/restore", s.adminSQLiteRestore)
	r.Post("/settings/proxy-test", s.adminTestProxy)

	// Tunnel SSE endpoint (must stay as raw Chi)
	r.Post("/tunnel/tailscale-install", s.adminTailscaleInstall)

	// Custom provider flows, extensions, CLI tools (raw Chi submounts)
	s.MountCustomProviders(r)
	s.MountExtensions(r)
	s.MountStore(r)
	s.MountCLITools(r)

	// Branding endpoints (not yet migrated)
	r.Get("/settings/branding", s.adminGetBranding)
	r.Post("/settings/branding", s.adminUpdateBranding)

	// Guardrail SSE endpoint (must stay as raw Chi)
	r.Get("/guardrails/logs/stream", s.adminGuardrailLogStream)
}

const adminTenant = schema.DefaultTenantID
