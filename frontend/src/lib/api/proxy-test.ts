import { request } from "./client";

// Proxy test.
export const testProxy = (proxyUrl: string) =>
  request<{ ok: boolean; status?: number; elapsedMs?: number; error?: string; exitIP?: string }>("POST", "/settings/proxy-test", { proxyUrl });

// Proxy pool test.
export const testProxyPool = (id: string) =>
  request<{ status: string; last_tested?: string }>("POST", `/proxy-pools/${id}/test`);
