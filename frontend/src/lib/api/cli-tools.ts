import { request } from "./client";
import type { CLIToolsResponse } from "./types";

export const cliTools = (model?: string) =>
  request<CLIToolsResponse>("GET", model ? `/cli-tools?model=${encodeURIComponent(model)}` : "/cli-tools");

export const cliToolConfigure = (toolId: string, body: { base_url: string; api_key: string; models?: string[] }) =>
  request<{ ok: boolean }>("POST", `/cli-tools/${toolId}/configure`, body);

export const cliToolRemove = (toolId: string) =>
  request<{ ok: boolean }>("POST", `/cli-tools/${toolId}/remove`);
