import { request } from "./client";
import type { ProxyPool } from "./types";

export const listProxyPools = () => request<{ pools: ProxyPool[] }>("GET", "/proxy-pools");

export const createProxyPool = (input: { name: string; type?: string; proxy_url: string; no_proxy?: string; strict?: boolean; is_active?: boolean }) =>
  request<{ id: string }>("POST", "/proxy-pools", input);

export const updateProxyPool = (id: string, patch: { name?: string; proxy_url?: string; no_proxy?: string; strict?: boolean; is_active?: boolean }) =>
  request<void>("PATCH", `/proxy-pools/${id}`, patch);

export const deleteProxyPool = (id: string) => request<void>("DELETE", `/proxy-pools/${id}`);
