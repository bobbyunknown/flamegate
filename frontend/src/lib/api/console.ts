import { request } from "./client";
import type { ConsoleLogEntry } from "./types";

export const consoleLog = () => request<{ logs: ConsoleLogEntry[] }>("GET", "/console");
