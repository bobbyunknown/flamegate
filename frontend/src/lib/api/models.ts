import { request } from "./client";

export const listDisabledModels = (providerAlias: string) =>
  request<{ ids: string[] }>("GET", `/models/disabled?provider=${encodeURIComponent(providerAlias)}`);

export const disableModels = (providerAlias: string, ids: string[]) =>
  request<{ ids: string[] }>("POST", "/models/disabled", { providerAlias, ids });

export const enableModels = (providerAlias: string, ids: string[]) =>
  request<{ ids: string[] }>("DELETE", "/models/disabled", { providerAlias, ids });
