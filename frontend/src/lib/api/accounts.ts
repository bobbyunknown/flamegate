import { request } from "./client";
import type { Account, AccountInput, BulkAccountInput, BulkAccountResponse, UpstreamQuota } from "./types";

export const listAccounts = () => request<{ accounts: Account[] }>("GET", "/accounts");

export const createAccount = (input: AccountInput) =>
  request<{ id: string }>("POST", "/accounts", input);

export const bulkCreateAccounts = (input: BulkAccountInput) =>
  request<BulkAccountResponse>("POST", "/accounts/bulk", input);

export const updateAccount = (id: string, patch: { label?: string; priority?: number; disabled?: boolean; proxy_pool_id?: string }) =>
  request<{ id: string }>("PATCH", `/accounts/${id}`, patch);

export const deleteAccount = (id: string) => request<void>("DELETE", `/accounts/${id}`);

export const testAccount = (id: string) =>
  request<{ id: string; status: string; message: string }>("POST", `/accounts/${id}/test`);

export const validateKey = (input: AccountInput) =>
  request<{ status: string; message?: string }>("POST", "/validate-key", input);

export const accountQuota = (id: string) =>
  request<{ provider: string; supported: boolean; plan_name?: string; message?: string; quotas?: UpstreamQuota[] }>(
    "GET", `/accounts/${id}/quota`,
  );
