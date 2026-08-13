import { request } from "./client";
import type { DeviceCode, OAuthPollResult } from "./types";

// Kiro connect flow (AWS SSO OIDC device flows + import token). Mounted under
// /kiro (not /oauth/kiro) to avoid the chi /oauth/{provider} route collision.
export const kiroDeviceStart = (input: { method: "builder-id" | "idc"; start_url?: string; region?: string }) =>
  request<DeviceCode>("POST", "/kiro/device-start", input);

export const kiroDevicePoll = (deviceCode: string, label?: string) =>
  request<OAuthPollResult>("POST", "/kiro/device-poll", { device_code: deviceCode, label });

export const kiroAPIKey = (apiKey: string, region?: string, label?: string) =>
  request<{ id: string; provider: string }>("POST", "/kiro/api-key", {
    api_key: apiKey,
    region,
    label,
  });

export const kiroImport = (refreshToken: string, label?: string) =>
  request<{ id: string; provider: string }>("POST", "/kiro/import", {
    refresh_token: refreshToken,
    label,
  });

// Qoder connect flow (PKCE device-token poll). Mounted under /qoder (not
// /oauth/qoder) to avoid the chi /oauth/{provider} route collision. The flow
// generates a PKCE pair + nonce locally, opens the Qoder account picker in
// the browser, then polls until the user authorizes.
export const qoderDeviceStart = () =>
  request<DeviceCode>("POST", "/qoder/device-start", {});

export const qoderDevicePoll = (deviceCode: string, label?: string) =>
  request<OAuthPollResult>("POST", "/qoder/device-poll", { device_code: deviceCode, label });

// KiloCode connect flow (custom device-auth). Mounted under /kilocode (not
// /oauth/kilocode) to avoid the chi /oauth/{provider} route collision.
export const kilocodeDeviceStart = () =>
  request<DeviceCode>("POST", "/kilocode/device-start", {});

export const kilocodeDevicePoll = (deviceCode: string, label?: string) =>
  request<OAuthPollResult>("POST", "/kilocode/device-poll", { device_code: deviceCode, label });

// CodeBuddy connect flow (browser-poll auth). Mounted under /codebuddy.
export const codebuddyAuthStart = () =>
  request<DeviceCode>("POST", "/codebuddy/auth-start", {});

export const codebuddyAuthPoll = (deviceCode: string, label?: string) =>
  request<OAuthPollResult>("POST", "/codebuddy/auth-poll", { device_code: deviceCode, label });

// Cursor connect flow (import token from Cursor IDE). Mounted under /cursor.
export const cursorImport = (token: string, label?: string) =>
  request<{ id: string; provider: string }>("POST", "/cursor/import", { token, label });

// Command Code connect flow (import token from CLI or studio). Mounted under /commandcode.
export const commandcodeImport = (token: string, label?: string) =>
  request<{ id: string; provider: string }>("POST", "/commandcode/import", { token, label });
