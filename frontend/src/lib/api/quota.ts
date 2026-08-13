import { request, browserTZ } from "./client";
import type { QuotaAccount } from "./types";

export const quota = (period: string) =>
  request<{ accounts: QuotaAccount[]; since: string }>("GET", `/quota?period=${period}&tz=${browserTZ()}`);

export const quotaByProvider = (provider: string) =>
  request<{ accounts: QuotaAccount[]; since: string }>("GET", `/quota?period=today&tz=${browserTZ()}&provider=${provider}`);
