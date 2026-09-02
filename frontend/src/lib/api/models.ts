import { request, BASE_URL } from "./client";
import type { TestChatMessage, TestChatResult, TestModelResult } from "./types";

export const listDisabledModels = (providerAlias: string) =>
  request<{ ids: string[] }>("GET", `/models/disabled?provider=${encodeURIComponent(providerAlias)}`);

export const disableModels = (providerAlias: string, ids: string[]) =>
  request<{ ids: string[] }>("POST", "/models/disabled", { providerAlias, ids });

export const enableModels = (providerAlias: string, ids: string[]) =>
  request<{ ids: string[] }>("DELETE", "/models/disabled", { providerAlias, ids });

export const testModel = (input: { provider: string; model: string; account_id?: string }) =>
  request<TestModelResult>("POST", "/models/test", input);

export const testChat = (input: {
  provider: string;
  model: string;
  messages: TestChatMessage[];
  system?: string;
  temperature?: number;
  max_tokens?: number;
}) => request<TestChatResult>("POST", "/models/chat", input);

export interface StreamChatCallbacks {
  onDelta?: (delta: string) => void;
  onThinking?: (delta: string) => void;
  onDone?: (info: { latency_ms: number; ttft_ms: number; prompt_tokens: number; completion_tokens: number; model: string }) => void;
  onError?: (error: string) => void;
}

export async function streamChat(
  input: {
    provider: string;
    model: string;
    messages: TestChatMessage[];
    system?: string;
    temperature?: number;
    max_tokens?: number;
  },
  callbacks: StreamChatCallbacks,
  signal?: AbortSignal
): Promise<void> {
  const url = `${BASE_URL}/api/models/chat/stream`;
  try {
    const res = await fetch(url, {
      method: "POST",
      headers: {
        "Content-Type": "application/json",
      },
      body: JSON.stringify(input),
      credentials: "include",
      signal,
    });

    if (!res.ok) {
      const txt = await res.text();
      let msg = res.statusText;
      try {
        const json = JSON.parse(txt);
        msg = json.error || msg;
      } catch {}
      callbacks.onError?.(msg);
      return;
    }

    if (!res.body) {
      callbacks.onError?.("No response body received from streaming endpoint");
      return;
    }

    const reader = res.body.getReader();
    const decoder = new TextDecoder();
    let buffer = "";

    while (true) {
      const { done, value } = await reader.read();
      if (done) break;
      buffer += decoder.decode(value, { stream: true });

      const lines = buffer.split("\n");
      buffer = lines.pop() ?? "";

      let currentEvent = "";
      for (const line of lines) {
        const trimmed = line.trim();
        if (!trimmed) continue;
        if (trimmed.startsWith("event:")) {
          currentEvent = trimmed.slice(6).trim();
        } else if (trimmed.startsWith("data:")) {
          const dataStr = trimmed.slice(5).trim();
          try {
            const data = JSON.parse(dataStr);
            if (currentEvent === "delta" || data.type === "delta") {
              callbacks.onDelta?.(data.delta ?? "");
            } else if (currentEvent === "thinking" || data.type === "thinking") {
              callbacks.onThinking?.(data.delta ?? "");
            } else if (currentEvent === "done" || data.type === "done") {
              callbacks.onDone?.(data);
            } else if (currentEvent === "error" || data.type === "error") {
              callbacks.onError?.(data.error ?? "Stream error");
            }
          } catch {
            callbacks.onDelta?.(dataStr);
          }
        }
      }
    }
  } catch (err: any) {
    if (err.name === "AbortError") {
      return;
    }
    callbacks.onError?.(err.message || "Failed to stream chat");
  }
}

