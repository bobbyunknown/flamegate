import { request } from "./client";
import type { Chain } from "./types";

export const listChains = () => request<{ chains: Chain[] }>("GET", "/chains");

export const createChain = (input: { name: string; strategy?: string; fallback_provider?: string; fallback_model?: string; steps: { provider: string; model: string }[] }) =>
  request<{ id: string }>("POST", "/chains", input);

export const updateChain = (id: string, patch: { name?: string; strategy?: string; fallback_provider?: string; fallback_model?: string; steps?: { provider: string; model: string }[] }) =>
  request<{ id: string }>("PATCH", `/chains/${id}`, patch);

export const deleteChain = (id: string) => request<void>("DELETE", `/chains/${id}`);
