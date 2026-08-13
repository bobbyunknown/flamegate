import { request, requestForm } from "./client";
import type { Extension } from "./types";

export const listExtensions = () =>
  request<{ extensions: Extension[] }>("GET", "/extensions");

export const getExtension = (slug: string) =>
  request<Extension>("GET", `/extensions/${encodeURIComponent(slug)}`);

export const installExtension = (wasm: File, schema: File) => {
  const body = new FormData();
  body.append("wasm", wasm);
  body.append("schema", schema);
  return requestForm<Extension>("POST", "/extensions/install", body);
};

export const uninstallExtension = (slug: string) =>
  request<{ ok: boolean; slug: string }>("DELETE", `/extensions/${encodeURIComponent(slug)}`);

export const enableExtension = (slug: string) =>
  request<{ ok: boolean; slug: string; state: string }>(
    "POST",
    `/extensions/${encodeURIComponent(slug)}/enable`,
  );

export const disableExtension = (slug: string) =>
  request<{ ok: boolean; slug: string; state: string }>(
    "POST",
    `/extensions/${encodeURIComponent(slug)}/disable`,
  );

export const syncExtensionModels = (slug: string) =>
  request<{ ok: boolean; slug: string; synced: number }>(
    "POST",
    `/extensions/${encodeURIComponent(slug)}/sync-models`,
  );

export const setExtensionAutoSyncModels = (slug: string, auto_sync_models: boolean) =>
  request<{ ok: boolean; slug: string; auto_sync_models: boolean }>(
    "PUT",
    `/extensions/${encodeURIComponent(slug)}/auto-sync-models`,
    { auto_sync_models },
  );
