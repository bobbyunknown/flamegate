import { request } from "./client";
import type { Plan, APIKey } from "./types";

export const listPlans = () => request<{ plans: Plan[] }>("GET", "/plans");

export const createPlan = (input: {
  name: string;
  description?: string;
  limit_usd?: number;
  limit_tokens?: number;
  rpm_limit?: number;
  tpm_limit?: number;
  concurrency_limit?: number;
  period?: string;
  alert_pct?: number;
  hard_cutoff?: boolean;
  allowed_models?: string[];
}) => request<Plan>("POST", "/plans", input);

export const updatePlan = (id: string, patch: {
  name?: string;
  description?: string;
  limit_usd?: number;
  limit_tokens?: number;
  rpm_limit?: number;
  tpm_limit?: number;
  concurrency_limit?: number;
  period?: string;
  alert_pct?: number;
  hard_cutoff?: boolean;
  allowed_models?: string[];
}) => request<Plan>("PATCH", `/plans/${id}`, patch);

export const deletePlan = (id: string) => request<void>("DELETE", `/plans/${id}`);

export const listPlanKeys = (id: string) => request<{ keys: APIKey[] }>("GET", `/plans/${id}/keys`);
