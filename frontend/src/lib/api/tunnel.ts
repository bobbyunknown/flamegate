import { request } from "./client";
import type { TunnelCombinedStatus, TunnelEnableResult, TailscaleCheckResult, TailscaleEnableResult } from "./types";

export const tunnelStatus = () => request<TunnelCombinedStatus>("GET", "/tunnel/status");

export const tunnelEnable = () => request<TunnelEnableResult>("POST", "/tunnel/enable");

export const tunnelDisable = () => request<{ success: boolean }>("POST", "/tunnel/disable");

export const tailscaleCheck = () => request<TailscaleCheckResult>("GET", "/tunnel/tailscale-check");

export const tailscaleEnable = (sudoPassword?: string) =>
  request<TailscaleEnableResult>("POST", "/tunnel/tailscale-enable", sudoPassword ? { sudoPassword } : {});

export const tailscaleDisable = () => request<{ success: boolean }>("POST", "/tunnel/tailscale-disable");
