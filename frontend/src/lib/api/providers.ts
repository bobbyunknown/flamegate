import { request } from "./client";
import type { Provider, ProviderModel, ProviderRoutingSettings, CustomProvider, CustomModel, CustomModelInput } from "./types";

export const providers = () => request<{ providers: Provider[] }>("GET", "/providers");

export const providerModels = (id: string, kind?: string) =>
  request<{ models: ProviderModel[] }>(
    "GET",
    `/providers/${id}/models${kind ? `?kind=${encodeURIComponent(kind)}` : ""}`,
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
