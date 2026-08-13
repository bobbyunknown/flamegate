export class APIError extends Error {
  status: number;
  constructor(status: number, message: string) {
    super(message);
    this.status = status;
  }
}

export const BASE_URL = import.meta.env.VITE_API_URL || "";

const DEFAULT_TIMEOUT_MS = 30_000;

// fetchWithTimeout wraps fetch with an AbortController-based deadline. On
// timeout the request is aborted and a clear APIError(408) is thrown so callers
// can distinguish a stall from a network/HTTP failure.
async function fetchWithTimeout(
  input: string,
  init: RequestInit = {},
  timeoutMs = DEFAULT_TIMEOUT_MS,
): Promise<Response> {
  const controller = new AbortController();
  const timer = setTimeout(() => controller.abort(), timeoutMs);
  try {
    return await fetch(input, { ...init, signal: controller.signal, credentials: "include" });
  } catch (err) {
    if (err instanceof DOMException && err.name === "AbortError") {
      throw new APIError(408, "Request timed out. Is the backend reachable?");
    }
    throw err;
  } finally {
    clearTimeout(timer);
  }
}

/** Returns the browser's IANA timezone (e.g. "Asia/Jakarta"), falling back to UTC. */
export function browserTZ(): string {
  try {
    return Intl.DateTimeFormat().resolvedOptions().timeZone || "UTC";
  } catch {
    return "UTC";
  }
}

export async function request<T>(method: string, path: string, body?: unknown): Promise<T> {
  const res = await fetchWithTimeout(`${BASE_URL}/api${path}`, {
    method,
    headers: body ? { "Content-Type": "application/json" } : undefined,
    body: body ? JSON.stringify(body) : undefined,
  });
  if (!res.ok) {
    let message = res.statusText;
    try {
      const data = await res.json();
      message = data?.error?.message ?? message;
    } catch {
      // keep statusText
    }
    throw new APIError(res.status, message);
  }
  if (res.status === 204) return undefined as T;
  return res.json() as Promise<T>;
}

export async function requestBlob(method: string, path: string): Promise<Blob> {
  const res = await fetchWithTimeout(`${BASE_URL}/api${path}`, { method });
  if (!res.ok) {
    let message = res.statusText;
    try {
      const data = await res.json();
      message = data?.error?.message ?? message;
    } catch {
      // keep statusText
    }
    throw new APIError(res.status, message);
  }
  return res.blob();
}

export async function requestForm<T>(method: string, path: string, body: FormData): Promise<T> {
  const res = await fetchWithTimeout(`${BASE_URL}/api${path}`, { method, body }, 60_000);
  if (!res.ok) {
    let message = res.statusText;
    try {
      const data = await res.json();
      message = data?.error?.message ?? message;
    } catch {
      // keep statusText
    }
    throw new APIError(res.status, message);
  }
  if (res.status === 204) return undefined as T;
  return res.json() as Promise<T>;
}
