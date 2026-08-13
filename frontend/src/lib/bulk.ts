// Standardized bulk-import parsing shared across the dashboard (API keys,
// proxy pools, and any future batch importer). The goal is one predictable,
// forgiving text format so users learn it once.
//
// Format rules (applied uniformly):
//   - One entry per line.
//   - Blank lines are ignored.
//   - Lines beginning with "#" (after trimming) are comments and ignored.
//   - Fields within a line are comma-separated. Surrounding whitespace on each
//     field is trimmed.
//   - A leading header row (e.g. "label,api_key") is detected and skipped.
//
// Each importer layers its own field mapping on top of stripComments().

/** A single parsed line with its 1-based source line number (for diagnostics). */
export interface RawLine {
  /** 1-based line number in the original text. */
  line: number;
  /** Trimmed, non-empty, non-comment content. */
  text: string;
}

/** stripComments returns the meaningful lines of a bulk-import blob. */
export function stripComments(input: string): RawLine[] {
  const out: RawLine[] = [];
  const lines = input.split(/\r?\n/);
  for (let i = 0; i < lines.length; i++) {
    const text = lines[i].trim();
    if (!text || text.startsWith("#")) continue;
    out.push({ line: i + 1, text });
  }
  return out;
}

/** splitFields splits a line on commas and trims each field. */
export function splitFields(text: string): string[] {
  return text.split(",").map((f) => f.trim());
}

// ─── API keys ────────────────────────────────────────────────────────────────

export interface ParsedKey {
  line: number;
  label: string;
  apiKey: string;
  baseURL?: string;
}

export interface ParsedKeyResult {
  entries: ParsedKey[];
  /** Lines skipped as exact duplicates of an earlier key. */
  duplicates: number;
  /** Per-line problems that prevented parsing (e.g. empty key). */
  errors: { line: number; message: string }[];
}

const KEY_HEADER = /^label\s*,\s*(api[_ ]?key|key|secret)/i;

/**
 * parseKeys reads the standardized key format. Each line is one of:
 *   - `apiKey`
 *   - `label,apiKey`
 *   - `label,apiKey,baseURL`
 * When only one field is present it is treated as the key (label auto-filled
 * downstream). Duplicate keys are dropped and counted.
 */
export function parseKeys(input: string): ParsedKeyResult {
  const entries: ParsedKey[] = [];
  const errors: { line: number; message: string }[] = [];
  const seen = new Set<string>();
  let duplicates = 0;

  const rows = stripComments(input);
  rows.forEach((row, idx) => {
    // Skip an optional CSV-style header on the first meaningful row.
    if (idx === 0 && KEY_HEADER.test(row.text)) return;

    const fields = splitFields(row.text);
    let label = "";
    let apiKey = "";
    let baseURL: string | undefined;

    if (fields.length === 1) {
      apiKey = fields[0];
    } else {
      label = fields[0];
      apiKey = fields[1];
      if (fields.length >= 3 && fields[2]) baseURL = fields[2];
    }

    if (!apiKey) {
      errors.push({ line: row.line, message: "missing API key" });
      return;
    }
    if (seen.has(apiKey)) {
      duplicates++;
      return;
    }
    seen.add(apiKey);
    entries.push({ line: row.line, label, apiKey, baseURL });
  });

  return { entries, duplicates, errors };
}

// ─── Proxies ─────────────────────────────────────────────────────────────────

export interface ParsedProxy {
  line: number;
  name?: string;
  url: string;
}

export interface ParsedProxyResult {
  entries: ParsedProxy[];
  duplicates: number;
  errors: { line: number; message: string }[];
}

const PROXY_HEADER = /^name\s*,\s*(proxy[_ ]?url|url)/i;

/**
 * normalizeProxyURL accepts the two supported proxy spellings and returns a
 * canonical `protocol://[user:pass@]host:port` URL:
 *   - `protocol://user:pass@host:port` (passed through)
 *   - `host:port:user:pass` (rewritten to http://user:pass@host:port)
 *   - `host:port` (rewritten to http://host:port)
 */
export function normalizeProxyURL(raw: string): string {
  const value = raw.trim();
  if (value.includes("://")) return value;
  const parts = value.split(":");
  if (parts.length === 4) {
    return `http://${parts[2]}:${parts[3]}@${parts[0]}:${parts[1]}`;
  }
  return `http://${value}`;
}

/**
 * parseProxies reads the standardized proxy format. Each line is one of:
 *   - `url`
 *   - `name,url`
 * where `url` follows normalizeProxyURL(). Duplicate URLs are dropped.
 */
export function parseProxies(input: string): ParsedProxyResult {
  const entries: ParsedProxy[] = [];
  const errors: { line: number; message: string }[] = [];
  const seen = new Set<string>();
  let duplicates = 0;

  const rows = stripComments(input);
  rows.forEach((row, idx) => {
    if (idx === 0 && PROXY_HEADER.test(row.text)) return;

    const fields = splitFields(row.text);
    let name: string | undefined;
    let rawURL = "";
    if (fields.length === 1) {
      rawURL = fields[0];
    } else {
      name = fields[0] || undefined;
      rawURL = fields[1];
    }
    if (!rawURL) {
      errors.push({ line: row.line, message: "missing proxy URL" });
      return;
    }
    const url = normalizeProxyURL(rawURL);
    if (seen.has(url)) {
      duplicates++;
      return;
    }
    seen.add(url);
    entries.push({ line: row.line, name, url });
  });

  return { entries, duplicates, errors };
}

// ─── Concurrency helper ──────────────────────────────────────────────────────

/**
 * runPool runs `task` over `items` with at most `concurrency` in flight,
 * preserving input order in the returned results. Used to parallelize
 * per-item network work (e.g. proxy connectivity tests) without flooding the
 * backend.
 */
export async function runPool<T, R>(
  items: T[],
  concurrency: number,
  task: (item: T, index: number) => Promise<R>,
): Promise<R[]> {
  const results = new Array<R>(items.length);
  let cursor = 0;
  const workers = new Array(Math.min(concurrency, items.length)).fill(0).map(async () => {
    while (true) {
      const index = cursor++;
      if (index >= items.length) break;
      results[index] = await task(items[index], index);
    }
  });
  await Promise.all(workers);
  return results;
}
