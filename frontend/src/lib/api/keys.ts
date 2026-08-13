import { request } from "./client";
import type { APIKey, CreatedKey } from "./types";

export const listKeys = () => request<{ keys: APIKey[] }>("GET", "/keys");

export const createKey = (name: string, opts?: {
  plan_id?: string;
  budget_limit_usd?: number;
  budget_limit_tokens?: number;
  budget_period?: string;
  budget_alert_pct?: number;
  budget_hard_cutoff?: boolean;
  allowed_models?: string[];
}) =>
  request<CreatedKey>("POST", "/keys", { name, ...(opts ? opts : {}) });

export const updateKey = (id: string, patch: { disabled?: boolean; allowed_models?: string[] }) =>
  request<{ id: string; disabled?: boolean; allowed_models?: string[] }>("PATCH", `/keys/${id}`, patch);

export const deleteKey = (id: string) => request<void>("DELETE", `/keys/${id}`);

export const deleteKeys = (ids: string[]) => Promise.all(ids.map((id) => request<void>("DELETE", `/keys/${id}`)));
