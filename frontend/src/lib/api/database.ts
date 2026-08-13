import { request, requestBlob, requestForm } from "./client";
import type { SQLiteStatus, SQLiteRestoreResult, UpdateInfo } from "./types";

// Update check (queries GitHub for the latest release + changelog).
// Pass force=true to bypass the backend's 6-hour cache (the "Check now" button).
export const updateCheck = (force?: boolean) =>
  request<UpdateInfo>("GET", `/update/check${force ? "?refresh=1" : ""}`);

// Database export/import. An optional passphrase produces a portable backup
// whose credentials are re-keyed to the passphrase (movable across machines
// with different master keys).
export const exportDatabase = (passphrase?: string) =>
  request<Record<string, unknown>>(
    "GET",
    passphrase ? `/settings/database?passphrase=${encodeURIComponent(passphrase)}` : "/settings/database",
  );

export const importDatabase = (payload: Record<string, unknown>, passphrase?: string) =>
  request<{ imported: number }>("POST", "/settings/database", passphrase ? { ...payload, passphrase } : payload);

export const sqliteStatus = () => request<SQLiteStatus>("GET", "/settings/sqlite");

export const backupSQLite = () => requestBlob("GET", "/settings/sqlite/backup");

export const restoreSQLite = (file: File) => {
  const body = new FormData();
  body.append("file", file);
  return requestForm<SQLiteRestoreResult>("POST", "/settings/sqlite/restore", body);
};
