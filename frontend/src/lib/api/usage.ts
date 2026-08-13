import { request, browserTZ } from "./client";
import type { UsageSummary, UsageInsights, ModelUsage } from "./types";

export const usage = (period: string) =>
  request<UsageSummary>("GET", `/usage?period=${period}&tz=${browserTZ()}`);

export const usageInsights = (period: string) =>
  request<UsageInsights>("GET", `/usage/insights?period=${period}&tz=${browserTZ()}`);

export const modelUsage = (period: string) =>
  request<{ models: ModelUsage[] }>("GET", `/usage/models?period=${period}&tz=${browserTZ()}`);
