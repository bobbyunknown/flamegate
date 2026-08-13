import { request } from "./client";
import type { EndpointSettings, HeadroomTestResult, AccessSettings, BrandingSettings } from "./types";

export const endpointSettings = () => request<EndpointSettings>("GET", "/settings/endpoint");

export const updateEndpointSettings = (patch: Partial<EndpointSettings>) =>
  request<EndpointSettings>("POST", "/settings/endpoint", patch);

export const testHeadroom = (body?: { url?: string; timeout_ms?: number }) =>
  request<HeadroomTestResult>("POST", "/settings/headroom-test", body ?? {});

export const accessSettings = () => request<AccessSettings>("GET", "/settings/access");

export const updateAccessSettings = (patch: Partial<Omit<AccessSettings, "endpoint_url">>) =>
  request<AccessSettings>("POST", "/settings/access", patch);

// Branding / white-label settings.
export const branding = () => request<BrandingSettings>("GET", "/settings/branding");

export const updateBranding = (patch: Partial<BrandingSettings>) =>
  request<BrandingSettings>("POST", "/settings/branding", patch);
