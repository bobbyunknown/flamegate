import { BASE_URL } from "./client";
import type { BrandingSettings, KeyUsageData } from "./types";

/**
 * Fetch branding settings for the public portal (no auth required)
 */
export async function fetchPortalBranding(): Promise<BrandingSettings> {
  const resp = await fetch(`${BASE_URL}/v1/portal/branding`);
  if (!resp.ok) {
    return { name: "FlameGate", logo_url: "", favicon_url: "", tagline: "", color_palette: "sage-terra" };
  }
  return resp.json();
}

/**
 * Fetch usage stats for an API Key, authenticated via the key itself (public portal)
 */
export async function fetchKeyUsage(key: string): Promise<KeyUsageData> {
  const resp = await fetch(`${BASE_URL}/v1/keys/me/usage`, {
    headers: { Authorization: `Bearer ${key}` },
  });
  if (!resp.ok) {
    const data = await resp.json().catch(() => ({}));
    throw new Error(data.error || "Invalid key or server error");
  }
  return resp.json();
}

/**
 * Fetch usage stats for an API Key using its database ID (public portal link sharing)
 */
export async function fetchKeyUsageById(id: string): Promise<KeyUsageData> {
  const resp = await fetch(`${BASE_URL}/v1/portal/keys/${id}/usage`);
  if (!resp.ok) {
    const data = await resp.json().catch(() => ({}));
    throw new Error(data.error || "Invalid key ID or server error");
  }
  return resp.json();
}
