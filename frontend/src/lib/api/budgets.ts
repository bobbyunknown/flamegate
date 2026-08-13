import { request } from "./client";
import type { Budget, BudgetStatus } from "./types";

export const listBudgets = () => request<{ budgets: Budget[] }>("GET", "/budgets");

export const budgetStatus = () => request<{ budgets: BudgetStatus[] }>("GET", "/budgets/status");

export const createBudget = (input: { scope_kind?: string; scope_id?: string; limit_usd?: number; limit_tokens?: number; period?: string; alert_pct?: number; hard_cutoff?: boolean }) =>
  request<{ id: string }>("POST", "/budgets", input);

export const updateBudget = (id: string, patch: { limit_usd?: number; limit_tokens?: number; period?: string; alert_pct?: number; hard_cutoff?: boolean }) =>
  request<void>("PATCH", `/budgets/${id}`, patch);

export const deleteBudget = (id: string) => request<void>("DELETE", `/budgets/${id}`);
