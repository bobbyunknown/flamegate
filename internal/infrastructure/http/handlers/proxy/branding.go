package proxy

import (
	"context"
	"encoding/json"
	"net/http"
)

// brandingSettingsKey is the settings-store key for white-label branding.
const brandingSettingsKey = "branding_settings"

// BrandingSettings holds configurable branding for the dashboard and portal.
// When empty, defaults to FlameGate branding.
type BrandingSettings struct {
	Name         string `json:"name"`          // Display name (e.g. "FlameGate", "Acme AI Gateway")
	LogoURL      string `json:"logo_url"`      // URL to logo image (SVG/PNG). Empty = default logo.
	FaviconURL   string `json:"favicon_url"`   // URL to favicon (PNG/ICO). Empty = default favicon.
	Tagline      string `json:"tagline"`       // Optional short tagline shown on portal login.
	ColorPalette string `json:"color_palette"` // Color palette identifier (e.g. "sage-terra", "ocean", "midnight").
}

func defaultBrandingSettings() BrandingSettings {
	return BrandingSettings{
		Name:         "FlameGate",
		LogoURL:      "",
		FaviconURL:   "",
		Tagline:      "",
		ColorPalette: "sage-terra",
	}
}

// loadBrandingSettings reads the persisted branding, falling back to defaults
// when unset. Never errors.
func (s *Handler) loadBrandingSettings(ctx context.Context) BrandingSettings {
	def := defaultBrandingSettings()
	if s.settings == nil {
		return def
	}
	raw, err := s.settings.Get(ctx, brandingSettingsKey)
	if err != nil || raw == "" {
		return def
	}
	var bs BrandingSettings
	if err := json.Unmarshal([]byte(raw), &bs); err != nil {
		return def
	}
	// Backfill defaults for empty fields.
	if bs.Name == "" {
		bs.Name = def.Name
	}
	return bs
}

// ---- public portal endpoint -------------------------------------------------

// portalBranding returns branding config for the public portal (no auth).
// This allows the portal to display the custom name and logo without requiring
// a dashboard session.
func (s *Handler) PortalBranding(w http.ResponseWriter, r *http.Request) {
	bs := s.loadBrandingSettings(r.Context())
	writeJSON(w, http.StatusOK, bs)
}
