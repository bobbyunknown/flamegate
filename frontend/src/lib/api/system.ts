import { request } from "./client";
import type { SystemSnapshot, SystemHistory } from "./types";

export const systemMonitor = () => request<SystemSnapshot>("GET", "/system");

export const systemHistory = () => request<SystemHistory>("GET", "/system/history");
