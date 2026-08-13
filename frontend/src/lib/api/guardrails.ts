import { request } from "./client";
import type {
  GuardrailScope,
  GuardrailPolicy,
  GuardrailPolicyConfig,
  EffectiveGuardrail,
  GuardrailLogEntry,
  GuardrailTestResult,
  GuardrailTemplate,
  GuardrailBundle,
} from "./types";

export const listGuardrails = (scope?: GuardrailScope) =>
  request<{ guardrails: GuardrailPolicy[] }>(
    "GET",
    scope ? `/guardrails?scope=${encodeURIComponent(scope)}` : "/guardrails",
  );

export const getGuardrail = (id: string) =>
  request<GuardrailPolicy>("GET", `/guardrails/${id}`);

export const createGuardrail = (input: {
  name?: string;
  scope: GuardrailScope;
  scope_id?: string;
  enabled?: boolean;
  config?: GuardrailPolicyConfig;
}) => request<GuardrailPolicy>("POST", "/guardrails", input);

export const updateGuardrail = (
  id: string,
  patch: { name?: string; enabled?: boolean; config?: GuardrailPolicyConfig },
) => request<GuardrailPolicy>("PATCH", `/guardrails/${id}`, patch);

export const deleteGuardrail = (id: string) =>
  request<void>("DELETE", `/guardrails/${id}`);

export const effectiveGuardrail = (params: {
  provider?: string;
  model?: string;
  chain?: string;
  apikey?: string;
}) => {
  const qs = new URLSearchParams();
  if (params.provider) qs.set("provider", params.provider);
  if (params.model) qs.set("model", params.model);
  if (params.chain) qs.set("chain", params.chain);
  if (params.apikey) qs.set("apikey", params.apikey);
  const suffix = qs.toString();
  return request<EffectiveGuardrail>(
    "GET",
    `/guardrails/effective${suffix ? `?${suffix}` : ""}`,
  );
};

export const listGuardrailEntities = () =>
  request<{ entities: string[] }>("GET", "/guardrails/entities");

export const listGuardrailLogs = (filter?: {
  api_key_id?: string;
  detector?: string;
  action?: string;
  limit?: number;
}) => {
  const qs = new URLSearchParams();
  if (filter?.api_key_id) qs.set("api_key_id", filter.api_key_id);
  if (filter?.detector) qs.set("detector", filter.detector);
  if (filter?.action) qs.set("action", filter.action);
  if (filter?.limit) qs.set("limit", String(filter.limit));
  const suffix = qs.toString();
  return request<{ logs: GuardrailLogEntry[] }>(
    "GET",
    `/guardrails/logs${suffix ? `?${suffix}` : ""}`,
  );
};

export const testGuardrail = (input: { text: string; config?: GuardrailPolicyConfig }) =>
  request<GuardrailTestResult>("POST", "/guardrails/test", input);

export const listGuardrailTemplates = () =>
  request<{ templates: GuardrailTemplate[] }>("GET", "/guardrails/templates");

export const exportGuardrails = (scope?: string) =>
  request<GuardrailBundle>(
    "GET",
    `/guardrails/export${scope ? `?scope=${encodeURIComponent(scope)}` : ""}`,
  );

export const importGuardrails = (bundle: GuardrailBundle) =>
  request<{
    imported: Array<{ name: string; scope: string; scope_id?: string }>;
    skipped: Array<{ name: string; reason: string }>;
  }>("POST", "/guardrails/import", bundle);

export const getGuardrailTenantFlags = () =>
  request<{ allow_external_engines: boolean }>(
    "GET",
    "/guardrails/tenant-flags",
  );

export const putGuardrailTenantFlags = (flags: { allow_external_engines?: boolean }) =>
  request<{ allow_external_engines: boolean }>(
    "PUT",
    "/guardrails/tenant-flags",
    flags,
  );
