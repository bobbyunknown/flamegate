import { request } from "./client";
import type { Provider, ProviderModel, ProviderRoutingSettings, CustomProvider, CustomModel, CustomModelInput } from "./types";

export const providers = () => request<{ providers: Provider[] }>("GET", "/providers");

export const providerModels = (id: string, kind?: string, refresh?: boolean) =>
  request<{ models: ProviderModel[] }>(
    "GET",
    `/providers/${id}/models?${new URLSearchParams({
      ...(kind ? { kind } : {}),
      ...(refresh ? { refresh: "true" } : {}),
    }).toString()}`,
  );

export const syncProviderModels = (id: string) =>
  providerModels(id, undefined, true);

export const deleteProviderModel = (id: string, modelId: string) =>
  request<{ provider: string; cleared: number }>(
    "DELETE",
    `/providers/${id}/models?model_id=${encodeURIComponent(modelId)}`,
  );

export const clearProviderModels = (id: string) =>
  request<{ provider: string; cleared: number }>("DELETE", `/providers/${id}/models`);

export const bulkDeleteProviderModels = (id: string, modelIds: string[]) =>
  request<{ provider: string; deleted: number }>(
    "POST",
    `/providers/${id}/models/bulk-delete`,
    { model_ids: modelIds },
  );

export const providerRouting = (id: string) =>
  request<ProviderRoutingSettings>("GET", `/providers/${id}/routing`);

export const updateProviderRouting = (id: string, patch: Partial<ProviderRoutingSettings>) =>
  request<ProviderRoutingSettings>("POST", `/providers/${id}/routing`, patch);

// Custom provider instances (dynamic OpenAI-/Anthropic-compatible providers).
export const listCustomProviders = () =>
  request<{ providers: CustomProvider[] }>("GET", "/custom-providers");

export const createCustomProvider = (input: { display_name: string; dialect: string; base_url: string }) =>
  request<CustomProvider>("POST", "/custom-providers", input);

export const updateCustomProvider = (id: string, patch: { display_name?: string; alias?: string; base_url?: string }) =>
  request<CustomProvider>("PATCH", `/custom-providers/${id}`, patch);

export const deleteCustomProvider = (id: string) =>
  request<{ id: string; deleted: boolean }>("DELETE", `/custom-providers/${id}`);

// Custom models, attachable to any provider id (custom or built-in).
export const listCustomModels = (providerId: string) =>
  request<{ models: CustomModel[] }>("GET", `/providers/${providerId}/custom-models`);

export const createCustomModel = (providerId: string, input: CustomModelInput) =>
  request<CustomModel>("POST", `/providers/${providerId}/custom-models`, input);

export const updateCustomModel = (providerId: string, dbId: string, patch: Partial<CustomModelInput>) =>
  request<CustomModel>("PATCH", `/providers/${providerId}/custom-models/${dbId}`, patch);

export const deleteCustomModel = (providerId: string, dbId: string) =>
  request<{ db_id: string; deleted: boolean }>("DELETE", `/providers/${providerId}/custom-models/${dbId}`);
