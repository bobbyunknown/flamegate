import { BASE_URL } from "./client";
import type { UsageEvent, GuardrailLogEntry } from "./types";

/**
 * Creates an EventSource connected to the usage SSE stream. The caller
 * provides a callback that fires on each usage event. Returns a cleanup
 * function that closes the connection.
 */
export function connectUsageStream(onEvent: (ev: UsageEvent) => void): () => void {
  let es: EventSource | null = null;
  let retryCount = 0;
  const maxRetries = 10;
  let closed = false;

  function connect() {
    if (closed) return;
    es = new EventSource(`${BASE_URL}/api/usage/stream`, { withCredentials: true });
    
    es.onopen = () => {
      retryCount = 0; // reset on successful connection
    };
    
    es.onmessage = (msg) => {
      try {
        const ev = JSON.parse(msg.data) as UsageEvent;
        onEvent(ev);
      } catch { /* ignore malformed events */ }
    };
    
    es.onerror = () => {
      es?.close();
      if (closed) return;
      
      if (retryCount < maxRetries) {
        const delay = Math.min(1000 * 2 ** retryCount, 30000);
        setTimeout(connect, delay);
        retryCount++;
      }
    };
  }

  connect();

  return () => {
    closed = true;
    es?.close();
  };
}

/**
 * Subscribe to the guardrails audit-log SSE stream. New rows arrive as they
 * land in the database (the AuditWriter publishes after each successful batch
 * insert). Returns a cleanup function that closes the connection.
 */
export function connectGuardrailLogStream(
  onEvent: (row: GuardrailLogEntry) => void,
): () => void {
  let es: EventSource | null = null;
  let retryCount = 0;
  const maxRetries = 10;
  let closed = false;

  function connect() {
    if (closed) return;
    es = new EventSource(`${BASE_URL}/api/guardrails/logs/stream`, { withCredentials: true });
    es.onopen = () => {
      retryCount = 0;
    };
    es.onmessage = (msg) => {
      try {
        const row = JSON.parse(msg.data) as GuardrailLogEntry;
        onEvent(row);
      } catch {
        /* ignore malformed events */
      }
    };
    es.onerror = () => {
      es?.close();
      if (closed) return;
      if (retryCount < maxRetries) {
        const delay = Math.min(1000 * 2 ** retryCount, 30000);
        setTimeout(connect, delay);
        retryCount++;
      }
    };
  }

  connect();
  return () => {
    closed = true;
    es?.close();
  };
}
